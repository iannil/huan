package main

import (
	"bytes"
	"embed"
	"html/template"
	"strings"
	"testing"
	"time"
)

//go:embed templates/partials/head.html
var headFS embed.FS

// headTestPage mirrors the huan template.Context fields head.html touches.
type headTestPage struct {
	Title         string
	Type          string
	IsPage        bool
	IsHome        bool
	Description   string
	Summary       string
	Keywords      []string
	Permalink     string
	RawContent    string
	Date          time.Time
	Lastmod       time.Time
	Params        map[string]interface{}
	Site          *headTestSite
	OutputFormats *headTestOutputFormats
}

type headTestSite struct {
	Title        string
	LanguageCode string
	Params       map[string]interface{}
	Taxonomies   map[string]interface{}
}

type headTestOutputFormats struct{}

type headTestOutputFormat struct {
	Rel       string
	MediaType struct{ Type string }
	Permalink string
}

func (o *headTestOutputFormats) Get(name string) *headTestOutputFormat {
	if name == "rss" {
		return nil
	}
	return nil
}

// parseHead parses the real head.html partial with the minimal funcMap the
// engine provides (custom funcs only; eq/ne/and/or/len/index/printf are
// html/template built-ins).
func parseHead(t *testing.T) *template.Template {
	t.Helper()
	tmpl := template.New("root").Funcs(template.FuncMap{
		"parseGuideYAML": parseGuideYAML,
		"safeHTML":       func(v interface{}) template.HTML { return template.HTML(strings.TrimSpace(string(rv(v)))) },
		"plainify":       func(s interface{}) string { return rv(s) },
		"isset": func(m interface{}, key string) bool {
			switch v := m.(type) {
			case map[string]interface{}:
				_, ok := v[key]
				return ok
			}
			return false
		},
		"first":  func(n int, s interface{}) interface{} { return s },
		"add":    func(a, b interface{}) int { return 0 },
		"absURL": func(p string) string { return p },
		"partial": func(name string, dot interface{}) (template.HTML, error) {
			return template.HTML(""), nil
		},
		"hreflang": func(dot interface{}) template.HTML { return template.HTML("") },
	})
	content, err := headFS.ReadFile("templates/partials/head.html")
	if err != nil {
		t.Fatalf("read head.html: %v", err)
	}
	// head.html references partial "schema.html"; provide a stub.
	if _, err := tmpl.New("partials/schema.html").Parse(``); err != nil {
		t.Fatalf("parse schema stub: %v", err)
	}
	if _, err := tmpl.New("partials/head.html").Parse(string(content)); err != nil {
		t.Fatalf("parse head.html: %v", err)
	}
	return tmpl
}

func newHeadTestPage() *headTestPage {
	return &headTestPage{
		Title:     "演示导读",
		Type:      "guide",
		IsPage:    true,
		Permalink: "/books/demo/guide/",
		RawContent: "```guide\nbook: demo\nsection: books\nthesis:\n  claim: \"演示主张\"\n  puzzle: \"演示困惑文本\"\nmain_chart:\n  chart_type: funnel\n  title: \"漏斗\"\n```\n",
		Date:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Lastmod:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Params:    map[string]interface{}{},
		Site: &headTestSite{
			Title:  "祝融说",
			Params: map[string]interface{}{"description": "站点描述"},
		},
		OutputFormats: &headTestOutputFormats{},
	}
}

func TestHeadGuideDescription(t *testing.T) {
	tmpl := parseHead(t)
	page := newHeadTestPage()
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, "partials/head.html", page); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := b.String()
	if !bytes.Contains([]byte(out), []byte(`content="演示主张`)) {
		t.Fatalf("og:description should start with thesis claim, got:\n%s", out)
	}
	if bytes.Contains([]byte(out), []byte("chart_type")) {
		t.Fatalf("og:description must not leak raw guide YAML:\n%s", out)
	}
}

func TestHeadGuideManualOverrideFallsBackToSiteDescription(t *testing.T) {
	tmpl := parseHead(t)
	page := newHeadTestPage()
	page.RawContent = "```guide\nbook: demo\nsection: books\n```\n"
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, "partials/head.html", page); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := b.String()
	if !bytes.Contains([]byte(out), []byte(`content="站点描述"`)) {
		t.Fatalf("manual-override guide page should fall back to site description, got:\n%s", out)
	}
	if bytes.Contains([]byte(out), []byte("chart_type")) {
		t.Fatalf("must not leak raw guide YAML:\n%s", out)
	}
}

func TestHeadFrontmatterDescriptionWins(t *testing.T) {
	tmpl := parseHead(t)
	page := newHeadTestPage()
	page.Description = "frontmatter 描述"
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, "partials/head.html", page); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := b.String()
	if !bytes.Contains([]byte(out), []byte(`content="frontmatter 描述"`)) {
		t.Fatalf("frontmatter description should win, got:\n%s", out)
	}
	if bytes.Contains([]byte(out), []byte("演示主张")) {
		t.Fatalf("thesis claim should not be used when frontmatter description exists:\n%s", out)
	}
}

// rv renders any value to its string form for stub funcs.
func rv(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
