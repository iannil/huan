package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iannil/huan/internal/config"
)

func newTestHandler(t *testing.T, requireToken bool) http.Handler {
	t.Helper()
	return NewHandler(HandlerOptions{
		Cfg:          config.Defaults(),
		SourceDir:    t.TempDir(),
		Rebuild:      func() {},
		ServeURL:     "http://127.0.0.1:1313/",
		BindAddr:     "127.0.0.1",
		Token:        "test-secret",
		RequireToken: requireToken,
	})
}

func TestNewHandler_DisabledAuth_NoHeader_PassesMiddleware(t *testing.T) {
	srv := httptest.NewServer(newTestHandler(t, false))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/admin/api/anything")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatal("RequireToken=false must not gate /admin/api/* (401 means middleware still mounted)")
	}
}

func TestNewHandler_DefaultEnabled_MissingToken_Returns401(t *testing.T) {
	srv := httptest.NewServer(newTestHandler(t, true))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/admin/api/anything")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("RequireToken=true without header must 401, got %d", resp.StatusCode)
	}
}
