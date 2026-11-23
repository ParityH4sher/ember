package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Options replaces dependencies in tests. The zero value is production-ready.
type Options struct {
	Client *http.Client
	Now    func() time.Time
	Logger *slog.Logger
	// Redis, when set, keeps sessions in Redis instead of process memory.
	// It is ignored while sign-in is disabled.
	Redis *RedisConfig
}

// Authenticator serves the /v1/auth routes and guards the rest of the API.
// With a nil Config, sign-in is disabled and every request passes through.
type Authenticator struct {
	cfg      *Config
	provider *provider
	store    sessionStore
	redis    *redisClient
	now      func() time.Time
	logger   *slog.Logger

	sessionCookie string
	loginCookie   string
}

func New(cfg *Config, opts Options) *Authenticator {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	a := &Authenticator{cfg: cfg, now: opts.Now, logger: opts.Logger}
	if cfg == nil {
		return a
	}
	if opts.Client == nil {
		opts.Client = &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	if len(cfg.AdminGroups) == 0 {
		opts.Logger.Warn("sign-in has no adminGroups: signed-in users cannot configure or activate plugins")
	}
	a.provider = &provider{cfg: cfg, client: opts.Client, now: opts.Now, logger: opts.Logger}
	if opts.Redis != nil {
		a.redis = newRedisClient(opts.Redis.Addr, opts.Redis.password)
		a.store = &redisStore{client: a.redis, now: opts.Now, logger: opts.Logger}
	} else {
		a.store = newMemoryStore(opts.Now)
	}
	// The __Host- prefix pins cookies to this exact origin; browsers require Secure for it.
	prefix := ""
	if cfg.secure {
		prefix = "__Host-"
	}
	a.sessionCookie, a.loginCookie = prefix+"ember_session", prefix+"ember_login"
	return a
}

// Mode names the configured sign-in method for startup logs.
func (a *Authenticator) Mode() string {
	if a.cfg == nil {
		return "none"
	}
	return "oidc"
}

// SessionStore names where sessions are kept, for startup logs.
func (a *Authenticator) SessionStore() string {
	switch {
	case a.cfg == nil:
		return "none"
	case a.redis != nil:
		return "redis"
	}
	return "memory"
}

// CheckSessionStore reports whether the session store answers. The memory
// store always does. The API starts even when Redis does not answer: signed-in
// access then fails closed until it does.
func (a *Authenticator) CheckSessionStore(ctx context.Context) error {
	if a.redis == nil {
		return nil
	}
	return a.redis.ping(ctx)
}

// Close releases idle connections to the session store.
func (a *Authenticator) Close() {
	if a.redis != nil {
		a.redis.close()
	}
}

// Wrap serves the auth routes and, when sign-in is configured, requires a
// session for every other route except /healthz. It passes the caller's Role
// on in the request context; the API's routes decide which role they need.
func (a *Authenticator) Wrap(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/auth/session", a.handleSession)
	mux.HandleFunc("GET /v1/auth/login", a.handleLogin)
	mux.HandleFunc("GET /v1/auth/callback", a.handleCallback)
	mux.HandleFunc("POST /v1/auth/logout", a.handleLogout)
	mux.Handle("/", a.require(next))
	return mux
}

func (a *Authenticator) require(next http.Handler) http.Handler {
	if a.cfg == nil {
		// Without sign-in nobody can be told apart, so every caller is an
		// admin, as before roles existed. Keep such an API off untrusted networks.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveAs(next, w, r, RoleAdmin)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if !a.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "cross_origin_request", "cross-origin writes are not allowed")
			return
		}
		s, ok, err := a.current(w, r)
		if err != nil {
			a.storeUnavailable(w, err)
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "sign in to use Ember")
			return
		}
		if a.provider.available(r.Context()) != nil {
			writeError(w, http.StatusServiceUnavailable, "identity_provider_unavailable",
				"the sign-in service is unavailable; access is paused until it returns")
			return
		}
		serveAs(next, w, r, s.role)
	})
}

// serveAs passes the request on with the caller's role. The API's mux records
// the route it matched on the copy that WithContext makes; copy it back so the
// access log names that route rather than this mux's catch-all.
func serveAs(next http.Handler, w http.ResponseWriter, r *http.Request, role Role) {
	inner := r.WithContext(WithRole(r.Context(), role))
	next.ServeHTTP(w, inner)
	r.Pattern = inner.Pattern
}

