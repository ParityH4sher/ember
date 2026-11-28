package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/harness"
)

const (
	testSession = "ses_ember_0123456789abcdef"
	session     = "/api/session/" + testSession
)

// fakeWorker checks the worker identity on every request and hands the
// request to route.
func fakeWorker(t *testing.T, route http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pw, _ := r.BasicAuth()
		if user != "opencode" || pw != "private-worker" {
			t.Error("wrong worker identity")
			w.WriteHeader(401)
			return
		}
		route(w, r)
	}))
	t.Cleanup(s.Close)
	return New(s.URL, "private-worker")
}

func TestSessionLifecycle(t *testing.T) {
	created := false
	c := fakeWorker(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.RequestURI() {
		case "GET " + session:
			if !created {
				w.WriteHeader(404)
				return
			}
			fmt.Fprintf(w, `{"data":{"id":%q}}`, testSession)
		case "POST /api/session":
			var v struct {
				ID       string
				Location struct{ Directory string }
			}
			if created || json.NewDecoder(r.Body).Decode(&v) != nil || v.ID != testSession || v.Location.Directory != "/workspace" {
				t.Error("wrong session creation")
			}
			created = true
			fmt.Fprintf(w, `{"data":{"id":%q}}`, testSession)
		case "POST " + session + "/prompt":
			var v struct{ Text, Delivery string }
			if json.NewDecoder(r.Body).Decode(&v) != nil || v.Delivery != "steer" || v.Text != "focus on errors" {
				t.Error("wrong prompt delivery")
			}
			fmt.Fprint(w, `{"data":{"delivery":"steer"}}`)
		case "GET " + session + "/message?limit=50&order=desc":
			fmt.Fprint(w, `{"data":[{"type":"idle","outcome":"succeeded"},{"type":"assistant","content":[{"type":"reasoning","text":"private"},{"type":"text","text":"A result"},{"type":"tool","name":"read","state":{"status":"completed"}}]},{"type":"user","text":"review"}]}`)
		case "GET " + session + "/permission":
			fmt.Fprint(w, `{"data":[{"id":"per_1","action":"bash","resources":["ls"]},{"id":"per_2","action":"edit"}]}`)
		case "POST " + session + "/permission/per_1/reply":
			var d struct{ Decision string }
			if json.NewDecoder(r.Body).Decode(&d) != nil || d.Decision != "once" {
				t.Error("wrong permission decision")
			}
			w.WriteHeader(204)
		case "POST " + session + "/interrupt?resume=false":
			fmt.Fprint(w, `{"interrupted":true}`)
		default:
			t.Errorf("unexpected upstream %s %s", r.Method, r.URL)
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	s := harness.Session{ID: testSession, Directory: "/workspace"}
	if err := c.Open(ctx, s); err != nil || !created {
		t.Fatalf("open (create) = %v", err)
	}
	if err := c.Open(ctx, s); err != nil {
		t.Fatalf("open (resume) = %v", err)
	}
	if err := c.Prompt(ctx, testSession, "focus on errors"); err != nil {
		t.Fatal(err)
	}
	msgs, err := c.Messages(ctx, testSession, 50)
	want := []harness.Message{{Role: "user", Text: "review"}, {Role: "agent", Text: "A result"}, {Role: "tool", Text: "read: completed"}, {Role: "run", Text: "succeeded"}}
	if err != nil || fmt.Sprint(msgs) != fmt.Sprint(want) {
		t.Fatalf("messages = %v, %v", msgs, err)
	}
	perms, err := c.Permissions(ctx, testSession)
	if err != nil || len(perms) != 2 || perms[0].Resources[0] != "ls" || perms[1].Resources == nil {
		t.Fatalf("permissions = %+v, %v", perms, err)
	}
	if err := c.Reply(ctx, testSession, "per_1", harness.Once); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.Interrupt(ctx, testSession); !ok || err != nil {
		t.Fatalf("interrupt = %v, %v", ok, err)
	}
}

func TestUpstreamErrorsStayBehindTheAdapter(t *testing.T) {
	c := fakeWorker(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/prompt"):
			http.Error(w, "provider said: invalid key sk-secret", 500)
		case strings.HasSuffix(r.URL.Path, "/interrupt"):
			fmt.Fprint(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/message"):
			fmt.Fprint(w, `{"data":`+strings.Repeat(" ", maxBody)+`[]}`)
		case strings.HasSuffix(r.URL.Path, "/permission"):
			w.WriteHeader(404)
		case strings.HasSuffix(r.URL.Path, "/reply"):
			w.WriteHeader(404)
		default:
			http.Redirect(w, r, "http://elsewhere.invalid/", http.StatusFound)
		}
	})
	ctx := context.Background()
	err := c.Prompt(ctx, testSession, "hi")
	if !errors.Is(err, harness.ErrUnavailable) || strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("prompt = %v", err)
	}
	if _, err := c.Interrupt(ctx, testSession); !errors.Is(err, harness.ErrUnavailable) {
		t.Fatalf("interrupt without a result = %v", err)
	}
	if _, err := c.Messages(ctx, testSession, 10); !errors.Is(err, harness.ErrUnavailable) {
		t.Fatalf("oversized messages = %v", err)
	}
	if p, err := c.Permissions(ctx, testSession); err != nil || len(p) != 0 {
		t.Fatalf("permissions of a missing session = %v, %v", p, err)
	}
	if err := c.Reply(ctx, testSession, "per_1", harness.Once); !errors.Is(err, harness.ErrNotFound) {
		t.Fatalf("reply to a missing permission = %v", err)
	}
	// A redirect is not followed (it would carry the password elsewhere).
	if err := c.Open(ctx, harness.Session{ID: testSession}); !errors.Is(err, harness.ErrUnavailable) {
		t.Fatalf("open with a redirect = %v", err)
	}
	// IDs that could change the request path never reach the worker.
	for _, id := range []string{"../config", "a/b", "", "x?y"} {
		if err := c.Prompt(ctx, id, "hi"); !errors.Is(err, harness.ErrNotFound) {
			t.Fatalf("prompt %q = %v", id, err)
		}
	}
	if err := c.Reply(ctx, testSession, "../../x", harness.Once); !errors.Is(err, harness.ErrNotFound) {
		t.Fatal("unsafe permission ID reached the worker")
	}
	if err := c.Reply(ctx, testSession, "per_1", "always"); !errors.Is(err, harness.ErrNotFound) {
		t.Fatal("unknown decision reached the worker")
	}
}

