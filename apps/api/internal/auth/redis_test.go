package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRedis is a minimal RESP2 server with the commands the session store
// uses. It expires keys on its own clock, so tests can share the API's clock.
type fakeRedis struct {
	t        *testing.T
	ln       net.Listener
	password string
	now      func() time.Time
	down     atomic.Bool // close every connection at once
	dials    atomic.Int32

	mu    sync.Mutex
	data  map[string]fakeEntry
	conns map[net.Conn]bool
	ttls  map[string]time.Duration // last PX per key
}

type fakeEntry struct {
	value   string
	expires time.Time
}

func newFakeRedis(t *testing.T, password string, now func() time.Time) *fakeRedis {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRedis{t: t, ln: ln, password: password, now: now,
		data: map[string]fakeEntry{}, conns: map[net.Conn]bool{}, ttls: map[string]time.Duration{}}
	go f.serve()
	t.Cleanup(func() {
		ln.Close()
		f.dropConnections()
	})
	return f
}

func (f *fakeRedis) addr() string { return f.ln.Addr().String() }

func (f *fakeRedis) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.dials.Add(1)
		if f.down.Load() {
			conn.Close()
			continue
		}
		f.mu.Lock()
		f.conns[conn] = true
		f.mu.Unlock()
		go f.handle(conn)
	}
}

// dropConnections simulates a Redis restart: open connections are closed.
func (f *fakeRedis) dropConnections() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for conn := range f.conns {
		conn.Close()
		delete(f.conns, conn)
	}
}

func (f *fakeRedis) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keys []string
	for k := range f.data {
		keys = append(keys, k)
	}
	return keys
}

func (f *fakeRedis) ttl(key string) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ttls[key]
}

func (f *fakeRedis) values() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b strings.Builder
	for _, e := range f.data {
		b.WriteString(e.value)
	}
	return b.String()
}

func (f *fakeRedis) handle(conn net.Conn) {
	defer conn.Close()
	r, w := bufio.NewReader(conn), bufio.NewWriter(conn)
	authed := f.password == ""
	for {
		args, err := readCommand(r)
		if err != nil {
			return
		}
		if f.down.Load() {
			return
		}
		reply := f.exec(args, &authed)
		if _, err := w.WriteString(reply); err != nil || w.Flush() != nil {
			return
		}
	}
}

func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "*") {
		return nil, errors.New("bad command")
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}
	args := make([]string, n)
	for i := range args {
		line, err := r.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "$") {
			return nil, errors.New("bad argument")
		}
		size, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args[i] = string(buf[:size])
	}
	return args, nil
}

func bulkReply(v string, ok bool) string {
	if !ok {
		return "$-1\r\n"
	}
	return fmt.Sprintf("$%d\r\n%s\r\n", len(v), v)
}

