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
	RawContent   string
	RelPermalink string
	File         *guideLayoutSmokeTestFile
	Site         *guideLayoutSmokeTestSite
}

type guideLayoutSmokeTestFile struct {
	Path         string
	Dir          string
	BaseFileName string
}

type guideLayoutSmokeTestSite struct {
	Data         map[string]interface{}
	LanguageCode string
}

// guideLayoutSmokeTestBookPage mirrors the book page fields the guide
// layout's map section touches (Title, RelPermalink, File.Dir,
// RegularPagesRecursive).
type guideLayoutSmokeTestBookPage struct {
	Title                 string
	RelPermalink          string
	File                  *guideLayoutSmokeTestFile
	RegularPagesRecursive []interface{}
}

func (s *guideLayoutSmokeTestSite) GetPage(ref string) interface{} {
	if ref == "/books/demo-book/" {
		return &guideLayoutSmokeTestBookPage{
			Title:        "演示书",
			RelPermalink: "/books/demo-book/",
			File:         &guideLayoutSmokeTestFile{Path: "books/demo-book/_index.md", Dir: "books/demo-book/", BaseFileName: "_index"},
			RegularPagesRecursive: []interface{}{
				&guideLayoutSmokeTestPage{
					Title: "第一章", RelPermalink: "/books/demo-book/part-01/chapter-01/",
					File: &guideLayoutSmokeTestFile{Path: "books/demo-book/part-01/chapter-01.md", Dir: "books/demo-book/part-01/", BaseFileName: "chapter-01"},
				},
			},
		}
	}
	return map[string]interface{}{"RelPermalink": "", "Title": "", "File": (*guideLayoutSmokeTestFile)(nil)}
}

// guideLayoutSmokeTestData mirrors data/books.yaml structure for
// part_titles lookup.
func guideLayoutSmokeTestData() map[string]interface{} {
	return map[string]interface{}{
		"books": map[string]interface{}{
			"part_titles": map[string]interface{}{
				"demo-book": map[string]interface{}{"part-01": "第一部：开篇"},
			},
		},
		"practices": map[string]interface{}{
			"part_titles": map[string]interface{}{},
		},
	}
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
		if strings.HasPrefix(path, "partials/charts/") || strings.HasPrefix(path, "partials/guide") || path == "guide/single.html" || path == "partials/guide_v2_body.html" {
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
	fm["fileExists"] = func(p string) bool { return false } // Task 4 manual override; none in smoke fixtures
	fm["parseGuideYAML"] = parseGuideYAML
	fm["failRender"] = failRender
	fm["guideChartTypes"] = guideChartTypesFn
	fm["strings_Replace"] = strings.ReplaceAll
	fm["trimSuffix"] = func(suffix, s string) string { return strings.TrimSuffix(s, suffix) }
	fm["isset"] = func(m interface{}, key string) bool {
		switch v := m.(type) {
		case map[string]interface{}:
			_, ok := v[key]
			return ok
		case map[interface{}]interface{}:
			_, ok := v[key]
			return ok
		}
		return false
	}
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
	fm["sort"] = func(v interface{}, _ ...string) interface{} { return v }
	// append mirrors engine appendSliceFunc: variadic; find the []interface{}
	// arg and append the rest (template pipes pass the slice plus items).
	fm["append"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("append requires at least 2 arguments")
		}
		var slice []interface{}
		sliceIdx := -1
		for i, a := range args {
			if s, ok := a.([]interface{}); ok {
				slice = s
				sliceIdx = i
				break
			}
		}
		if sliceIdx == -1 {
			return nil, fmt.Errorf("append: no slice argument")
		}
		for i, a := range args {
			if i != sliceIdx {
				slice = append(slice, a)
			}
		}
		return slice, nil
	}
	fm["printf"] = fmt.Sprintf
	fm["ne"] = func(a, b interface{}) bool { return fmt.Sprintf("%v", a) != fmt.Sprintf("%v", b) }
	fm["not"] = func(v interface{}) bool {
		return v == nil || v == false || v == "" || v == interface{}(0.0)
	}
	fm["or"] = func(args ...interface{}) interface{} {
		for _, a := range args {
			if !(a == nil || a == false || a == "") {
				return a
			}
		}
		if len(args) > 0 {
			return args[len(args)-1]
		}
		return nil
	}
	fm["and"] = func(args ...interface{}) interface{} {
		var last interface{} = true
		for _, a := range args {
			last = a
			if a == nil || a == false {
				return a
			}
		}
		return last
	}
	fm["absURL"] = func(p string) string { return p }
	fm["slice"] = func(args ...interface{}) []interface{} { return args }
	fm["safeHTML"] = func(v interface{}) template.HTML { return template.HTML(fmt.Sprintf("%v", v)) }
	return fm
}

