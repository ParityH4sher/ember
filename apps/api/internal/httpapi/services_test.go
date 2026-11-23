package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ParityH4sher/ember/apps/api/internal/auth"
)

func TestServiceDirectory(t *testing.T) {
	for _, url := range []string{"javascript:alert(1)", "/relative", "https://user:secret@git.test", "https://id.test/?token=secret"} {
		if _, err := ParseServiceLinks(`[{"name":"Service","url":"` + url + `"}]`); err == nil {
			t.Fatalf("accepted %s", url)
		}
	}
	links, err := ParseServiceLinks(`[{"name":"Gitea","url":"https://git.test"},{"name":"Authentik","url":"https://id.test"}]`)
	if err != nil {
		t.Fatal(err)
	}
	handler := as(auth.RoleUser, newHandler("test", nil, nil, links, options{}))
	for path, want := range map[string]string{"/v1/services": `"name":"Authentik"`, "/v1/plugins": `"plugins":[]`} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 200 || !strings.Contains(r.Body.String(), want) {
			t.Fatal(path, r.Code, r.Body.String())
		}
	}
}
