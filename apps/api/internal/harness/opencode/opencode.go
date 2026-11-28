// Package opencode is the OpenCode harness: it drives an OpenCode V2 worker
// (`opencode serve`) over its HTTP session API with the worker's basic-auth
// password. It uses only the endpoints the retired review panel verified
// against OpenCode 2.3.2 (see protocol_test.go):
//
//	GET  /api/session/{id}                       resume
//	POST /api/session                            create {"id","location":{"directory"}}
//	POST /api/session/{id}/prompt                {"text","delivery":"steer"}
//	POST /api/session/{id}/interrupt?resume=false
//	GET  /api/session/{id}/message?limit=N&order=desc
//	GET  /api/session/{id}/permission
//	POST /api/session/{id}/permission/{pid}/reply {"decision"}
//	GET  /api/event                              global SSE stream
package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/harness"
)

const (
	callTimeout   = 10 * time.Second
	maxBody       = 4 << 20
	maxEventLine  = 1 << 20
	maxMessages   = 200
	eventBuffer   = 256
	basicAuthUser = "opencode"
)

// IDs are put into URL paths; refuse anything that could change the path.
var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// Client is one OpenCode worker. It implements harness.Harness.
type Client struct {
	base, password string
	client         *http.Client
}

var _ harness.Harness = (*Client)(nil)

