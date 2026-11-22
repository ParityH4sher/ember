package gitea

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFixedRepository(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forge/api/v1/repos/team/widget" || r.Method != "GET" || r.URL.RawQuery != "" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		w.Write([]byte(`{"full_name":"team/widget","default_branch":"main","owner":{"email":"hidden"}}`))
	}))
	defer s.Close()
	reg, err := Register(s.URL + "/forge/team/widget")
	if err != nil {
		t.Fatal(err)
	}
	result, err := reg.Adapter.Invoke(context.Background(), "repository", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := result.(map[string]string)
	if len(got) != 3 || got["repository"] != "team/widget" || got["defaultBranch"] != "main" {
		t.Fatal(got)
	}
}

func TestRejectRedirectOrOversizedResponse(t *testing.T) {
	for _, redirect := range []bool{true, false} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/redirected" {
				t.Error("followed redirect")
			}
			if redirect {
				http.Redirect(w, r, "/redirected", 302)
			} else {
				w.Write([]byte(`{"full_name":"x"}` + strings.Repeat(" ", 128<<10)))
			}
		}))
		reg, err := Register(s.URL + "/owner/repo")
		if err != nil {
			t.Fatal(err)
		}
		_, err = reg.Adapter.Invoke(context.Background(), "repository", nil, nil)
		s.Close()
		if err == nil {
			t.Fatal("accepted invalid upstream response")
		}
	}
}