func (f *fakeRedis) exec(args []string, authed *bool) string {
	if len(args) == 0 {
		return "-ERR empty command\r\n"
	}
	cmd := strings.ToUpper(args[0])
	if cmd == "AUTH" {
		if len(args) == 2 && f.password != "" && args[1] == f.password {
			*authed = true
			return "+OK\r\n"
		}
		return "-WRONGPASS invalid username-password pair or user is disabled.\r\n"
	}
	if !*authed {
		return "-NOAUTH Authentication required.\r\n"
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	lookup := func(key string) (string, bool) {
		e, ok := f.data[key]
		if ok && !f.now().Before(e.expires) {
			delete(f.data, key)
			return "", false
		}
		return e.value, ok
	}
	switch {
	case cmd == "PING" && len(args) == 1:
		return "+PONG\r\n"
	case cmd == "SET" && len(args) == 5 && strings.EqualFold(args[3], "PX"):
		ms, err := strconv.ParseInt(args[4], 10, 64)
		if err != nil || ms <= 0 {
			return "-ERR invalid expire time in 'set' command\r\n"
		}
		ttl := time.Duration(ms) * time.Millisecond
		f.data[args[1]] = fakeEntry{value: args[2], expires: f.now().Add(ttl)}
		f.ttls[args[1]] = ttl
		return "+OK\r\n"
	case cmd == "GET" && len(args) == 2:
		if args[1] == "huge" {
			return fmt.Sprintf("$%d\r\n", redisMaxBulk+1)
		}
		return bulkReply(lookup(args[1]))
	case cmd == "GETDEL" && len(args) == 2:
		v, ok := lookup(args[1])
		delete(f.data, args[1])
		return bulkReply(v, ok)
	case cmd == "DEL" && len(args) == 2:
		_, ok := lookup(args[1])
		delete(f.data, args[1])
		if ok {
			return ":1\r\n"
		}
		return ":0\r\n"
	}
	return "-ERR unknown command\r\n"
}

func TestRedisClientCommands(t *testing.T) {
	clk := &clock{t: time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)}
	srv := newFakeRedis(t, "pw", clk.Now)
	c := newRedisClient(srv.addr(), "pw")
	defer c.close()
	ctx := context.Background()

	if err := c.ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.set(ctx, "k", "binary\r\nvalue", time.Minute); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := c.get(ctx, "k"); err != nil || !ok || v != "binary\r\nvalue" {
		t.Fatalf("get: %q %v %v", v, ok, err)
	}
	if _, ok, err := c.get(ctx, "missing"); err != nil || ok {
		t.Fatalf("missing key: %v %v", ok, err)
	}
	if v, ok, err := c.getDel(ctx, "k"); err != nil || !ok || v != "binary\r\nvalue" {
		t.Fatalf("getdel: %q %v %v", v, ok, err)
	}
	if _, ok, err := c.getDel(ctx, "k"); err != nil || ok {
		t.Fatalf("getdel is not single-use: %v %v", ok, err)
	}

	if err := c.set(ctx, "short", "v", 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if srv.ttl("short") != 1500*time.Millisecond {
		t.Fatalf("PX=%v", srv.ttl("short"))
	}
	clk.Add(2 * time.Second)
	if _, ok, err := c.get(ctx, "short"); err != nil || ok {
		t.Fatalf("expired key returned: %v %v", ok, err)
	}

	if err := c.set(ctx, "d", "v", time.Minute); err != nil {
		t.Fatal(err)
	}
	if existed, err := c.del(ctx, "d"); err != nil || !existed {
		t.Fatalf("del: %v %v", existed, err)
	}
	if existed, err := c.del(ctx, "d"); err != nil || existed {
		t.Fatalf("second del: %v %v", existed, err)
	}

	// An error reply leaves the connection usable.
	var replyErr redisError
	if _, err := c.do(ctx, "NOPE"); !errors.As(err, &replyErr) {
		t.Fatalf("error reply: %v", err)
	}
	// An oversized reply is refused without reading it.
	if _, _, err := c.get(ctx, "huge"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized reply: %v", err)
	}
	dials := srv.dials.Load()
	if err := c.ping(ctx); err != nil {
		t.Fatal(err)
	}
	if srv.dials.Load() != dials+1 {
		t.Fatal("the connection that received an oversized reply was reused")
	}
}

func TestRedisClientAuthentication(t *testing.T) {
	srv := newFakeRedis(t, "right", time.Now)
	for name, password := range map[string]string{"wrong password": "wrong", "no password": ""} {
		c := newRedisClient(srv.addr(), password)
		err := c.ping(context.Background())
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		if strings.Contains(err.Error(), "WRONGPASS") || strings.Contains(err.Error(), "wrong") {
			t.Fatalf("%s: error echoes the server: %v", name, err)
		}
	}
}

func TestRedisClientReconnects(t *testing.T) {
	srv := newFakeRedis(t, "", time.Now)
	c := newRedisClient(srv.addr(), "")
	ctx := context.Background()
	if err := c.ping(ctx); err != nil {
		t.Fatal(err)
	}
	// Redis restarted: the idle connection is dead, and one retry hides it.
	srv.dropConnections()
	if err := c.ping(ctx); err != nil {
		t.Fatalf("after restart: %v", err)
	}
	srv.down.Store(true)
	srv.dropConnections()
	if err := c.ping(ctx); err == nil {
		t.Fatal("ping succeeded while Redis was down")
	}
	srv.down.Store(false)
	if err := c.ping(ctx); err != nil {
		t.Fatalf("after recovery: %v", err)
	}
}

func TestLoadRedisConfig(t *testing.T) {
	if c, err := LoadRedisConfig("", ""); c != nil || err != nil {
		t.Fatalf("unset: %v %v", c, err)
	}
	if _, err := LoadRedisConfig("", "/run/secrets/x"); err == nil {
		t.Fatal("password file without address accepted")
	}
	for _, addr := range []string{"redis", "redis:", ":6379", "redis:0", "redis:65536", "redis:http", "redis://x:6379"} {
		if _, err := LoadRedisConfig(addr, ""); err == nil {
			t.Errorf("address %q accepted", addr)
		}
	}
	file := filepath.Join(t.TempDir(), "redis-password")
	if err := os.WriteFile(file, []byte("pw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadRedisConfig("redis:6379", file)
	if err != nil || c.Addr != "redis:6379" || c.password != "pw" {
		t.Fatalf("load: %+v %v", c, err)
	}
	if c, err := LoadRedisConfig("[::1]:6379", ""); err != nil || c.password != "" {
		t.Fatalf("IPv6 without password: %+v %v", c, err)
	}
	if err := os.WriteFile(file, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRedisConfig("redis:6379", file); err == nil {
		t.Fatal("empty password accepted")
	}
	if _, err := LoadRedisConfig("redis:6379", filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing password file accepted")
	}
}

func newRedisHandler(t *testing.T, idp *fakeIdP, srv *fakeRedis, password string) http.Handler {
	t.Helper()
	a := New(testConfig(t, idp), Options{
		Client: idp.srv.Client(), Now: idp.clock.Now,
		Redis: &RedisConfig{Addr: srv.addr(), password: password},
	})
	t.Cleanup(a.Close)
	return a.Wrap(protected)
}

func TestRedisSessionsSurviveRestart(t *testing.T) {
	idp := newFakeIdP(t)
	srv := newFakeRedis(t, "pw", idp.clock.Now)
	h := newRedisHandler(t, idp, srv, "pw")
	session := signIn(t, h, idp)

	// Only hashes of the cookie and state are stored, never the values.
	keys := srv.keys()
	if len(keys) != 1 || keys[0] != redisKey(redisSessionPrefix, session.Value) ||
		len(strings.TrimPrefix(keys[0], redisSessionPrefix)) != 64 {
		t.Fatalf("keys=%v", keys)
	}
	if strings.Contains(srv.values()+strings.Join(keys, ""), session.Value) {
		t.Fatal("Redis holds the session cookie value")
	}
	if ttl := srv.ttl(keys[0]); ttl != 8*time.Hour {
		t.Fatalf("session TTL=%v", ttl)
	}

	// A second API process with the same Redis knows the session.
	restarted := newRedisHandler(t, idp, srv, "pw")
	if rec := do(restarted, http.MethodGet, "/v1/projects", []*http.Cookie{session}, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("after restart status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec := do(restarted, http.MethodPost, "/v1/auth/logout", []*http.Cookie{session}, map[string]string{"Sec-Fetch-Site": "same-origin"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "id_token_hint") {
		t.Fatalf("logout status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(srv.keys()) != 0 {
		t.Fatalf("logout left keys %v", srv.keys())
	}
	if rec := do(h, http.MethodGet, "/v1/projects", []*http.Cookie{session}, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("signed-out session still works, status=%d", rec.Code)
	}
}

func TestRedisPendingStateIsSingleUseAndExpires(t *testing.T) {
	idp := newFakeIdP(t)
	srv := newFakeRedis(t, "", idp.clock.Now)
	h := newRedisHandler(t, idp, srv, "")

	q, login := startLogin(t, h, "/x")
	keys := srv.keys()
	if len(keys) != 1 || keys[0] != redisKey(redisPendingPrefix, q.Get("state")) || srv.ttl(keys[0]) != pendingTTL {
		t.Fatalf("pending keys=%v ttl=%v", keys, srv.ttls)
	}
	if strings.Contains(srv.values(), q.Get("state")) {
		t.Fatal("Redis holds the state value")
	}
	code := idp.authorize(q)
	if rec := callback(h, code, q.Get("state"), login); rec.Code != http.StatusSeeOther {
		t.Fatalf("callback status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := callback(h, code, q.Get("state"), login); rec.Code != http.StatusBadRequest {
		t.Fatalf("replayed callback status=%d", rec.Code)
	}

	q, login = startLogin(t, h, "/")
	idp.clock.Add(pendingTTL)
	if rec := callback(h, idp.authorize(q), q.Get("state"), login); rec.Code != http.StatusBadRequest {
		t.Fatalf("expired attempt status=%d", rec.Code)
	}
}

func TestRedisSessionsExpire(t *testing.T) {
	idp := newFakeIdP(t)
	srv := newFakeRedis(t, "", idp.clock.Now)
	h := newRedisHandler(t, idp, srv, "")
	session := []*http.Cookie{signIn(t, h, idp)}
	idp.clock.Add(8*time.Hour - time.Second)
	if rec := do(h, http.MethodGet, "/v1/projects", session, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rec.Code)
	}
	idp.clock.Add(time.Second)
	if rec := do(h, http.MethodGet, "/v1/projects", session, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session status=%d", rec.Code)
	}
}

func TestRedisOutageFailsClosed(t *testing.T) {
	idp := newFakeIdP(t)
	srv := newFakeRedis(t, "", idp.clock.Now)
	h := newRedisHandler(t, idp, srv, "")
	session := []*http.Cookie{signIn(t, h, idp)}

	srv.down.Store(true)
	srv.dropConnections()
	expectUnavailable := func(name string, rec interface {
		Result() *http.Response
	}, code int, body string) {
		t.Helper()
		if code != http.StatusServiceUnavailable || !strings.Contains(body, "session_store_unavailable") {
			t.Errorf("%s: status=%d body=%s", name, code, body)
		}
		for _, c := range rec.Result().Cookies() {
			if c.Name == "__Host-ember_session" {
				t.Errorf("%s: changed the session cookie during an outage", name)
			}
		}
	}
	for _, tc := range []struct{ name, method, target string }{
		{"protected route", http.MethodGet, "/v1/projects"},
		{"session", http.MethodGet, "/v1/auth/session"},
		{"logout", http.MethodPost, "/v1/auth/logout"},
	} {
		rec := do(h, tc.method, tc.target, session, map[string]string{"Sec-Fetch-Site": "same-origin"})
		expectUnavailable(tc.name, rec, rec.Code, rec.Body.String())
	}
	rec := do(h, http.MethodGet, "/v1/auth/login", nil, nil)
	expectUnavailable("login", rec, rec.Code, rec.Body.String())
	if rec := do(h, http.MethodGet, "/healthz", nil, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("health during outage status=%d", rec.Code)
	}
	// Signed-out callers do not need the store to learn that sign-in is on.
	if rec := do(h, http.MethodGet, "/v1/auth/session", nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("anonymous session status=%d", rec.Code)
	}

	srv.down.Store(false)
	if rec := do(h, http.MethodGet, "/v1/projects", session, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("after recovery status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRedisOutageDuringCallback(t *testing.T) {
	idp := newFakeIdP(t)
	srv := newFakeRedis(t, "", idp.clock.Now)
	h := newRedisHandler(t, idp, srv, "")
	q, login := startLogin(t, h, "/")
	srv.down.Store(true)
	srv.dropConnections()
	rec := callback(h, idp.authorize(q), q.Get("state"), login)
	if rec.Code != http.StatusServiceUnavailable || errorCode(t, rec) != "session_store_unavailable" ||
		responseCookie(rec, "__Host-ember_session") != nil {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRedisUnreachableOrWrongPassword(t *testing.T) {
	idp := newFakeIdP(t)
	srv := newFakeRedis(t, "right", idp.clock.Now)
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refused := closed.Addr().String()
	closed.Close()

	for name, cfg := range map[string]*RedisConfig{
		"wrong password":     {Addr: srv.addr(), password: "wrong"},
		"connection refused": {Addr: refused},
	} {
		a := New(testConfig(t, idp), Options{Client: idp.srv.Client(), Now: idp.clock.Now, Redis: cfg})
		if a.CheckSessionStore(context.Background()) == nil {
			t.Errorf("%s: store check passed", name)
		}
		h := a.Wrap(protected)
		rec := do(h, http.MethodGet, "/v1/projects", []*http.Cookie{{Name: "__Host-ember_session", Value: "x"}}, nil)
		if rec.Code != http.StatusServiceUnavailable || errorCode(t, rec) != "session_store_unavailable" {
			t.Errorf("%s: status=%d body=%s", name, rec.Code, rec.Body.String())
		}
		if rec := do(h, http.MethodGet, "/v1/auth/login", nil, nil); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: login status=%d", name, rec.Code)
		}
		a.Close()
	}
}

func TestRedisIgnoresUnreadableRecords(t *testing.T) {
	idp := newFakeIdP(t)
	srv := newFakeRedis(t, "", idp.clock.Now)
	h := newRedisHandler(t, idp, srv, "")
	for name, value := range map[string]string{
		"not JSON":     "{",
		"unknown role": `{"user":{"subject":"u"},"role":"root","expires":"2030-01-01T00:00:00Z"}`,
		"no subject":   `{"user":{},"role":"admin","expires":"2030-01-01T00:00:00Z"}`,
	} {
		srv.mu.Lock()
		srv.data[redisKey(redisSessionPrefix, name)] = fakeEntry{value: value, expires: idp.clock.Now().Add(time.Hour)}
		srv.mu.Unlock()
		rec := do(h, http.MethodGet, "/v1/projects", []*http.Cookie{{Name: "__Host-ember_session", Value: name}}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status=%d", name, rec.Code)
		}
	}
}
