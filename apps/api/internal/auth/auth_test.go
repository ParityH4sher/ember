package auth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testClientID = "ember"
	testSecret   = "s3cr:t+/" // characters that must be form-encoded for Basic auth
	testRedirect = "https://ember.test/api/v1/auth/callback"
	testOrigin   = "https://ember.test"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type grant struct{ nonce, challenge, redirect string }

// fakeIdP is a minimal OpenID provider for the authorization code flow.
type fakeIdP struct {
	t     *testing.T
	srv   *httptest.Server
	clock *clock
	down  atomic.Bool

	mu       sync.Mutex
	key      *rsa.PrivateKey
	kid      string
	grants   map[string]grant
	tamper   func(claims map[string]any)
	override func(claims map[string]any) string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	idp := &fakeIdP{
		t:      t,
		clock:  &clock{t: time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)},
		key:    newRSAKey(t),
		kid:    "key-1",
		grants: map[string]grant{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", idp.discovery)
	mux.HandleFunc("GET /jwks", idp.jwks)
	mux.HandleFunc("POST /token", idp.token)
	idp.srv = httptest.NewTLSServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func (idp *fakeIdP) discovery(w http.ResponseWriter, _ *http.Request) {
	if idp.down.Load() {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	base := idp.srv.URL
	writeJSON(w, http.StatusOK, map[string]string{
		"issuer":                 base,
		"authorization_endpoint": base + "/authorize?tenant=t1",
		"token_endpoint":         base + "/token",
		"jwks_uri":               base + "/jwks",
		"end_session_endpoint":   base + "/logout",
	})
}

func (idp *fakeIdP) jwks(w http.ResponseWriter, _ *http.Request) {
	idp.mu.Lock()
	defer idp.mu.Unlock()
	pub := idp.key.PublicKey
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{
		{"kty": "oct", "kid": "ignored", "k": "c2VjcmV0"},
		{
			"kty": "RSA", "kid": idp.kid, "use": "sig", "alg": "RS256",
			"n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes()),
		},
	}})
}

func (idp *fakeIdP) token(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	user, _ = url.QueryUnescape(user)
	pass, _ = url.QueryUnescape(pass)
	if !ok || user != testClientID || pass != testSecret {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != "authorization_code" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	idp.mu.Lock()
	g, found := idp.grants[r.PostForm.Get("code")]
	delete(idp.grants, r.PostForm.Get("code"))
	tamper, override, key, kid := idp.tamper, idp.override, idp.key, idp.kid
	idp.mu.Unlock()
	verifier := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !found || r.PostForm.Get("redirect_uri") != g.redirect || b64(verifier[:]) != g.challenge {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	now := idp.clock.Now()
	claims := map[string]any{
		"iss": idp.srv.URL, "sub": "user-1", "aud": testClientID,
		"exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(), "nonce": g.nonce,
		"name": "Ada Lovelace", "email": "ada@example.test", "groups": []string{"makers"},
	}
	if tamper != nil {
		tamper(claims)
	}
	token := signJWT(idp.t, "RS256", kid, key, claims)
	if override != nil {
		token = override(claims)
	}
	writeJSON(w, http.StatusOK, map[string]string{"id_token": token, "access_token": "unused", "token_type": "Bearer"})
}

// authorize stands in for the user approving sign-in at the provider.
func (idp *fakeIdP) authorize(q url.Values) string {
	code := randomToken()
	idp.mu.Lock()
	defer idp.mu.Unlock()
	idp.grants[code] = grant{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri")}
	return code
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func signJWT(t *testing.T, alg, kid string, key crypto.Signer, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": alg, "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	h, _ := json.Marshal(header)
	p, _ := json.Marshal(claims)
	input := b64(h) + "." + b64(p)
	digest := sha256.Sum256([]byte(input))
	var sig []byte
	var err error
	switch k := key.(type) {
	case *rsa.PrivateKey:
		sig, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, digest[:])
	case *ecdsa.PrivateKey:
		var r, s *big.Int
		r, s, err = ecdsa.Sign(rand.Reader, k, digest[:])
		if err == nil {
			sig = append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + b64(sig)
}

func testConfig(t *testing.T, idp *fakeIdP) *Config {
	t.Helper()
	raw := fmt.Sprintf(`{"issuer":%q,"clientId":%q,"redirectUrl":%q,"postLogoutRedirectUrl":"https://ember.test/"}`,
		idp.srv.URL, testClientID, testRedirect)
	cfg, err := ParseConfig([]byte(raw), testSecret)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

var protected = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

func newTestHandler(t *testing.T, idp *fakeIdP, cfg *Config) http.Handler {
	t.Helper()
	return New(cfg, Options{Client: idp.srv.Client(), Now: idp.clock.Now}).Wrap(protected)
}

func do(h http.Handler, method, target string, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func responseCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct{ Code string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body %q: %v", rec.Body.String(), err)
	}
	return body.Code
}

// startLogin returns the authorization request parameters and login cookie.
func startLogin(t *testing.T, h http.Handler, returnTo string) (url.Values, *http.Cookie) {
	t.Helper()
	rec := do(h, http.MethodGet, "/v1/auth/login?"+url.Values{"returnTo": {returnTo}}.Encode(), nil, nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	login := responseCookie(rec, "__Host-ember_login")
	if login == nil {
		t.Fatal("login did not set the state cookie")
	}
	return loc.Query(), login
}

func callback(h http.Handler, code, state string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return do(h, http.MethodGet, "/v1/auth/callback?"+url.Values{"code": {code}, "state": {state}}.Encode(), cookies, nil)
}

func signIn(t *testing.T, h http.Handler, idp *fakeIdP) *http.Cookie {
	t.Helper()
	q, login := startLogin(t, h, "/apps/notes/?project=p")
	rec := callback(h, idp.authorize(q), q.Get("state"), login)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/apps/notes/?project=p" {
		t.Fatalf("callback status=%d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	session := responseCookie(rec, "__Host-ember_session")
	if session == nil {
		t.Fatal("callback did not set a session cookie")
	}
	return session
}

func TestDisabledAuthPassesThrough(t *testing.T) {
	h := New(nil, Options{}).Wrap(protected)
	if rec := do(h, http.MethodPost, "/v1/projects", nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusNoContent {
		t.Fatalf("protected status=%d", rec.Code)
	}
	rec := do(h, http.MethodGet, "/v1/auth/session", nil, nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"authenticated":false,"enabled":false}` {
		t.Fatalf("session status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(h, http.MethodGet, "/v1/auth/login", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("login status=%d", rec.Code)
	}
}

func TestSignInFlow(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, idp, testConfig(t, idp))

	if rec := do(h, http.MethodGet, "/v1/projects", nil, nil); rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "unauthenticated" {
		t.Fatalf("anonymous status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(h, http.MethodGet, "/healthz", nil, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("health must stay public, status=%d", rec.Code)
	}

	q, _ := startLogin(t, h, "/")
	for key, want := range map[string]string{
		"tenant": "t1", "response_type": "code", "client_id": testClientID, "redirect_uri": testRedirect,
		"scope": "openid profile email", "code_challenge_method": "S256",
	} {
		if got := q.Get(key); got != want {
			t.Errorf("authorization %s=%q want %q", key, got, want)
		}
	}
	for _, key := range []string{"state", "nonce", "code_challenge"} {
		if len(q.Get(key)) < 43 {
			t.Errorf("authorization %s=%q is too short", key, q.Get(key))
		}
	}

	session := signIn(t, h, idp)
	if !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteLaxMode || session.Path != "/" {
		t.Fatalf("session cookie attributes: %+v", session)
	}
	if rec := do(h, http.MethodGet, "/v1/projects", []*http.Cookie{session}, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("signed-in status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec := do(h, http.MethodGet, "/v1/auth/session", []*http.Cookie{session}, nil)
	var body struct {
		Enabled, Authenticated bool
		User                   User
		ExpiresAt              string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Enabled || !body.Authenticated || body.User.Subject != "user-1" || body.User.Name != "Ada Lovelace" ||
		body.User.Email != "ada@example.test" || len(body.User.Groups) != 1 || body.ExpiresAt != "2026-11-21T20:00:00Z" {
		t.Fatalf("session body=%s", rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("session response must not be cached")
	}
}

func TestCallbackRequiresMatchingSingleUseState(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, idp, testConfig(t, idp))
	q, login := startLogin(t, h, "/")
	code := idp.authorize(q)

	if rec := callback(h, code, q.Get("state")); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing login cookie status=%d", rec.Code)
	}
	if rec := callback(h, code, "forged", login); rec.Code != http.StatusBadRequest {
		t.Fatalf("forged state status=%d", rec.Code)
	}
	if rec := callback(h, code, q.Get("state"), login); rec.Code != http.StatusSeeOther {
		t.Fatalf("valid callback status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := callback(h, code, q.Get("state"), login); rec.Code != http.StatusBadRequest || responseCookie(rec, "__Host-ember_session") != nil {
		t.Fatalf("replayed callback status=%d", rec.Code)
	}
}

func TestCallbackRejectsInvalidIDTokens(t *testing.T) {
	other := newRSAKey(t)
	cases := map[string]struct {
		tamper   func(map[string]any)
		override func(*fakeIdP, map[string]any) string
	}{
		"wrong audience":              {tamper: func(c map[string]any) { c["aud"] = "other-client" }},
		"wrong issuer":                {tamper: func(c map[string]any) { c["iss"] = "https://issuer.invalid" }},
		"expired":                     {tamper: func(c map[string]any) { c["exp"] = c["iat"].(int64) - 120 }},
		"issued in the future":        {tamper: func(c map[string]any) { c["iat"] = c["iat"].(int64) + 600 }},
		"missing issue time":          {tamper: func(c map[string]any) { delete(c, "iat") }},
		"wrong nonce":                 {tamper: func(c map[string]any) { c["nonce"] = "replayed" }},
		"foreign authorized party":    {tamper: func(c map[string]any) { c["azp"] = "other-client" }},
		"extra audience, no azp":      {tamper: func(c map[string]any) { c["aud"] = []string{testClientID, "other-client"} }},
		"empty subject":               {tamper: func(c map[string]any) { c["sub"] = "" }},
		"signed by an unknown key":    {override: func(idp *fakeIdP, c map[string]any) string { return signJWT(t, "RS256", idp.kid, other, c) }},
		"unsigned (alg none)":         {override: func(_ *fakeIdP, c map[string]any) string { return unsigned(c) }},
		"HMAC with the client secret": {override: func(_ *fakeIdP, c map[string]any) string { return hmacSigned(c) }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			idp := newFakeIdP(t)
			h := newTestHandler(t, idp, testConfig(t, idp))
			idp.tamper = tc.tamper
			if tc.override != nil {
				idp.override = func(c map[string]any) string { return tc.override(idp, c) }
			}
			q, login := startLogin(t, h, "/")
			rec := callback(h, idp.authorize(q), q.Get("state"), login)
			if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "login_failed" || responseCookie(rec, "__Host-ember_session") != nil {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func unsigned(claims map[string]any) string {
	h, _ := json.Marshal(map[string]string{"alg": "none"})
	p, _ := json.Marshal(claims)
	return b64(h) + "." + b64(p) + "."
}

func hmacSigned(claims map[string]any) string {
	h, _ := json.Marshal(map[string]string{"alg": "HS256", "kid": "key-1"})
	p, _ := json.Marshal(claims)
	input := b64(h) + "." + b64(p)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(input))
	return input + "." + b64(mac.Sum(nil))
}

func TestAllowedGroups(t *testing.T) {
	idp := newFakeIdP(t)
	cfg := testConfig(t, idp)
	cfg.AllowedGroups = []string{"admins"}
	h := newTestHandler(t, idp, cfg)
	q, login := startLogin(t, h, "/")
	if rec := callback(h, idp.authorize(q), q.Get("state"), login); rec.Code != http.StatusForbidden || errorCode(t, rec) != "access_denied" {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	cfg.AllowedGroups = []string{"admins", "makers"}
	signIn(t, h, idp)
}

func TestCrossOriginWritesAreRejected(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, idp, testConfig(t, idp))
	session := []*http.Cookie{signIn(t, h, idp)}
	for _, tc := range []struct {
		headers map[string]string
		want    int
	}{
		{map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{map[string]string{"Origin": "https://attacker.test"}, http.StatusForbidden},
		{map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": testOrigin}, http.StatusNoContent},
		{map[string]string{"Origin": testOrigin}, http.StatusNoContent},
		{nil, http.StatusNoContent},
	} {
		if rec := do(h, http.MethodPut, "/v1/projects/p/notes", session, tc.headers); rec.Code != tc.want {
			t.Errorf("headers=%v status=%d want %d", tc.headers, rec.Code, tc.want)
		}
	}
	if rec := do(h, http.MethodPost, "/v1/auth/logout", session, map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site logout status=%d", rec.Code)
	}
}

func TestSignOut(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, idp, testConfig(t, idp))
	session := signIn(t, h, idp)
	rec := do(h, http.MethodPost, "/v1/auth/logout", []*http.Cookie{session}, map[string]string{"Sec-Fetch-Site": "same-origin"})
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", rec.Code, rec.Body.String())
	}
	if c := responseCookie(rec, "__Host-ember_session"); c == nil || c.MaxAge >= 0 {
		t.Fatalf("logout must clear the session cookie: %+v", c)
	}
	var body struct{ EndSessionURL string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	end, err := url.Parse(body.EndSessionURL)
	if err != nil || !strings.HasPrefix(body.EndSessionURL, idp.srv.URL+"/logout?") ||
		end.Query().Get("client_id") != testClientID || end.Query().Get("id_token_hint") == "" ||
		end.Query().Get("post_logout_redirect_uri") != "https://ember.test/" {
		t.Fatalf("endSessionUrl=%q", body.EndSessionURL)
	}
	if rec := do(h, http.MethodGet, "/v1/projects", []*http.Cookie{session}, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("signed-out session still works, status=%d", rec.Code)
	}
}

func TestUnknownSessionCookieIsCleared(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, idp, testConfig(t, idp))
	rec := do(h, http.MethodGet, "/v1/projects", []*http.Cookie{{Name: "__Host-ember_session", Value: "lost-on-restart"}}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
	if c := responseCookie(rec, "__Host-ember_session"); c == nil || c.MaxAge >= 0 {
		t.Fatalf("stale cookie not cleared: %+v", c)
	}
}

func TestSessionsExpire(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, idp, testConfig(t, idp))
	session := signIn(t, h, idp)
	idp.clock.Add(8 * time.Hour)
	if rec := do(h, http.MethodGet, "/v1/projects", []*http.Cookie{session}, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session status=%d", rec.Code)
	}
}

func TestProviderOutagePausesSessionsAndSignIn(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, idp, testConfig(t, idp))
	session := []*http.Cookie{signIn(t, h, idp)}
	if rec := do(h, http.MethodGet, "/v1/projects", session, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rec.Code)
	}

	idp.down.Store(true)
	idp.clock.Add(availabilityTTL)
	if rec := do(h, http.MethodGet, "/v1/projects", session, nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("outage status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(h, http.MethodGet, "/v1/auth/login", nil, nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sign-in during outage status=%d", rec.Code)
	}
	if rec := do(h, http.MethodGet, "/healthz", nil, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("health during outage status=%d", rec.Code)
	}

	idp.down.Store(false)
	idp.clock.Add(availabilityTTL)
	if rec := do(h, http.MethodGet, "/v1/projects", session, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("recovered status=%d", rec.Code)
	}
}

func TestSigningKeyRotationIsRateLimited(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, idp, testConfig(t, idp))
	signIn(t, h, idp)

	idp.mu.Lock()
	idp.key, idp.kid = newRSAKey(t), "key-2"
	idp.mu.Unlock()
	q, login := startLogin(t, h, "/")
	if rec := callback(h, idp.authorize(q), q.Get("state"), login); rec.Code != http.StatusUnauthorized {
		t.Fatalf("JWKS refetched too soon, status=%d", rec.Code)
	}
	idp.clock.Add(keyRefetchInterval)
	signIn(t, h, idp)
}

func TestES256Signatures(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := parseKeys([]jwk{{
		Kty: "EC", Kid: "ec", Crv: "P-256",
		X: b64(key.X.FillBytes(make([]byte, 32))), Y: b64(key.Y.FillBytes(make([]byte, 32))),
	}})
	if len(keys) != 1 {
		t.Fatalf("parsed %d keys", len(keys))
	}
	token := signJWT(t, "ES256", "ec", key, map[string]any{"sub": "x"})
	header, _, err := parseJWT(token)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifySignature(token, header, keys); err != nil {
		t.Fatal(err)
	}
	header.Alg = "ES384" // a P-256 key must not verify another algorithm
	if verifySignature(token, header, keys) == nil {
		t.Fatal("curve/algorithm mismatch accepted")
	}
	offCurve := jwk{Kty: "EC", Crv: "P-256", X: b64(make([]byte, 32)), Y: b64(make([]byte, 32))}
	if len(parseKeys([]jwk{offCurve})) != 0 {
		t.Fatal("point not on the curve accepted")
	}
	weakRSA := jwk{Kty: "RSA", N: b64(big.NewInt(3233).Bytes()), E: "AQAB"}
	if len(parseKeys([]jwk{weakRSA})) != 0 {
		t.Fatal("RSA key below 2048 bits accepted")
	}
}

func TestParseConfig(t *testing.T) {
	valid := `{"issuer":"https://auth.example/application/o/ember/","clientId":"ember","redirectUrl":"https://ember.example/api/v1/auth/callback"}`
	cfg, err := ParseConfig([]byte(valid), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenEndpointAuthMethod != methodBasic || cfg.GroupsClaim != "groups" || cfg.sessionTTL != 8*time.Hour ||
		strings.Join(cfg.Scopes, " ") != "openid profile email" || !cfg.secure || cfg.origin != "https://ember.example" {
		t.Fatalf("defaults: %+v", cfg)
	}
	public, err := ParseConfig([]byte(`{"issuer":"http://127.0.0.1:5556/dex","clientId":"ember","redirectUrl":"http://localhost:5173/api/v1/auth/callback"}`), "")
	if err != nil || public.TokenEndpointAuthMethod != methodNone || public.secure {
		t.Fatalf("loopback public client: %+v, %v", public, err)
	}

	for name, tc := range map[string]struct{ raw, secret string }{
		"http issuer off loopback": {`{"issuer":"http://auth.example","clientId":"ember","redirectUrl":"https://ember.example/cb"}`, "s"},
		"credentials in issuer":    {`{"issuer":"https://u:p@auth.example","clientId":"ember","redirectUrl":"https://ember.example/cb"}`, "s"},
		"missing client id":        {`{"issuer":"https://auth.example","redirectUrl":"https://ember.example/cb"}`, "s"},
		"relative redirect":        {`{"issuer":"https://auth.example","clientId":"ember","redirectUrl":"/api/v1/auth/callback"}`, "s"},
		"no openid scope":          {`{"issuer":"https://auth.example","clientId":"ember","redirectUrl":"https://ember.example/cb","scopes":["profile"]}`, "s"},
		"scope with space":         {`{"issuer":"https://auth.example","clientId":"ember","redirectUrl":"https://ember.example/cb","scopes":["openid email"]}`, "s"},
		"post without secret":      {`{"issuer":"https://auth.example","clientId":"ember","redirectUrl":"https://ember.example/cb","tokenEndpointAuthMethod":"client_secret_post"}`, ""},
		"none with secret":         {`{"issuer":"https://auth.example","clientId":"ember","redirectUrl":"https://ember.example/cb","tokenEndpointAuthMethod":"none"}`, "s"},
		"unknown method":           {`{"issuer":"https://auth.example","clientId":"ember","redirectUrl":"https://ember.example/cb","tokenEndpointAuthMethod":"private_key_jwt"}`, "s"},
		"short session":            {`{"issuer":"https://auth.example","clientId":"ember","redirectUrl":"https://ember.example/cb","sessionTtl":"1m"}`, "s"},
		"unknown field":            {`{"issuer":"https://auth.example","clientId":"ember","redirectUrl":"https://ember.example/cb","clientSecret":"inline"}`, "s"},
		"trailing data":            {valid + `{}`, "s"},
	} {
		if _, err := ParseConfig([]byte(tc.raw), tc.secret); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	if cfg, err := LoadConfig("", ""); cfg != nil || err != nil {
		t.Fatalf("unconfigured: %v, %v", cfg, err)
	}
	if _, err := LoadConfig("", "/run/secrets/x"); err == nil {
		t.Fatal("secret without configuration accepted")
	}
	dir := t.TempDir()
	configFile := filepath.Join(dir, "auth.json")
	secretFile := filepath.Join(dir, "secret")
	if err := os.WriteFile(configFile, []byte(`{"issuer":"https://auth.example","clientId":"ember","redirectUrl":"https://ember.example/cb"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretFile, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(configFile, secretFile)
	if err != nil || cfg.clientSecret != "from-file" {
		t.Fatalf("load: %v", err)
	}
	if err := os.WriteFile(secretFile, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(configFile, secretFile); err == nil {
		t.Fatal("empty secret accepted")
	}
}

func TestSafeReturnTo(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "/",
		"/apps/notes/?project=p": "/apps/notes/?project=p",
		"https://attacker.test/": "/",
		"//attacker.test/":       "/",
		"/\\attacker.test":       "/",
		"apps/notes/":            "/",
		"/a\r\nSet-Cookie: x":    "/",
	} {
		if got := safeReturnTo(in); got != want {
			t.Errorf("safeReturnTo(%q)=%q want %q", in, got, want)
		}
	}
}
