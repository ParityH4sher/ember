package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxResponseBytes   = 1 << 20
	availabilityTTL    = 30 * time.Second
	availabilityProbe  = 3 * time.Second
	keyRefetchInterval = time.Minute
	clockSkew          = time.Minute
	maxClaimItems      = 256
	maxClaimLength     = 256
)

type metadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

// provider talks to the configured OpenID provider. Discovery metadata is
// cached for the process lifetime; signing keys are refetched when an ID token
// names an unknown key, at most once per keyRefetchInterval.
type provider struct {
	cfg    *Config
	client *http.Client
	now    func() time.Time
	logger *slog.Logger

	mu          sync.Mutex
	meta        *metadata
	keys        []verifyKey
	keysFetched time.Time

	probe    sync.Mutex
	checked  time.Time
	checkErr error
}

func (p *provider) discoveryURL() string {
	return strings.TrimSuffix(p.cfg.Issuer, "/") + "/.well-known/openid-configuration"
}

func (p *provider) fetchMetadata(ctx context.Context) (*metadata, error) {
	var m metadata
	if err := p.getJSON(ctx, p.discoveryURL(), &m); err != nil {
		return nil, err
	}
	if m.Issuer != p.cfg.Issuer {
		return nil, fmt.Errorf("discovery issuer %q does not match the configured issuer", m.Issuer)
	}
	for name, raw := range map[string]string{
		"authorization_endpoint": m.AuthorizationEndpoint,
		"token_endpoint":         m.TokenEndpoint,
		"jwks_uri":               m.JWKSURI,
	} {
		if _, err := endpointURL(raw, name); err != nil {
			return nil, fmt.Errorf("discovery document: %w", err)
		}
	}
	if m.EndSessionEndpoint != "" {
		if _, err := endpointURL(m.EndSessionEndpoint, "end_session_endpoint"); err != nil {
			m.EndSessionEndpoint = "" // optional; sign out locally only
		}
	}
	return &m, nil
}

// discover returns the cached discovery metadata, fetching it on first use.
func (p *provider) discover(ctx context.Context) (*metadata, error) {
	p.mu.Lock()
	m := p.meta
	p.mu.Unlock()
	if m != nil {
		return m, nil
	}
	m, err := p.fetchMetadata(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.meta == nil {
		p.meta = m
	}
	return p.meta, nil
}

func (p *provider) cachedMetadata() *metadata {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.meta
}

// available reports whether the issuer answered discovery within the last
// availabilityTTL. Existing sessions pause while it does not (DEC-002).
func (p *provider) available(ctx context.Context) error {
	p.probe.Lock()
	defer p.probe.Unlock()
	if !p.checked.IsZero() && p.now().Sub(p.checked) < availabilityTTL {
		return p.checkErr
	}
	// One cancelled browser request must not mark the issuer unavailable for everyone.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), availabilityProbe)
	defer cancel()
	m, err := p.fetchMetadata(ctx)
	if err == nil {
		p.mu.Lock()
		if p.meta == nil {
			p.meta = m
		}
		p.mu.Unlock()
	}
	// Log transitions only, not every paused request.
	switch {
	case err != nil && (p.checked.IsZero() || p.checkErr == nil):
		p.logger.Warn("identity provider unavailable; signed-in access is paused", "error", err)
	case err == nil && p.checkErr != nil:
		p.logger.Info("identity provider available again")
	}
	p.checked, p.checkErr = p.now(), err
	return err
}

func (p *provider) getJSON(ctx context.Context, rawURL string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP %d", rawURL, resp.StatusCode)
	}
	return decodeLimited(resp.Body, dst)
}

func decodeLimited(r io.Reader, dst any) error {
	data, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxResponseBytes {
		return errors.New("identity provider response is too large")
	}
	return json.Unmarshal(data, dst)
}