// sameOrigin rejects cross-site state-changing requests, which a session
// cookie would otherwise authenticate. Requests without browser metadata,
// such as curl, have no ambient cookie and pass.
func (a *Authenticator) sameOrigin(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
	default:
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || origin == a.cfg.origin
}

// current returns the caller's session. A store error is neither signed in nor
// signed out: the cookie is kept, and the caller must answer 503.
func (a *Authenticator) current(w http.ResponseWriter, r *http.Request) (session, bool, error) {
	c, err := r.Cookie(a.sessionCookie)
	if err != nil {
		return session{}, false, nil
	}
	s, ok, err := a.store.session(r.Context(), c.Value)
	if err != nil {
		return session{}, false, err
	}
	if !ok {
		// Expired, signed out, or lost when an API without Redis restarted.
		http.SetCookie(w, a.cookie(a.sessionCookie, "", -1))
	}
	return s, ok, nil
}

// storeUnavailable answers when the session store fails. Like an identity
// provider outage, it pauses sign-in and signed-in access (fail closed).
func (a *Authenticator) storeUnavailable(w http.ResponseWriter, err error) {
	a.logger.Warn("session store unavailable", "error", err)
	writeError(w, http.StatusServiceUnavailable, "session_store_unavailable",
		"the session store is unavailable; sign-in is paused until it returns")
}

// providerInfo tells the web app which identity provider it signs in with.
type providerInfo struct {
	Name       string `json:"name,omitempty"`
	AccountURL string `json:"accountUrl,omitempty"`
}

func (a *Authenticator) handleSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if a.cfg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "authenticated": false})
		return
	}
	s, ok, err := a.current(w, r)
	if err != nil {
		a.storeUnavailable(w, err)
		return
	}
	body := map[string]any{"enabled": true, "authenticated": ok}
	if a.cfg.ProviderName != "" || a.cfg.AccountURL != "" {
		body["provider"] = providerInfo{Name: a.cfg.ProviderName, AccountURL: a.cfg.AccountURL}
	}
	if ok {
		body["user"] = s.user
		body["role"] = s.role.String()
		body["expiresAt"] = s.expires.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, body)
}