// newGuideSmokePage builds a guide test page for the given raw guide YAML
// block, mirroring the v1 smoke test's page construction.
func newGuideSmokePage(raw string) *guideLayoutSmokeTestPage {
	return &guideLayoutSmokeTestPage{
		Title:        "演示书 · 可视化导读",
		Type:         "guide",
		Kind:         "page",
		RawContent:   "前言文字。\n\n" + raw + "\n后记文字。",
		RelPermalink: "/books/demo-book/guide/",
		File:         &guideLayoutSmokeTestFile{Path: "books/demo-book/guide/index.md", Dir: "books/demo-book/guide/", BaseFileName: "index"},
		Site:         &guideLayoutSmokeTestSite{Data: guideLayoutSmokeTestData()},
	}
}

func TestGuideLayoutSmokeRender(t *testing.T) {
	tmpl := parseGuideLayout(t)
	page := newGuideSmokePage(guideSmokeGuideData)
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
		Site:         &guideLayoutSmokeTestSite{Data: guideLayoutSmokeTestData()},
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

const guideSmokeGuideV2Data = "```guide\nbook: demo-book\nsection: books\nmeta:\n  reading_minutes: 8\nthesis:\n  claim: \"世界是被建构的\"\n  puzzle: \"如果是被建构的，建构者是谁？本书回答这个根本困惑。\"\nhook:\n  scene: \"清晨你睁眼，世界已经在那里等着。但它是怎么到的？\"\n  question: \"世界是谁搭好的？\"\nchapters:\n  - title: \"表层：可见的世界\"\n    intro: \"从可感知的日常表层出发，先看见我们习以为常的东西。\"\n    chart_type: layers\n    steps:\n      - label: \"感知\"\n        sublabel: \"感官接收\"\n        explain: \"感官持续接收外界信号，这是建构的原料。没有原料就没有后续。\"\n      - label: \"命名\"\n        sublabel: \"语言归类\"\n        explain: \"语言把信号归类命名，世界开始有形状。名字即边界。\"\n    pitfall:\n      - misread: \"看见的就是全部\"\n        fix: \"表层只是入口，深层机制尚未展开。\"\n  - title: \"中层：看不见的机制\"\n    intro: \"穿过表层，看承载世界运转的中层机制。\"\n    chart_type: flow\n    steps:\n      - label: \"归类\"\n        sublabel: \"模式抽取\"\n        explain: \"机制把散乱的例子收拢为模式。模式即捷径。\"\n      - label: \"运转\"\n        sublabel: \"规则执行\"\n        explain: \"规则一旦运转，就不再需要逐例审视。机制即省力。\"\n  - title: \"深层：建构的根基\"\n    intro: \"最后抵达根基处，看清建构本身如何可能。\"\n    chart_type: ladder\n    steps:\n      - label: \"奠基\"\n        sublabel: \"前提交换\"\n        explain: \"一切建构都始于不可再追问的前提。前提即选择。\"\n      - label: \"回望\"\n        sublabel: \"整体重构\"\n        explain: \"带着根基回望全程，世界显出它被搭好的痕迹。回望即理解。\"\nconcepts:\n  - name: \"建构\"\n    what: \"心智组装经验\"\n    why: \"没有组装就没有世界\"\nmap:\n  - part: \"part-01\"\n    note: \"第一部分\"\ntakeaways:\n  - \"世界是被建构的\"\nnext:\n  - label: \"读第一部\"\n    href: \"/books/demo-book/part-01/chapter-01/\"\n```\n"

func TestGuideLayoutV2Chapters(t *testing.T) {
	tmpl := parseGuideLayout(t)
	page := newGuideSmokePage(guideSmokeGuideV2Data)
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, "single.html", page); err != nil {
		t.Fatalf("render v2: %v", err)
	}
	out := b.String()
	for _, want := range []string{
		`guide_hook`,           // hook 区块
		`世界是谁搭好的？`,     // hook.question
		`guide_chapter`,        // 章节容器
		`表层：可见的世界`,      // chapter.title
		`guide_step__explain`,  // step 解释文字容器（与 SVG 内文字区分）
		`名字即边界。`,          // step.explain
		`guide_pitfall`,        // 误解区
		`guide_next`,           // 上手路径区
	} {
		if !strings.Contains(out, want) {
			t.Errorf("v2 output missing %q", want)
		}
	}
	// v2 不再渲染 v1 的主图标题结构
	if strings.Contains(out, `guide_mainchart`) {
		t.Error("v2 layout must not render v1 mainchart section")
	}
}

func TestGuideLayoutV1StillWorks(t *testing.T) {
	tmpl := parseGuideLayout(t)
	page := newGuideSmokePage(guideSmokeGuideData)
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, "single.html", page); err != nil {
		t.Fatalf("render v1: %v", err)
	}
	if !strings.Contains(b.String(), "guide_mainchart") {
		t.Error("v1 layout regression: mainchart missing")
	}
}
