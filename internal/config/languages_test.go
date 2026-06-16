package config

import "testing"

func TestTopSection(t *testing.T) {
	cases := map[string]string{
		"/books/foo/":         "books",
		"products/x.md":       "products",
		"http://h/books/":     "books",
		"http://h/en/books/":  "en",
		"/":                   "",
		"":                    "",
		"https://host":        "",
		"/posts/2020/08/2.md": "posts",
	}
	for in, want := range cases {
		if got := TopSection(in); got != want {
			t.Errorf("TopSection(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsSectionExcludedForLang(t *testing.T) {
	cfg := &Config{
		Languages: map[string]LanguageConfig{
			"zh-cn": {Weight: 1, BaseURL: ""},
			"en":    {Weight: 2, BaseURL: "/en", ExcludeSections: []string{"books", "gallery"}},
		},
	}

	excluded := []string{"/books/", "/en/books/", "http://h/en/books/", "/gallery/foo/", "/en/gallery/"}
	for _, u := range excluded {
		if !cfg.IsSectionExcludedForLang("en", u) {
			t.Errorf("IsSectionExcludedForLang(en, %q) = false, want true", u)
		}
	}

	kept := []string{"/posts/", "/products/", "/en/products/x/", "/en/", "/"}
	for _, u := range kept {
		if cfg.IsSectionExcludedForLang("en", u) {
			t.Errorf("IsSectionExcludedForLang(en, %q) = true, want false", u)
		}
	}

	// Default language has no exclusions configured → never excluded.
	if cfg.IsSectionExcludedForLang("zh-cn", "/books/") {
		t.Error("zh-cn should not exclude books (no ExcludeSections)")
	}
	// Unknown language code → not excluded.
	if cfg.IsSectionExcludedForLang("fr", "/books/") {
		t.Error("unknown lang should not exclude")
	}
}
