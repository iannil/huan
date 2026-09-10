package main

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"testing"
)

type bookNavTestFile struct {
	BaseFileName string
	Path         string
}

type bookNavTestPage struct {
	Title                 string
	Type                  string
	Section               string
	PageLanguage          string
	RelPermalink          string
	File                  bookNavTestFile
	Parent                *bookNavTestPage
	RegularPagesRecursive []*bookNavTestPage
}

func parseBookReadingNav(t *testing.T) *template.Template {
	t.Helper()
	content, err := templateFS.ReadFile("templates/partials/book-reading-nav.html")
	if err != nil {
		t.Fatalf("read book navigation partial: %v", err)
	}
	tmpl, err := template.New("book-reading-nav").Funcs(template.FuncMap{
		"slice": func(values ...interface{}) []interface{} { return values },
		"append": func(value interface{}, values []interface{}) []interface{} {
			return append(values, value)
		},
		"dict":      func() map[string]interface{} { return map[string]interface{}{} },
		"hasPrefix": strings.HasPrefix,
		"where": func(pages []*bookNavTestPage, field string, value interface{}) []interface{} {
			if field != "File.BaseFileName" {
				return nil
			}
			want := fmt.Sprint(value)
			result := make([]interface{}, 0)
			for _, page := range pages {
				if page.File.BaseFileName == want {
					result = append(result, page)
				}
			}
			return result
		},
		"sort": func(pages []*bookNavTestPage, field string) []interface{} {
			if field != "File.Path" {
				return nil
			}
			ordered := append([]*bookNavTestPage(nil), pages...)
			sort.Slice(ordered, func(i, j int) bool { return ordered[i].File.Path < ordered[j].File.Path })
			result := make([]interface{}, len(ordered))
			for i, page := range ordered {
				result[i] = page
			}
			return result
		},
	}).Parse(string(content))
	if err != nil {
		t.Fatalf("parse book navigation partial: %v", err)
	}
	return tmpl
}

func TestBookReadingNavChapterPositions(t *testing.T) {
	tmpl := parseBookReadingNav(t)
	book := &bookNavTestPage{
		Title:        "测试书",
		Type:         "book",
		Section:      "books",
		RelPermalink: "/books/test/",
	}
	chapters := []*bookNavTestPage{
		{Title: "第一章", Section: "books", RelPermalink: "/books/test/chapter-01/", File: bookNavTestFile{BaseFileName: "chapter-01", Path: "chapter-01.md"}, Parent: book},
		{Title: "第二章", Section: "books", RelPermalink: "/books/test/chapter-02/", File: bookNavTestFile{BaseFileName: "chapter-02", Path: "chapter-02.md"}, Parent: book},
		{Title: "第三章", Section: "books", RelPermalink: "/books/test/chapter-03/", File: bookNavTestFile{BaseFileName: "chapter-03", Path: "chapter-03.md"}, Parent: book},
	}
	book.RegularPagesRecursive = chapters

	tests := []struct {
		name       string
		page       *bookNavTestPage
		wantPrev   bool
		wantNext   bool
		wantTitles []string
	}{
		{name: "first chapter", page: chapters[0], wantNext: true, wantTitles: []string{"全书目录", "第二章"}},
		{name: "middle chapter", page: chapters[1], wantPrev: true, wantNext: true, wantTitles: []string{"第一章", "全书目录", "第三章"}},
		{name: "last chapter", page: chapters[2], wantPrev: true, wantTitles: []string{"第二章", "全书目录"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tmpl.Execute(&out, tt.page); err != nil {
				t.Fatalf("execute book navigation: %v", err)
			}
			html := out.String()
			if strings.Contains(html, `book-reading-nav__previous`) != tt.wantPrev {
				t.Fatalf("previous link presence mismatch: %s", html)
			}
			if strings.Contains(html, `book-reading-nav__next`) != tt.wantNext {
				t.Fatalf("next link presence mismatch: %s", html)
			}
			for _, title := range tt.wantTitles {
				if !strings.Contains(html, title) {
					t.Fatalf("navigation missing %q: %s", title, html)
				}
			}
			contents := strings.Index(html, `book-reading-nav__contents`)
			if tt.wantPrev && strings.Index(html, `book-reading-nav__previous`) > contents {
				t.Fatalf("previous link must precede contents: %s", html)
			}
			if tt.wantNext && contents > strings.Index(html, `book-reading-nav__next`) {
				t.Fatalf("contents must precede next link: %s", html)
			}
		})
	}
}

func TestBookReadingNavCSSContract(t *testing.T) {
	content, err := assetFS.ReadFile("assets/css/guide.css")
	if err != nil {
		t.Fatalf("read guide.css: %v", err)
	}
	css := string(content)
	for _, rule := range []string{
		"grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr)",
		"border-top: 1px solid #f2f2f2",
		"border-bottom: 1px solid #f2f2f2",
		".book-reading-nav__previous { grid-column: 1; text-align: left; }",
		".book-reading-nav__contents {",
		".book-reading-nav__next { grid-column: 3; text-align: right; }",
	} {
		if !strings.Contains(css, rule) {
			t.Fatalf("guide.css missing book navigation rule %q", rule)
		}
	}
}