// New returns a client for the worker at base (http://host[:port]). It uses
// no proxy, follows no redirects and gives up on a worker that does not
// send response headers within 8 seconds.
func New(base, password string) *Client {
	return NewWithClient(base, password, &http.Client{
		Transport: &http.Transport{
			Proxy:                  nil,
			ResponseHeaderTimeout:  8 * time.Second,
			MaxResponseHeaderBytes: 64 << 10,
			IdleConnTimeout:        90 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	})
}

// NewWithClient uses client for requests, for tests. The client should not
// follow redirects: a redirect would carry the worker password elsewhere.
func NewWithClient(base, password string, client *http.Client) *Client {
	return &Client{base: strings.TrimSuffix(base, "/"), password: password, client: client}
}

func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(basicAuthUser, c.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.client.Do(req)
}

// call makes one bounded request. Worker errors, which can include provider
// responses, stay behind the adapter: callers see ErrNotFound or ErrUnavailable.
func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	res, err := c.request(ctx, method, path, body)
	if err != nil {
		return fmt.Errorf("%w: %s %s", harness.ErrUnavailable, method, routeName(path))
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		return harness.ErrNotFound
	case res.StatusCode < 200 || res.StatusCode >= 300:
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		return fmt.Errorf("%w: %s %s answered %d", harness.ErrUnavailable, method, routeName(path), res.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil || len(b) > maxBody || json.Unmarshal(b, out) != nil {
		return fmt.Errorf("%w: unreadable answer to %s %s", harness.ErrUnavailable, method, routeName(path))
	}
	return nil
}

// routeName names a route for logs without IDs or query values.
func routeName(path string) string {
	path, _, _ = strings.Cut(path, "?")
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		if parts[i-1] == "session" || parts[i-1] == "permission" {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func sessionPath(id string) (string, error) {
	if !safeID.MatchString(id) {
		return "", harness.ErrNotFound
	}
	return "/api/session/" + id, nil
}

// Open resumes the session, or creates it in s.Directory if it is missing.
func (c *Client) Open(ctx context.Context, s harness.Session) error {
	path, err := sessionPath(s.ID)
	if err != nil {
		return err
	}
	var info struct{ Data struct{ ID string } }
	err = c.call(ctx, http.MethodGet, path, nil, &info)
	if errors.Is(err, harness.ErrNotFound) {
		create := map[string]any{"id": s.ID, "location": map[string]string{"directory": s.Directory}}
		err = c.call(ctx, http.MethodPost, "/api/session", create, &info)
	}
	if err != nil {
		return err
	}
	if info.Data.ID != s.ID {
		return fmt.Errorf("%w: session answer names another session", harness.ErrUnavailable)
	}
	return nil
}

// Prompt always uses steer delivery: OpenCode starts a run on an idle
// session and folds the text into the active run otherwise.
func (c *Client) Prompt(ctx context.Context, sessionID, text string) error {
	path, err := sessionPath(sessionID)
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, path+"/prompt", map[string]string{"text": text, "delivery": "steer"}, nil)
}

// Interrupt stops the active run without resuming queued input.
func (c *Client) Interrupt(ctx context.Context, sessionID string) (bool, error) {
	path, err := sessionPath(sessionID)
	if err != nil {
		return false, err
	}
	var result struct {
		Interrupted *bool `json:"interrupted"`
	}
	if err := c.call(ctx, http.MethodPost, path+"/interrupt?resume=false", nil, &result); err != nil {
		return false, err
	}
	if result.Interrupted == nil {
		return false, fmt.Errorf("%w: interrupt answer without a result", harness.ErrUnavailable)
	}
	return *result.Interrupted, nil
}

// Messages maps OpenCode's history onto harness messages. Reasoning and any
// other content parts are dropped.
func (c *Client) Messages(ctx context.Context, sessionID string, limit int) ([]harness.Message, error) {
	path, err := sessionPath(sessionID)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > maxMessages {
		limit = maxMessages
	}
	var page struct {
		Data []struct {
			Type, Text, Outcome string
			Content             []struct {
				Type, Name, Text string
				State            struct{ Status string }
			}
		}
	}
	if err := c.call(ctx, http.MethodGet, path+"/message?limit="+strconv.Itoa(limit)+"&order=desc", nil, &page); err != nil {
		return nil, err
	}
	if page.Data == nil {
		return nil, fmt.Errorf("%w: message answer without data", harness.ErrUnavailable)
	}
	out := []harness.Message{}
	for i := len(page.Data) - 1; i >= 0; i-- {
		m := page.Data[i]
		switch m.Type {
		case "user":
			out = append(out, harness.Message{Role: harness.RoleUser, Text: m.Text})
		case "idle":
			out = append(out, harness.Message{Role: harness.RoleRun, Text: m.Outcome})
		case "assistant":
			for _, part := range m.Content {
				switch part.Type {
				case "text":
					out = append(out, harness.Message{Role: harness.RoleAgent, Text: part.Text})
				case "tool":
					out = append(out, harness.Message{Role: harness.RoleTool, Text: part.Name + ": " + part.State.Status})
				}
			}
		}
	}
	return out, nil
}

// Permissions lists the session's open permission requests. A session that
// does not exist yet has none.
func (c *Client) Permissions(ctx context.Context, sessionID string) ([]harness.Permission, error) {
	path, err := sessionPath(sessionID)
	if err != nil {
		return nil, err
	}
	var page struct{ Data []harness.Permission }
	err = c.call(ctx, http.MethodGet, path+"/permission", nil, &page)
	if errors.Is(err, harness.ErrNotFound) {
		return []harness.Permission{}, nil
	}
	if err != nil {
		return nil, err
	}
	if page.Data == nil {
		return nil, fmt.Errorf("%w: permission answer without data", harness.ErrUnavailable)
	}
	for i := range page.Data {
		if page.Data[i].Resources == nil {
			page.Data[i].Resources = []string{}
		}
	}
	return page.Data, nil
}

// Reply answers one permission request.
func (c *Client) Reply(ctx context.Context, sessionID, permissionID string, d harness.Decision) error {
	path, err := sessionPath(sessionID)
	if err != nil {
		return err
	}
	if !safeID.MatchString(permissionID) || (d != harness.Once && d != harness.Reject) {
		return harness.ErrNotFound
	}
	return c.call(ctx, http.MethodPost, path+"/permission/"+url.PathEscape(permissionID)+"/reply", map[string]string{"decision": string(d)}, nil)
}

// eventTypes maps OpenCode's event names onto Ember's. Everything else on
// the global stream (other sessions' configuration, file watching, ...) is
// dropped.
var eventTypes = map[string]string{
	"session.text.delta":            harness.TextDelta,
	"session.tool.called":           harness.ToolCalled,
	"session.tool.success":          harness.ToolSucceeded,
	"session.tool.failed":           harness.ToolFailed,
	"session.execution.started":     harness.RunStarted,
	"session.execution.succeeded":   harness.RunSucceeded,
	"session.execution.failed":      harness.RunFailed,
	"session.execution.interrupted": harness.RunInterrupted,
	"permission.asked":              harness.PermissionAsked,
	"permission.replied":            harness.PermissionReplied,
}

// MapEvent turns one SSE data payload into an Ember event.
func MapEvent(data []byte) (harness.Event, bool) {
	var e struct {
		Type string
		Data struct{ SessionID, Delta string }
	}
	if json.Unmarshal(data, &e) != nil || e.Data.SessionID == "" {
		return harness.Event{}, false
	}
	t, ok := eventTypes[e.Type]
	if !ok {
		return harness.Event{}, false
	}
	out := harness.Event{Session: e.Data.SessionID, Type: t}
	if t == harness.TextDelta {
		if e.Data.Delta == "" {
			return harness.Event{}, false
		}
		out.Delta = e.Data.Delta
	}
	return out, true
}

// Subscribe opens the worker's global event stream. The channel closes when
// the stream ends, fails or ctx is cancelled.
func (c *Client) Subscribe(ctx context.Context) (<-chan harness.Event, error) {
	res, err := c.request(ctx, http.MethodGet, "/api/event", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: event stream", harness.ErrUnavailable)
	}
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		res.Body.Close()
		return nil, fmt.Errorf("%w: event stream answered %d", harness.ErrUnavailable, res.StatusCode)
	}
	events := make(chan harness.Event, eventBuffer)
	go func() {
		defer close(events)
		defer res.Body.Close()
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 4096), maxEventLine)
		for sc.Scan() {
			line, ok := strings.CutPrefix(sc.Text(), "data:")
			if !ok {
				continue
			}
			e, ok := MapEvent([]byte(line))
			if !ok {
				continue
			}
			select {
			case events <- e:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, nil
}
