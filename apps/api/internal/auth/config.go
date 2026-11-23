// Package auth adds optional OpenID Connect sign-in in front of the Ember API.
//
// The API is the OAuth 2.0 client: it runs the authorization code flow with
// PKCE, validates the ID token, and keeps the session on the server. Browsers
// receive only an opaque, HttpOnly session cookie, never an IdP token.
package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	methodBasic = "client_secret_basic"
	methodPost  = "client_secret_post"
	methodNone  = "none"

	maxConfigBytes      = 64 << 10
	maxSecretBytes      = 4 << 10
	maxProviderNameRune = 64
)

// Config is the operator's OpenID Connect client configuration. It is read
// from a JSON file; the client secret is read from a separate file.
type Config struct {
	// Issuer must exactly match the issuer in the provider's discovery document.
	Issuer   string `json:"issuer"`
	ClientID string `json:"clientId"`
	// RedirectURL is the browser-visible callback, including the gateway's
	// /api prefix, for example https://ember.example/api/v1/auth/callback.
	RedirectURL           string   `json:"redirectUrl"`
	PostLogoutRedirectURL string   `json:"postLogoutRedirectUrl,omitempty"`
	Scopes                []string `json:"scopes,omitempty"`
	// TokenEndpointAuthMethod is client_secret_basic, client_secret_post or
	// none. It defaults to client_secret_basic with a secret and none without.
	TokenEndpointAuthMethod string `json:"tokenEndpointAuthMethod,omitempty"`
	// GroupsClaim names the ID token claim holding group names.
	GroupsClaim string `json:"groupsClaim,omitempty"`
	// AllowedGroups, when set, admits only users in at least one listed group
	// (or in AdminGroups). Admitted users get the user role.
	AllowedGroups []string `json:"allowedGroups,omitempty"`
	// AdminGroups gives members the admin role, which may change plugins.
	// Without it, nobody signed in can change plugins.
	AdminGroups []string `json:"adminGroups,omitempty"`
	// SessionTTL is a Go duration such as "8h"; sessions do not renew.
	SessionTTL string `json:"sessionTtl,omitempty"`
	// ProviderName is the identity provider's display name, such as
	// "Authentik", which the web app shows on its sign-in button.
	ProviderName string `json:"providerName,omitempty"`
	// AccountURL is where people manage their own account at the provider.
	AccountURL string `json:"accountUrl,omitempty"`

	clientSecret string
	sessionTTL   time.Duration
	origin       string
	secure       bool
}

// LoadConfig reads the configuration named by EMBER_AUTH_CONFIG_FILE and the
// secret named by EMBER_OIDC_CLIENT_SECRET_FILE. Without a configuration file
// sign-in is disabled and LoadConfig returns nil.
func LoadConfig(configFile, secretFile string) (*Config, error) {
	if configFile == "" {
		if secretFile != "" {
			return nil, errors.New("EMBER_OIDC_CLIENT_SECRET_FILE is set but EMBER_AUTH_CONFIG_FILE is not")
		}
		return nil, nil
	}
	raw, err := readSmallFile(configFile, maxConfigBytes)
	if err != nil {
		return nil, fmt.Errorf("read auth configuration: %w", err)
	}
	secret := ""
	if secretFile != "" {
		data, err := readSmallFile(secretFile, maxSecretBytes)
		if err != nil {
			return nil, fmt.Errorf("read OIDC client secret: %w", err)
		}
		// Editors commonly add a trailing newline; secrets never contain one.
		if secret = strings.TrimSpace(string(data)); secret == "" {
			return nil, errors.New("OIDC client secret file is empty")
		}
	}
	return ParseConfig(raw, secret)
}

func readSmallFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, limit)
	}
	return data, nil
}

// ParseConfig validates a JSON configuration document and applies defaults.
func ParseConfig(raw []byte, clientSecret string) (*Config, error) {
	var c Config
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return nil, fmt.Errorf("auth configuration: %w", err)
	}
	if decoder.More() {
		return nil, errors.New("auth configuration must contain one JSON object")
	}
	c.clientSecret = clientSecret
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("auth configuration: %w", err)
	}
	return &c, nil
}

