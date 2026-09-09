package main

import (
	"path/filepath"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain ascii", "Demo Book", "Demo Book"},
		{"chinese with fullwidth colon", "实在建构：从无限可能到有限确定", "实在建构：从无限可能到有限确定"},
		{"halfwidth colon to fullwidth", "Reality Construction: From A to B", "Reality Construction：From A to B"},
		{"windows illegal chars dropped", `a\b/c*d?e"f<g>h|i`, "abcdefghi"},
		{"control chars dropped", "a\x00b\x1fc", "abc"},
		{"whitespace collapsed", "a \t\n b", "a b"},
		{"leading trailing spaces and dots", "  ..name.. ", "name"},
		{"empty stays empty", "///", ""},
		{"only spaces", "   ", ""},
		{"fullwidth colon swallows space", "a： b", "a：b"},
	}
	for _, c := range cases {
		if got := sanitizeFilename(c.in); got != c.want {
			t.Errorf("%s: sanitizeFilename(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestFileStem(t *testing.T) {
	bilingual := &unit{
		baseName: "reality-construction",
		agg: &content.BookEntry{
			TitleZH: "实在建构：从无限可能到有限确定",
			TitleEN: "Reality Construction: From Infinite Possibility to Finite Certainty",
		},
	}
	if got := fileStem(bilingual, content.LangZH); got != "实在建构：从无限可能到有限确定" {
		t.Errorf("zh stem = %q", got)
	}
	if got := fileStem(bilingual, content.LangEN); got != "Reality Construction：From Infinite Possibility to Finite Certainty" {
		t.Errorf("en stem = %q", got)
	}

	// EN title missing: falls back to the ZH title, "-en" keeps files distinct.
	zhOnly := &unit{
		baseName: "demo-book",
		agg:      &content.BookEntry{TitleZH: "示范书", TitleEN: ""},
	}
	if got := fileStem(zhOnly, content.LangZH); got != "示范书" {
		t.Errorf("zh-only stem = %q", got)
	}
	if got := fileStem(zhOnly, content.LangEN); got != "示范书-en" {
		t.Errorf("zh-only en stem = %q", got)
	}

	// Identical titles: the en stem equals the zh stem, so "-en" applies too.
	sameTitles := &unit{
		baseName: "demo-book",
		agg:      &content.BookEntry{TitleZH: "示范书", TitleEN: "示范书"},
	}
	if got := fileStem(sameTitles, content.LangEN); got != "示范书-en" {
		t.Errorf("same-titles en stem = %q", got)
	}

	// Aggregate unit: synthesized titles from expandUnits flow through as-is.
	vol := &unit{
		baseName: "volume-1",
		agg:      &content.BookEntry{TitleZH: "第1卷合集", TitleEN: "Collected Books: Volume 1"},
	}
	if got := fileStem(vol, content.LangZH); got != "第1卷合集" {
		t.Errorf("volume zh stem = %q", got)
	}
	if got := fileStem(vol, content.LangEN); got != "Collected Books：Volume 1" {
		t.Errorf("volume en stem = %q", got)
	}

	// Both titles sanitize to empty: fall back to the slug (export never fails).
	blank := &unit{baseName: "demo-book", agg: &content.BookEntry{}}
	if got := fileStem(blank, content.LangZH); got != "demo-book" {
		t.Errorf("blank zh stem = %q", got)
	}
	if got := fileStem(blank, content.LangEN); got != "demo-book-en" {
		t.Errorf("blank en stem = %q", got)
	}
}

func TestOutPathUsesTitleStem(t *testing.T) {
	u := &unit{
		kind:     "books",
		dirName:  "individual",
		baseName: "demo-book",
		agg:      &content.BookEntry{TitleZH: "示范书", TitleEN: "Demo Book: A Story"},
	}
	wantZH := filepath.Join("out", "epub", "books", "individual", "示范书.epub")
	if got := outPath("out", u.kind, u.dirName, u, content.LangZH, "epub"); got != wantZH {
		t.Errorf("zh outPath = %q, want %q", got, wantZH)
	}
	wantEN := filepath.Join("out", "epub", "books", "individual", "Demo Book：A Story.epub")
	if got := outPath("out", u.kind, u.dirName, u, content.LangEN, "epub"); got != wantEN {
		t.Errorf("en outPath = %q, want %q", got, wantEN)
	}
}

func TestLegacyPathIsSlugBased(t *testing.T) {
	got := legacyPath("out", "books", "individual", "demo-book", content.LangZH, "epub")
	if want := filepath.Join("out", "epub", "books", "individual", "demo-book.epub"); got != want {
		t.Errorf("zh legacyPath = %q, want %q", got, want)
	}
	got = legacyPath("out", "books", "individual", "demo-book", content.LangEN, "epub")
	if want := filepath.Join("out", "epub", "books", "individual", "demo-book-en.epub"); got != want {
		t.Errorf("en legacyPath = %q, want %q", got, want)
	}
}