func (p *provider) keysFor(ctx context.Context, jwksURI, kid string) ([]verifyKey, error) {
	p.mu.Lock()
	keys, fetched := p.keys, p.keysFetched
	p.mu.Unlock()
	if match := selectKeys(keys, kid); len(match) > 0 {
		return match, nil
	}
	if !fetched.IsZero() && p.now().Sub(fetched) < keyRefetchInterval {
		return nil, errors.New("no signing key matches the ID token")
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := p.getJSON(ctx, jwksURI, &set); err != nil {
		return nil, err
	}
	keys = parseKeys(set.Keys)
	p.mu.Lock()
	p.keys, p.keysFetched = keys, p.now()
	p.mu.Unlock()
	if match := selectKeys(keys, kid); len(match) > 0 {
		return match, nil
	}
	return nil, errors.New("no signing key matches the ID token")
}

func selectKeys(keys []verifyKey, kid string) []verifyKey {
	if kid == "" {
		return keys
	}
	var match []verifyKey
	for _, k := range keys {
		if k.kid == kid {
			match = append(match, k)
		}
	}
	return match
}

// exchange redeems an authorization code. Only the ID token is kept; Ember
// does not call other APIs on the user's behalf, so it discards access and
// refresh tokens.
func (p *provider) exchange(ctx context.Context, m *metadata, code, verifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.cfg.RedirectURL},
		"code_verifier": {verifier},
	}
	method := p.cfg.TokenEndpointAuthMethod
	if method != methodBasic {
		form.Set("client_id", p.cfg.ClientID)
	}
	if method == methodPost {
		form.Set("client_secret", p.cfg.clientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if method == methodBasic {
		// RFC 6749 section 2.3.1 form-encodes both values before Basic encoding.
		req.SetBasicAuth(url.QueryEscape(p.cfg.ClientID), url.QueryEscape(p.cfg.clientSecret))
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		_ = decodeLimited(resp.Body, &failure)
		return "", fmt.Errorf("token endpoint returned HTTP %d (%s)", resp.StatusCode, printable(failure.Error, 64))
	}
	var tokens struct {
		IDToken string `json:"id_token"`
	}
	if err := decodeLimited(resp.Body, &tokens); err != nil {
		return "", err
	}
	if tokens.IDToken == "" {
		return "", errors.New("token response has no id_token; check that the openid scope is granted")
	}
	return tokens.IDToken, nil
}

// verifyIDToken validates the ID token per OpenID Connect Core 3.1.3.7.
func (p *provider) verifyIDToken(ctx context.Context, m *metadata, raw, nonce string) (User, error) {
	header, payload, err := parseJWT(raw)
	if err != nil {
		return User{}, err
	}
	keys, err := p.keysFor(ctx, m.JWKSURI, header.Kid)
	if err != nil {
		return User{}, err
	}
	if err := verifySignature(raw, header, keys); err != nil {
		return User{}, err
	}
	var claims struct {
		Issuer            string   `json:"iss"`
		Subject           string   `json:"sub"`
		Audience          audience `json:"aud"`
		AuthorizedParty   string   `json:"azp"`
		Expiry            *float64 `json:"exp"`
		IssuedAt          *float64 `json:"iat"`
		Nonce             string   `json:"nonce"`
		Name              string   `json:"name"`
		PreferredUsername string   `json:"preferred_username"`
		Email             string   `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return User{}, errors.New("ID token claims are malformed")
	}
	now := p.now()
	switch {
	case claims.Issuer != p.cfg.Issuer:
		return User{}, errors.New("ID token issuer does not match")
	case !claims.Audience.contains(p.cfg.ClientID):
		return User{}, errors.New("ID token audience does not include this client")
	case len(claims.Audience) > 1 && claims.AuthorizedParty != p.cfg.ClientID,
		claims.AuthorizedParty != "" && claims.AuthorizedParty != p.cfg.ClientID:
		return User{}, errors.New("ID token authorized party is not this client")
	case claims.Expiry == nil || !now.Before(unixTime(*claims.Expiry).Add(clockSkew)):
		return User{}, errors.New("ID token has expired")
	case claims.IssuedAt == nil || unixTime(*claims.IssuedAt).After(now.Add(clockSkew)):
		return User{}, errors.New("ID token issue time is missing or in the future")
	case subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1:
		return User{}, errors.New("ID token nonce does not match the sign-in request")
	case claims.Subject == "" || len(claims.Subject) > maxClaimLength:
		return User{}, errors.New("ID token subject is missing or too long")
	}

	var all map[string]json.RawMessage
	if err := json.Unmarshal(payload, &all); err != nil {
		return User{}, errors.New("ID token claims are malformed")
	}
	name := claims.Name
	if name == "" {
		name = claims.PreferredUsername
	}
	return User{
		Subject: claims.Subject,
		Name:    printable(name, maxClaimLength),
		Email:   printable(claims.Email, maxClaimLength),
		Groups:  stringList(all[p.cfg.GroupsClaim]),
	}, nil
}

// unixTime converts a JWT NumericDate, clamped so huge values cannot overflow.
func unixTime(seconds float64) time.Time {
	return time.Unix(int64(min(max(seconds, 0), 1e12)), 0)
}

// stringList reads a claim holding a string or an array of strings.
func stringList(raw json.RawMessage) []string {
	if raw == nil {
		return nil
	}
	var single string
	if json.Unmarshal(raw, &single) == nil {
		raw, _ = json.Marshal([]string{single})
	}
	var items []any
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var out []string
	for _, item := range items {
		if s, ok := item.(string); ok && s != "" && len(s) <= maxClaimLength && len(out) < maxClaimItems {
			out = append(out, s)
		}
	}
	return out
}

// printable bounds identity-provider text to limit bytes of valid UTF-8
// without control characters.
func printable(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	if len(s) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut]
	}
	return s
}
