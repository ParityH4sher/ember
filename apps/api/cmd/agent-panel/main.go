// agent-panel controls one disposable review workspace. The upstream address,
// working directory and session are deployment decisions, never browser inputs.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	prefix    = "/apps/agent/"
	sessionID = "ses_ember_review"
	session   = "/api/session/" + sessionID
)

//go:embed index.html panel.js
var assets embed.FS

type panel struct {
	upstream, password, operator string
	client                       *http.Client
	connectMu                    sync.Mutex
}

func (p *panel) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.upstream+path, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("opencode", p.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return p.client.Do(req)
}

// Keep worker errors (which can include provider responses) behind the adapter.
func (p *panel) call(ctx context.Context, method, path string, body io.Reader, out any) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := p.request(ctx, method, path, body)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return res.StatusCode, errors.New("worker rejected request")
	}
	if out == nil {
		return res.StatusCode, nil
	}
	const limit = 4 << 20
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || len(b) > limit {
		return res.StatusCode, errors.New("unreadable worker response")
	}
	return res.StatusCode, json.Unmarshal(b, out)
}

func output(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func unavailable(w http.ResponseWriter) {
	http.Error(w, "OpenCode worker unavailable or incompatible", http.StatusBadGateway)
}

func (p *panel) connect(w http.ResponseWriter, r *http.Request) {
	p.connectMu.Lock()
	defer p.connectMu.Unlock()
	var info struct{ Data struct{ ID string } }
	code, err := p.call(r.Context(), "GET", session, nil, &info)
	if code == 404 {
		body := strings.NewReader(`{"id":"` + sessionID + `","location":{"directory":"/workspace"}}`)
		_, err = p.call(r.Context(), "POST", "/api/session", body, &info)
	}
	if err != nil || info.Data.ID != sessionID {
		unavailable(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p *panel) steer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(new(any)) != io.EOF || strings.TrimSpace(in.Text) == "" {
		http.Error(w, "expected a non-empty text message (32 KiB maximum)", 400)
		return
	}
	body, _ := json.Marshal(map[string]string{"text": in.Text, "delivery": "steer"})
	if _, err := p.call(r.Context(), "POST", session+"/prompt", bytes.NewReader(body), nil); err != nil {
		unavailable(w)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (p *panel) interrupt(w http.ResponseWriter, r *http.Request) {
	var result struct {
		Interrupted *bool `json:"interrupted"`
	}
	if _, err := p.call(r.Context(), "POST", session+"/interrupt?resume=false", nil, &result); err != nil || result.Interrupted == nil {
		unavailable(w)
		return
	}
	output(w, result)
}

type permission struct {
	ID        string   `json:"id"`
	Action    string   `json:"action"`
	Resources []string `json:"resources"`
}

func (p *panel) permissions(w http.ResponseWriter, r *http.Request) {
	var page struct{ Data []permission }
	if _, err := p.call(r.Context(), "GET", session+"/permission", nil, &page); err != nil || page.Data == nil {
		unavailable(w)
		return
	}
	output(w, page.Data)
}

var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func (p *panel) decide(w http.ResponseWriter, r *http.Request) {
	id, decision := r.PathValue("id"), r.PathValue("decision")
	if !safeID.MatchString(id) || (decision != "once" && decision != "reject") {
		http.Error(w, "invalid permission decision", 400)
		return
	}
	body, _ := json.Marshal(map[string]string{"decision": decision})
	if _, err := p.call(r.Context(), "POST", session+"/permission/"+id+"/reply", bytes.NewReader(body), nil); err != nil {
		unavailable(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p *panel) messages(w http.ResponseWriter, r *http.Request) {
	var page struct {
		Data []struct {
			Type, Text, Outcome string
			Content             []struct {
				Type, Name, Text string
				State            struct{ Status string }
			}
		}
	}
	if _, err := p.call(r.Context(), "GET", session+"/message?limit=50&order=desc", nil, &page); err != nil || page.Data == nil {
		unavailable(w)
		return
	}
	type line struct {
		Role string `json:"role"`
		Text string `json:"text"`
	}
	lines := []line{}
	for i := len(page.Data) - 1; i >= 0; i-- {
		m := page.Data[i]
		switch m.Type {
		case "user":
			lines = append(lines, line{"You", m.Text})
		case "idle":
			lines = append(lines, line{"Run", m.Outcome})
		case "assistant":
			for _, c := range m.Content {
				switch c.Type {
				case "text":
					lines = append(lines, line{"Agent", c.Text})
				case "tool":
					lines = append(lines, line{"Tool", c.Name + ": " + c.State.Status})
				}
			}
		}
	}
	output(w, lines)
}

// OpenCode's global SSE stream includes other sessions and configuration
// changes. Only project a few event types for this task onto the browser stream.
func (p *panel) events(w http.ResponseWriter, r *http.Request) {
	res, err := p.request(r.Context(), "GET", "/api/event", nil)
	if err != nil {
		unavailable(w)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		unavailable(w)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	rc := http.NewResponseController(w)
	send := func(payload []byte) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err := fmt.Fprintf(w, "data: %s\n\n", payload)
		return err == nil && rc.Flush() == nil
	}
	if !send([]byte(`{"type":"connected"}`)) {
		return
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 4096), 1<<20)
	for sc.Scan() {
		line, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		var e struct {
			Type string
			Data struct{ SessionID, Delta string }
		}
		if json.Unmarshal([]byte(line), &e) != nil || e.Data.SessionID != sessionID {
			continue
		}
		switch e.Type {
		case "session.text.delta", "session.tool.called", "session.tool.success", "session.tool.failed", "session.execution.started", "session.execution.succeeded", "session.execution.failed", "session.execution.interrupted", "permission.asked", "permission.replied":
			b, _ := json.Marshal(map[string]string{"type": e.Type, "delta": e.Data.Delta})
			if !send(b) {
				return
			}
		}
	}
}

func (p *panel) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+prefix+"{$}", func(w http.ResponseWriter, r *http.Request) { http.ServeFileFS(w, r, assets, "index.html") })
	mux.HandleFunc("GET "+prefix+"panel.js", func(w http.ResponseWriter, r *http.Request) { http.ServeFileFS(w, r, assets, "panel.js") })
	mux.HandleFunc("POST "+prefix+"connect", p.connect)
	mux.HandleFunc("POST "+prefix+"steer", p.steer)
	mux.HandleFunc("POST "+prefix+"interrupt", p.interrupt)
	mux.HandleFunc("GET "+prefix+"messages", p.messages)
	mux.HandleFunc("GET "+prefix+"events", p.events)
	mux.HandleFunc("GET "+prefix+"permissions", p.permissions)
	mux.HandleFunc("POST "+prefix+"permissions/{id}/{decision}", p.decide)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; connect-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		user, pass, ok := r.BasicAuth()
		if !ok || user != "operator" || subtle.ConstantTimeCompare([]byte(pass), []byte(p.operator)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Ember agent panel"`)
			http.Error(w, "operator login required", 401)
			return
		}
		// Browser Basic credentials are ambient. Cross-origin forms cannot set
		// this header, and the controller never grants CORS preflight.
		if r.Method == "POST" && r.Header.Get("X-Ember-Action") != "1" {
			http.Error(w, "missing action header", 403)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func main() {
	operator, worker := os.Getenv("EMBER_PANEL_PASSWORD"), os.Getenv("OPENCODE_PASSWORD")
	if len(operator) < 32 || len(worker) < 32 || operator == worker {
		log.Fatal("set distinct panel and worker passwords of at least 32 bytes each")
	}
	p := &panel{upstream: "http://opencode-worker:4096", operator: operator, password: worker, client: &http.Client{
		Transport:     &http.Transport{ResponseHeaderTimeout: 8 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	srv := &http.Server{Addr: ":8080", Handler: p.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 65 * time.Second, MaxHeaderBytes: 16 << 10}
	log.Fatal(srv.ListenAndServe())
}