func (a *Authenticator) handleLogin(w http.ResponseWriter, r *http.Request) {
	if a.cfg == nil {
		notConfigured(w)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	// Block new sign-ins while the provider is down (DEC-002), even with cached metadata.
	if a.provider.available(ctx) != nil {
		writeError(w, http.StatusServiceUnavailable, "identity_provider_unavailable", "the sign-in service is unavailable")
		return
	}
	m, err := a.provider.discover(ctx)
	if err != nil {
		a.logger.Warn("identity provider discovery failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "identity_provider_unavailable", "the sign-in service is unavailable")
		return
	}
	state, nonce, verifier := randomToken(), randomToken(), randomToken()
	if err := a.store.addPending(ctx, state, pending{
		nonce:    nonce,
		verifier: verifier,
		returnTo: safeReturnTo(r.URL.Query().Get("returnTo")),
		expires:  a.now().Add(pendingTTL),
	}); err != nil {
		a.storeUnavailable(w, err)
		return
	}
	http.SetCookie(w, a.cookie(a.loginCookie, state, int(pendingTTL.Seconds())))

	target, err := url.Parse(m.AuthorizationEndpoint)
	if err != nil {
		writeError(w, http.StatusBadGateway, "identity_provider_misconfigured", "the sign-in service advertised an invalid endpoint")
		return
	}
	challenge := sha256.Sum256([]byte(verifier))
	query := target.Query()
	query.Set("response_type", "code")
	query.Set("client_id", a.cfg.ClientID)
	query.Set("redirect_uri", a.cfg.RedirectURL)
	query.Set("scope", strings.Join(a.cfg.Scopes, " "))
	query.Set("state", state)
	query.Set("nonce", nonce)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("code_challenge_method", "S256")
	target.RawQuery = query.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (a *Authenticator) handleCallback(w http.ResponseWriter, r *http.Request) {
	if a.cfg == nil {
		notConfigured(w)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, a.cookie(a.loginCookie, "", -1))
	query := r.URL.Query()
	state := query.Get("state")
	c, err := r.Cookie(a.loginCookie)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 {
		writeError(w, http.StatusBadRequest, "invalid_login_state", "sign-in expired or was started elsewhere; start again")
		return
	}
	attempt, ok, err := a.store.takePending(r.Context(), state)
	if err != nil {
		a.storeUnavailable(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_login_state", "sign-in expired or was started elsewhere; start again")
		return
	}
	if refusal := query.Get("error"); refusal != "" {
		a.logger.Info("identity provider refused sign-in", "error", printable(refusal, 64))
		writeError(w, http.StatusUnauthorized, "login_failed", "the identity provider did not complete sign-in")
		return
	}
	code := query.Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "login_failed", "the identity provider returned no authorization code")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	m, err := a.provider.discover(ctx)
	if err != nil {
		a.logger.Warn("identity provider discovery failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "identity_provider_unavailable", "the sign-in service is unavailable")
		return
	}
	rawIDToken, err := a.provider.exchange(ctx, m, code, attempt.verifier)
	if err != nil {
		a.logger.Warn("authorization code exchange failed", "error", err)
		writeError(w, http.StatusBadGateway, "login_failed", "could not complete sign-in with the identity provider")
		return
	}
	user, err := a.provider.verifyIDToken(ctx, m, rawIDToken, attempt.nonce)
	if err != nil {
		a.logger.Warn("ID token rejected", "error", err)
		writeError(w, http.StatusUnauthorized, "login_failed", "the identity provider returned an invalid ID token")
		return
	}
	role := a.roleFor(user)
	if role == RoleNone {
		a.logger.Info("sign-in denied by group policy", "subject", user.Subject)
		writeError(w, http.StatusForbidden, "access_denied", "your account is not in a group allowed to use Ember")
		return
	}

	// Never reuse a session ID across sign-ins (session fixation).
	if old, err := r.Cookie(a.sessionCookie); err == nil {
		if _, _, err := a.store.removeSession(ctx, old.Value); err != nil {
			a.storeUnavailable(w, err)
			return
		}
	}
	id := randomToken()
	err = a.store.addSession(ctx, id, session{user: user, role: role, idToken: rawIDToken, expires: a.now().Add(a.cfg.sessionTTL)})
	if errors.Is(err, errTooManySessions) {
		writeError(w, http.StatusServiceUnavailable, "too_many_sessions", "too many active sessions; try again later")
		return
	}
	if err != nil {
		a.storeUnavailable(w, err)
		return
	}
	http.SetCookie(w, a.cookie(a.sessionCookie, id, int(a.cfg.sessionTTL.Seconds())))
	a.logger.Info("user signed in", "subject", user.Subject, "role", role.String())
	http.Redirect(w, r, attempt.returnTo, http.StatusSeeOther)
}

func (a *Authenticator) handleLogout(w http.ResponseWriter, r *http.Request) {
	if a.cfg == nil {
		notConfigured(w)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if !a.sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross_origin_request", "cross-origin writes are not allowed")
		return
	}
	result := map[string]string{}
	if c, err := r.Cookie(a.sessionCookie); err == nil {
		s, ok, err := a.store.removeSession(r.Context(), c.Value)
		if err != nil {
			// Keep the cookie: the session still exists, so let the person retry.
			a.storeUnavailable(w, err)
			return
		}
		if ok {
			a.logger.Info("user signed out", "subject", s.user.Subject)
			if m := a.provider.cachedMetadata(); m != nil && m.EndSessionEndpoint != "" {
				if target := a.endSessionURL(m.EndSessionEndpoint, s.idToken); target != "" {
					result["endSessionUrl"] = target
				}
			}
		}
	}
	http.SetCookie(w, a.cookie(a.sessionCookie, "", -1))
	writeJSON(w, http.StatusOK, result)
}

// endSessionURL builds an OpenID Connect RP-initiated logout request.
func (a *Authenticator) endSessionURL(endpoint, idToken string) string {
	target, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	query := target.Query()
	query.Set("client_id", a.cfg.ClientID)
	query.Set("id_token_hint", idToken)
	if a.cfg.PostLogoutRedirectURL != "" {
		query.Set("post_logout_redirect_uri", a.cfg.PostLogoutRedirectURL)
	}
	target.RawQuery = query.Encode()
	return target.String()
}

func (a *Authenticator) cookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   a.cfg.secure,
		// Lax, not Strict: the provider's redirect back is a cross-site navigation.
		SameSite: http.SameSiteLaxMode,
	}
}

// safeReturnTo accepts only a local absolute path, preventing open redirects.
func safeReturnTo(target string) string {
	if target == "" || len(target) > 2048 || !strings.HasPrefix(target, "/") ||
		strings.HasPrefix(target, "//") || strings.ContainsRune(target, '\\') {
		return "/"
	}
	for _, r := range target {
		if r < 0x20 || r == 0x7f {
			return "/"
		}
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return "/"
	}
	return target
}

func notConfigured(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "auth_not_configured", "sign-in is not configured on this Ember instance")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}