func (c *Config) validate() error {
	issuer, err := endpointURL(c.Issuer, "issuer")
	if err != nil {
		return err
	}
	if issuer.RawQuery != "" {
		return errors.New("issuer must not contain a query")
	}
	if strings.TrimSpace(c.ClientID) == "" {
		return errors.New("clientId is required")
	}
	redirect, err := endpointURL(c.RedirectURL, "redirectUrl")
	if err != nil {
		return err
	}
	if c.PostLogoutRedirectURL != "" {
		if _, err := endpointURL(c.PostLogoutRedirectURL, "postLogoutRedirectUrl"); err != nil {
			return err
		}
	}

	c.ProviderName = strings.TrimSpace(c.ProviderName)
	if utf8.RuneCountInString(c.ProviderName) > maxProviderNameRune {
		return fmt.Errorf("providerName must be at most %d characters", maxProviderNameRune)
	}
	for _, r := range c.ProviderName {
		if !unicode.IsPrint(r) {
			return errors.New("providerName must contain only printable characters")
		}
	}
	if c.AccountURL != "" {
		if _, err := endpointURL(c.AccountURL, "accountUrl"); err != nil {
			return err
		}
	}

	if len(c.Scopes) == 0 {
		c.Scopes = []string{"openid", "profile", "email"}
	}
	hasOpenID := false
	for _, scope := range c.Scopes {
		if !validScope(scope) {
			return fmt.Errorf("invalid scope %q", scope)
		}
		hasOpenID = hasOpenID || scope == "openid"
	}
	if !hasOpenID {
		return errors.New("scopes must include openid")
	}

	switch c.TokenEndpointAuthMethod {
	case "":
		c.TokenEndpointAuthMethod = methodNone
		if c.clientSecret != "" {
			c.TokenEndpointAuthMethod = methodBasic
		}
	case methodBasic, methodPost:
		if c.clientSecret == "" {
			return fmt.Errorf("%s requires EMBER_OIDC_CLIENT_SECRET_FILE", c.TokenEndpointAuthMethod)
		}
	case methodNone:
		if c.clientSecret != "" {
			return errors.New("tokenEndpointAuthMethod none must not have a client secret")
		}
	default:
		return fmt.Errorf("tokenEndpointAuthMethod must be %s, %s or %s", methodBasic, methodPost, methodNone)
	}

	if c.GroupsClaim == "" {
		c.GroupsClaim = "groups"
	}
	for _, group := range c.AllowedGroups {
		if strings.TrimSpace(group) == "" {
			return errors.New("allowedGroups must not contain empty names")
		}
	}
	for _, group := range c.AdminGroups {
		if strings.TrimSpace(group) == "" {
			return errors.New("adminGroups must not contain empty names")
		}
	}

	if c.SessionTTL == "" {
		c.SessionTTL = "8h"
	}
	if c.sessionTTL, err = time.ParseDuration(c.SessionTTL); err != nil ||
		c.sessionTTL < 5*time.Minute || c.sessionTTL > 30*24*time.Hour {
		return errors.New("sessionTtl must be a duration from 5m to 720h")
	}

	c.origin = redirect.Scheme + "://" + redirect.Host
	c.secure = redirect.Scheme == "https"
	return nil
}

// endpointURL accepts HTTPS URLs, and plain HTTP only for loopback development.
func endpointURL(raw, name string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("%s must be an absolute URL without credentials or fragment", name)
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && loopback(u.Hostname()):
	default:
		return nil, fmt.Errorf("%s must use https (http is allowed only for loopback development)", name)
	}
	return u, nil
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validScope follows RFC 6749 section 3.3: printable ASCII except space, " and \.
func validScope(scope string) bool {
	if scope == "" {
		return false
	}
	for _, r := range scope {
		if r < 0x21 || r > 0x7e || r == '"' || r == '\\' {
			return false
		}
	}
	return true
}
