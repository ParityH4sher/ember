package immich

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testKey = "test-key"

func config(base string) map[string]any {
	return map[string]any{"baseUrl": base, "apiKey": testKey}
}

// fixture serves Immich responses under /photos, as behind a reverse-proxy subpath.
func fixture(t *testing.T, albums string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("x-api-key") != testKey || r.URL.RawQuery != "" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/photos/api/server/version":
			fmt.Fprint(w, `{"major":3,"minor":4,"patch":2,"prerelease":null}`)
		case "/photos/api/users/me":
			fmt.Fprint(w, `{"id":"u1","name":"ember-poc","email":"hidden@example.test","isAdmin":false}`)
		case "/photos/api/albums":
			fmt.Fprint(w, albums)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestServerInfoAndAlbums(t *testing.T) {
	s := fixture(t, `[{"id":"a1","albumName":"Workshop","assetCount":12,"shared":true,
		"updatedAt":"2026-11-23T10:00:00.000Z","owner":{"name":"Owner","email":"hidden@example.test"}}]`)
	a := New()

	server, err := a.Invoke(context.Background(), "server", config(s.URL+"/photos/"), nil)
	if err != nil {
		t.Fatal(err)
	}
	info := server.(map[string]any)
	if info["version"] != "3.4.2" || info["user"] != "ember-poc" || info["isAdmin"] != false {
		t.Fatalf("server info = %v", info)
	}

	result, err := a.Invoke(context.Background(), "albums", config(s.URL+"/photos"), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := result.(map[string]any)
	albums := got["albums"].([]album)
	want := album{"a1", "Workshop", 12, true, "Owner", "2026-11-23T10:00:00.000Z"}
	if len(albums) != 1 || albums[0] != want || got["truncated"] != false {
		t.Fatalf("albums = %v", got)
	}

	for _, out := range []any{server, result} {
		encoded, _ := json.Marshal(out)
		if strings.Contains(string(encoded), "hidden") || strings.Contains(string(encoded), testKey) {
			t.Fatalf("result exposes upstream-only data: %s", encoded)
		}
	}
}

func TestAlbumsAreTruncated(t *testing.T) {
	items := make([]string, MaxAlbums+5)
	for i := range items {
		items[i] = fmt.Sprintf(`{"id":"a%d","albumName":"Album %d","owner":{"name":"o"}}`, i, i)
	}
	s := fixture(t, "["+strings.Join(items, ",")+"]")
	result, err := New().Invoke(context.Background(), "albums", config(s.URL+"/photos"), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := result.(map[string]any)
	if len(got["albums"].([]album)) != MaxAlbums || got["truncated"] != true {
		t.Fatalf("got %d albums, truncated=%v", len(got["albums"].([]album)), got["truncated"])
	}
}

func TestRejectsInvalidUpstreamResponses(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/elsewhere" {
				t.Error("followed a redirect, sending the key to another location")
			}
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		},
		"unauthorized": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"Invalid API key"}`)
		},
		"oversized": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `[`+strings.Repeat(" ", responseLimit)+`]`)
		},
		"not immich": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"status":"ok"}`)
		},
		"not json": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `<html>login</html>`)
		},
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			s := httptest.NewServer(handler)
			defer s.Close()
			for _, operation := range []string{"server", "albums"} {
				if _, err := New().Invoke(context.Background(), operation, config(s.URL), nil); err == nil {
					t.Fatalf("%s accepted an invalid upstream response", operation)
				}
			}
		})
	}
}

func TestRejectsUnsafeConfigurationWithoutCalling(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer s.Close()
	for name, configuration := range map[string]map[string]any{
		"file scheme":      {"baseUrl": "file:///etc/passwd", "apiKey": testKey},
		"credentials":      {"baseUrl": strings.Replace(s.URL, "://", "://user:pass@", 1), "apiKey": testKey},
		"query":            {"baseUrl": s.URL + "?next=/", "apiKey": testKey},
		"missing key":      {"baseUrl": s.URL},
		"header injection": {"baseUrl": s.URL, "apiKey": "key\r\nX-Other: 1"},
		"no configuration": nil,
	} {
		if _, err := New().Invoke(context.Background(), "server", configuration, nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := New().Invoke(context.Background(), "delete", config(s.URL), nil); err == nil {
		t.Error("accepted an unknown operation")
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("made %d upstream calls for rejected configurations", n)
	}
}