func TestSubscribeMapsAndFiltersEvents(t *testing.T) {
	c := fakeWorker(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/event" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range []string{
			`: comment`,
			`data: {"type":"session.text.delta","data":{"sessionID":"ses_other","delta":"other"}}`,
			`data: {"type":"session.text.delta","data":{"sessionID":"` + testSession + `","delta":"progress"}}`,
			`data: {"type":"session.reasoning.delta","data":{"sessionID":"` + testSession + `","delta":"private"}}`,
			`data: {"type":"config.updated","data":{}}`,
			`data:{"type":"session.tool.success","data":{"sessionID":"` + testSession + `","delta":"ignored"}}`,
			`data: not json`,
			`data: {"type":"session.execution.succeeded","data":{"sessionID":"` + testSession + `"}}`,
			`data: {"type":"permission.asked","data":{"sessionID":"` + testSession + `"}}`,
		} {
			fmt.Fprint(w, line+"\n\n")
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for e := range events {
		got = append(got, e.Session+" "+e.Type+" "+e.Delta)
	}
	want := []string{
		"ses_other text.delta other",
		testSession + " text.delta progress",
		testSession + " tool.succeeded ",
		testSession + " run.succeeded ",
		testSession + " permission.asked ",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events:\n%s", strings.Join(got, "\n"))
	}
}

func TestSubscribeRefusesNonStream(t *testing.T) {
	c := fakeWorker(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) })
	if _, err := c.Subscribe(context.Background()); !errors.Is(err, harness.ErrUnavailable) {
		t.Fatalf("subscribe = %v", err)
	}
}
