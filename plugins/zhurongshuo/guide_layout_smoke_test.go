package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"strings"
	"testing"
)

// guideLayoutSmokeTestPage is the minimal context the guide layout needs.
// It mirrors the huan template.Context fields the template touches.
type guideLayoutSmokeTestPage struct {
	Title        string
	Type         string
	Kind         string
	Plain        string
	RawContent    string
	RelPermalink string
	File         *guideLayoutSmokeTestFile
	Site         *guideLayoutSmokeTestSite
}

type guideLayoutSmokeTestFile struct {
	Path         string
	Dir          string
	BaseFileName string
}

type guideLayoutSmokeTestSite struct{}

func (s *guideLayoutSmokeTestSite) GetPage(ref string) interface{} {
	if ref == "/books/demo-book/" {
		return map[string]interface{}{
			"Title":        "演示书",
			"RelPermalink": "/books/demo-book/",
		}
	}
	return map[string]interface{}{"RelPermalink": "", "Title": "", "File": (*guideLayoutSmokeTestFile)(nil)}
}

const guideSmokeGuideData = "```guide\nbook: demo-book\nsection: books\nthesis:\n  claim: \"世界是被建构的\"\n  puzzle: \"如果是被建构的，建构者是谁？\"\nmain_chart:\n  chart_type: funnel\n  title: \"建构漏斗\"\n  nodes:\n    - { label: \"感知\" }\n    - { label: \"建构\" }\nconcepts:\n  - name: \"建构\"\n    what: \"心智组装经验\"\n    why: \"没有组装就没有世界\"\nmap:\n  - part: \"part-01\"\n    note: \"第一部分\"\ntakeaways:\n  - \"世界是被建构的\"\n```\n"

// parseGuideLayout parses guide/single.html together with the chart partials
// and a minimal head/nav/footer/js/search stub set, mirroring how huan
// registers all templates in one namespace.
func parseGuideLayout(t *testing.T) *template.Template {
	t.Helper()
	tmpl := template.New("root")
	fm := guideLayoutTestFuncMap()
	fm["partial"] = func(name string, dot interface{}) (template.HTML, error) {
		// huan registers partials under both "partials/<name>" and "<name>"
		// (charts partials are invoked as "charts/<type>.html").
		// html/template.ParseFS names templates by file base name.
		base := name
		if i := strings.LastIndex(name, "/"); i >= 0 {
			base = name[i+1:]
		}
		for _, key := range []string{"partials/" + name, name, base} {
			if key != "" && tmpl.Lookup(key) != nil {
				var b bytes.Buffer
				if err := tmpl.ExecuteTemplate(&b, key, dot); err != nil {
					return "", err
				}
				return template.HTML(b.String()), nil
			}
		}
		return "", fmt.Errorf("partial %s not found", name)
	}
	tmpl.Funcs(fm)
	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	// Stubs for partials whose full versions pull in too much engine context.
	stubs := map[string]string{
		"partials/head.html":   `<!DOCTYPE html><html><head><title>{{ .Title }}</title></head>`,
		"partials/nav.html":    `<nav></nav>`,
		"partials/header.html": `<header></header>`,
		"partials/footer.html": `<footer></footer>`,
		"partials/js.html":     ``,
		"partials/search.html": ``,
	}
	for name, content := range stubs {
		if _, err := tmpl.New(name).Parse(content); err != nil {
			t.Fatalf("parse stub %s: %v", name, err)
		}
	}
	var files []string
	err = fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if _, isStub := stubs[path]; isStub {
			return nil
		}
		if strings.HasPrefix(path, "partials/charts/") || path == "guide/single.html" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk templates: %v", err)
	}
	if _, err := tmpl.ParseFS(sub, files...); err != nil {
		t.Fatalf("parse guide layout: %v", err)
	}
	return tmpl
}

// guideLayoutTestFuncMap reuses the chart smoke-test funcMap plus the extra
// engine funcs the guide layout relies on (i18n, failRender, guideChartTypes,
// strings_Replace, path_Base, merge, in, where, sort).
func guideLayoutTestFuncMap() template.FuncMap {
	fm := chartTestFuncMap()
	fm["i18n"] = func(key string) string { return key }
	fm["parseGuideYAML"] = parseGuideYAML
	fm["failRender"] = failRender
	fm["guideChartTypes"] = guideChartTypesFn
	fm["strings_Replace"] = strings.ReplaceAll
	fm["path_Base"] = func(p string) string {
		p = strings.TrimSuffix(p, "/")
		if i := strings.LastIndex(p, "/"); i >= 0 {
			return p[i+1:]
		}
		return p
	}
	fm["merge"] = func(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
		for k, v := range src {
			dst[k] = v
		}
		return dst
	}
	fm["in"] = func(haystack interface{}, needle string) bool {
		switch h := haystack.(type) {
		case []string:
			for _, s := range h {
				if s == needle {
					return true
				}
			}
		case []interface{}:
			for _, s := range h {
				if fmt.Sprintf("%v", s) == needle {
					return true
				}
			}
		}
		return false
	}
	fm["where"] = func(pages interface{}, field string, val string) interface{} { return pages }
	fm["sort"] = func(v interface{}) interface{} { return v }
	fm["append"] = func(slice interface{}, items ...interface{}) []interface{} {
		s, _ := slice.([]interface{})
		return append(s, items...)
	}
	fm["absURL"] = func(p string) string { return p }
	fm["slice"] = func(args ...interface{}) []interface{} { return args }
	fm["safeHTML"] = func(v interface{}) template.HTML { return template.HTML(fmt.Sprintf("%v", v)) }
	return fm
}

func TestGuideLayoutSmokeRender(t *testing.T) {
	tmpl := parseGuideLayout(t)
	page := &guideLayoutSmokeTestPage{
		Title:        "演示书 · 可视化导读",
		Type:         "guide",
		Kind:         "page",
		RawContent:    "前言文字。\n\n" + guideSmokeGuideData + "\n后记文字。",
		RelPermalink: "/books/demo-book/guide/",
		File:         &guideLayoutSmokeTestFile{Path: "books/demo-book/guide/index.md", Dir: "books/demo-book/guide/", BaseFileName: "index"},
		Site:         &guideLayoutSmokeTestSite{},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "single.html", page); err != nil {
		t.Fatalf("render guide layout: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"<svg",    // main chart SVG
		"世界是被建构的", // thesis claim
		"如果是被建构的，建构者是谁？",           // thesis puzzle
		"世界是被建构的</li>",             // takeaway
		`href="/books/demo-book/"`, // book link
		"guide_thesis", "guide_concept_card", "guide_map", "guide_takeaways", "guide_enter_book",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestGuideLayoutSmokeFailsOnBadChartType(t *testing.T) {
	tmpl := parseGuideLayout(t)
	bad := strings.Replace(guideSmokeGuideData, "chart_type: funnel", "chart_type: pie", 1)
	page := &guideLayoutSmokeTestPage{
		Title: "bad", Type: "guide", Kind: "page", RawContent: bad,
		RelPermalink: "/books/demo-book/guide/",
		File:         &guideLayoutSmokeTestFile{Path: "books/demo-book/guide/index.md", Dir: "books/demo-book/guide/", BaseFileName: "index"},
		Site:         &guideLayoutSmokeTestSite{},
	}
	var buf bytes.Buffer
	err := tmpl.ExecuteTemplate(&buf, "single.html", page)
	if err == nil {
		t.Fatal("expected render error for unsupported chart_type pie")
	}
	if !strings.Contains(err.Error(), "chart_type") {
		t.Errorf("error should mention chart_type: %v", err)
	}
}
