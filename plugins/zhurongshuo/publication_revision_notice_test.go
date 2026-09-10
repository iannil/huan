package main

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

type publicationNoticeTestPage struct {
	PageLanguage string
}

func TestPublicationRevisionNoticeIsEmbeddedAndUsed(t *testing.T) {
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
	if strings.Contains(partial, "hasPrefix") {
		t.Fatal("publication preview notice must not be restricted by route")
	}
	for _, text := range []string{"RC预览版，非最终版本", "RC preview, not the final version"} {
		if !strings.Contains(partial, text) {
			t.Fatalf("notice partial does not contain %q", text)
		}
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

func TestPublicationRevisionNoticeLanguage(t *testing.T) {
	content, err := templateFS.ReadFile("templates/partials/publication-revision-notice.html")
	if err != nil {
		t.Fatalf("read notice partial: %v", err)
	}
	tmpl, err := template.New("notice").Parse(string(content))
	if err != nil {
		t.Fatalf("parse notice partial: %v", err)
	}

	tests := []struct {
		name     string
		language string
		wantText string
	}{
		{name: "Chinese", language: "zh-cn", wantText: "RC预览版，非最终版本"},
		{name: "English", language: "en", wantText: "RC preview, not the final version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tmpl.Execute(&out, publicationNoticeTestPage{PageLanguage: tt.language}); err != nil {
				t.Fatalf("execute notice partial: %v", err)
			}
			if !strings.Contains(out.String(), `class="publication-revision-notice"`) {
				t.Fatalf("notice is not visible: %s", out.String())
			}
			if !strings.Contains(out.String(), tt.wantText) {
				t.Fatalf("notice output missing %q: %s", tt.wantText, out.String())
			}
		})
	}
}
