package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKrokiRenderSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mermaid/svg" {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "graph TD\nA-->B\n" {
			t.Errorf("body = %q", string(body))
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write([]byte(`<?xml version="1.0"?>` + "\n" + `<svg xmlns="http://www.w3.org/2000/svg"><g/></svg>`))
	}))
	defer srv.Close()

	k := NewKrokiClient(srv.URL, 2*time.Second)
	svg, err := k.Render(context.Background(), "mermaid", "graph TD\nA-->B\n")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(svg, "<?xml") {
		t.Errorf("xml prolog not stripped: %q", svg)
	}
	if !strings.Contains(svg, `class="kroki"`) {
		t.Errorf("kroki class not added: %q", svg)
	}
}

func TestKrokiRenderErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("syntax error"))
	}))
	defer srv.Close()
	k := NewKrokiClient(srv.URL, 2*time.Second)
	if _, err := k.Render(context.Background(), "mermaid", "bad"); err == nil {
		t.Errorf("expected error on 400")
	}
}
