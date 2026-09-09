package main

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

type publicationNoticeTestPage struct {
	RelPermalink string
	PageLanguage string
}

func TestPublicationRevisionNoticeIsEmbeddedAndScoped(t *testing.T) {
	theme := &ZhurongshuoTheme{}
	var partial, single, bookList, guide string
	for _, entry := range theme.Templates() {
		switch entry["path"] {
		case "partials/publication-revision-notice.html":
			partial = entry["content"]
		case "_default/single.html":
			single = entry["content"]
		case "book/list.html":
			bookList = entry["content"]
		case "guide/single.html":
			guide = entry["content"]
		}
	}

	if partial == "" {
		t.Fatal("publication revision notice partial is not embedded")
	}
	for _, volume := range []string{"volume-1", "volume-2", "volume-3", "volume-4"} {
		if !strings.Contains(partial, volume) {
			t.Fatalf("notice partial does not cover %s", volume)
		}
	}
	if strings.Contains(partial, "volume-5") {
		t.Fatal("notice partial must not mark volume-5 as under this revision")
	}
	for name, body := range map[string]string{
		"default single": single,
		"book list":      bookList,
		"guide single":   guide,
	} {
		if !strings.Contains(body, `partial "publication-revision-notice.html" .`) {
			t.Fatalf("%s does not render the revision notice", name)
		}
	}
}

func TestPublicationRevisionNoticeRouteScope(t *testing.T) {
	content, err := templateFS.ReadFile("templates/partials/publication-revision-notice.html")
	if err != nil {
		t.Fatalf("read notice partial: %v", err)
	}
	tmpl, err := template.New("notice").Funcs(template.FuncMap{
		"hasPrefix": strings.HasPrefix,
	}).Parse(string(content))
	if err != nil {
		t.Fatalf("parse notice partial: %v", err)
	}

	tests := []struct {
		name     string
		url      string
		language string
		wantText string
		visible  bool
	}{
		{name: "Chinese volume 1", url: "/books/volume-1/book/", language: "zh-cn", wantText: "出版修订中", visible: true},
		{name: "Chinese volume 4", url: "/books/volume-4/book/guide/", language: "zh-cn", wantText: "出版修订中", visible: true},
		{name: "English volume 2", url: "/en/books/volume-2/book/", language: "en", wantText: "Publication revision in progress", visible: true},
		{name: "English volume 4", url: "/en/books/volume-4/book/guide/", language: "en", wantText: "Publication revision in progress", visible: true},
		{name: "Volume 5", url: "/books/volume-5/book/", language: "zh-cn", visible: false},
		{name: "Non-book page", url: "/posts/example/", language: "zh-cn", visible: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tmpl.Execute(&out, publicationNoticeTestPage{RelPermalink: tt.url, PageLanguage: tt.language}); err != nil {
				t.Fatalf("execute notice partial: %v", err)
			}
			visible := strings.Contains(out.String(), `class="publication-revision-notice"`)
			if visible != tt.visible {
				t.Fatalf("notice visibility = %v, want %v; output: %s", visible, tt.visible, out.String())
			}
			if tt.visible && !strings.Contains(out.String(), tt.wantText) {
				t.Fatalf("notice output missing %q: %s", tt.wantText, out.String())
			}
		})
	}
}
