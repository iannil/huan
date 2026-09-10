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

func publicationNoticeConfig(enabled bool, pattern string) map[string]any {
	return map[string]any{
		"publicationRevisionNotice": map[string]any{
			"enabled":    enabled,
			"urlPattern": pattern,
			"content": map[string]any{
				"zh-cn": map[string]any{
					"ariaLabel": "状态",
					"title":     "RC预览版，非最终版本。",
					"body":      "中文说明。",
				},
				"en": map[string]any{
					"ariaLabel": "Status",
					"title":     "RC preview, not the final version.",
					"body":      "English notice.",
				},
			},
		},
	}
}

func TestParseConfigPublicationRevisionNotice(t *testing.T) {
	cfg, err := ParseConfig(publicationNoticeConfig(true, `^/(?:en/)?books/`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	notice := cfg.PublicationRevisionNotice
	if !notice.Enabled || notice.URLPattern != `^/(?:en/)?books/` || notice.urlRegexp == nil {
		t.Fatalf("unexpected notice config: %+v", notice)
	}
	if got := notice.Content["en"].Title; got != "RC preview, not the final version." {
		t.Fatalf("English title = %q", got)
	}
}

func TestParseConfigPublicationRevisionNoticeDefaultsDisabled(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig(nil): %v", err)
	}
	if cfg.PublicationRevisionNotice.Enabled {
		t.Fatal("notice must default to disabled")
	}
}

func TestParseConfigPublicationRevisionNoticeRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]any
		wantErr string
	}{
		{name: "wrong top-level type", raw: map[string]any{"publicationRevisionNotice": "yes"}, wantErr: "expected map"},
		{name: "enabled without pattern", raw: publicationNoticeConfig(true, ""), wantErr: "urlPattern: required"},
		{name: "invalid regexp", raw: publicationNoticeConfig(true, "["), wantErr: "urlPattern"},
		{
			name: "missing localized body",
			raw: map[string]any{"publicationRevisionNotice": map[string]any{
				"enabled": true, "urlPattern": "^/", "content": map[string]any{
					"en": map[string]any{"ariaLabel": "Status", "title": "Preview"},
				},
			}},
			wantErr: "content.en.body: required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig(tt.raw)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ParseConfig error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestPublicationRevisionNoticeIsEmbeddedAndConfigured(t *testing.T) {
	theme := New(&Config{})
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
	for _, contract := range []string{"publicationRevisionNotice .RelPermalink .PageLanguage", ".AriaLabel", ".Title", ".Body"} {
		if !strings.Contains(partial, contract) {
			t.Fatalf("notice partial is missing config contract %q", contract)
		}
	}
	for _, hardcoded := range []string{"RC预览版", "RC preview"} {
		if strings.Contains(partial, hardcoded) {
			t.Fatalf("notice partial must not hardcode content %q", hardcoded)
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

func TestPublicationRevisionNoticeRendering(t *testing.T) {
	cfg, err := ParseConfig(publicationNoticeConfig(true, `^/(?:en/)?books/`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	tmpl := parsePublicationRevisionNoticeTemplate(t, New(cfg))

	tests := []struct {
		name     string
		page     publicationNoticeTestPage
		wantText string
		visible  bool
	}{
		{name: "Chinese match", page: publicationNoticeTestPage{RelPermalink: "/books/volume-1/book/", PageLanguage: "zh-cn"}, wantText: "中文说明。", visible: true},
		{name: "English match", page: publicationNoticeTestPage{RelPermalink: "/en/books/volume-1/book/", PageLanguage: "en"}, wantText: "English notice.", visible: true},
		{name: "URL mismatch", page: publicationNoticeTestPage{RelPermalink: "/posts/example/", PageLanguage: "zh-cn"}, visible: false},
		{name: "language missing", page: publicationNoticeTestPage{RelPermalink: "/books/volume-1/book/", PageLanguage: "fr"}, visible: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tmpl.Execute(&out, tt.page); err != nil {
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

func TestPublicationRevisionNoticeDisabled(t *testing.T) {
	cfg, err := ParseConfig(publicationNoticeConfig(false, `^/`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	tmpl := parsePublicationRevisionNoticeTemplate(t, New(cfg))
	var out bytes.Buffer
	if err := tmpl.Execute(&out, publicationNoticeTestPage{RelPermalink: "/books/example/", PageLanguage: "en"}); err != nil {
		t.Fatalf("execute notice partial: %v", err)
	}
	if strings.Contains(out.String(), `class="publication-revision-notice"`) {
		t.Fatalf("disabled notice rendered: %s", out.String())
	}
}

func TestPublicationRevisionNoticeEscapesConfiguredContent(t *testing.T) {
	raw := publicationNoticeConfig(true, `^/`)
	content := raw["publicationRevisionNotice"].(map[string]any)["content"].(map[string]any)
	content["en"].(map[string]any)["title"] = `<script>alert("x")</script>`
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	tmpl := parsePublicationRevisionNoticeTemplate(t, New(cfg))
	var out bytes.Buffer
	if err := tmpl.Execute(&out, publicationNoticeTestPage{RelPermalink: "/", PageLanguage: "en"}); err != nil {
		t.Fatalf("execute notice partial: %v", err)
	}
	if strings.Contains(out.String(), "<script>") || !strings.Contains(out.String(), "&lt;script&gt;") {
		t.Fatalf("configured content was not escaped: %s", out.String())
	}
}

func TestInitPluginRejectsInvalidPublicationRevisionNoticeRegexp(t *testing.T) {
	_, err := InitPlugin(publicationNoticeConfig(true, "["))
	if err == nil || !strings.Contains(err.Error(), "urlPattern") {
		t.Fatalf("InitPlugin error = %v, want invalid urlPattern", err)
	}
}

func parsePublicationRevisionNoticeTemplate(t *testing.T, theme *ZhurongshuoTheme) *template.Template {
	t.Helper()
	content, err := templateFS.ReadFile("templates/partials/publication-revision-notice.html")
	if err != nil {
		t.Fatalf("read notice partial: %v", err)
	}
	tmpl, err := template.New("notice").Funcs(theme.FuncMap()).Parse(string(content))
	if err != nil {
		t.Fatalf("parse notice partial: %v", err)
	}
	return tmpl
}
