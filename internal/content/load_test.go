package content

import "testing"

func TestDetectLanguageFromFilename(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain md", "foo.md", ""},
		{"en sidecar", "foo.en.md", "en"},
		{"zh-cn sidecar", "foo.zh-cn.md", "zh-cn"},
		{"_index sidecar", "_index.en.md", "en"},
		{"index plain", "index.md", ""},
		{"no extension", "foo", ""},
		{"double suffix", "foo.bar.md", "bar"},
		{"numeric", "foo.001.md", "001"},
		{"too short", "foo.a.md", ""},
		{"too long", "foo.toolongsuffix.md", ""},
		{"uppercase rejected", "foo.EN.md", ""},
		{"special char rejected", "foo.en_US.md", ""},
		{"with path separator", "posts/2026/foo.en.md", "en"}, // base name only matters
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectLanguageFromFilename(tc.in)
			if got != tc.want {
				t.Errorf("detectLanguageFromFilename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPage_IsDefaultLanguage(t *testing.T) {
	tests := []struct {
		name        string
		pageLang    string
		defaultCode string
		want        bool
	}{
		{"empty matches default", "", "zh-cn", true},
		{"explicit match", "zh-cn", "zh-cn", true},
		{"explicit mismatch", "en", "zh-cn", false},
		{"empty matches en default", "", "en", true},
		{"en matches en", "en", "en", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Page{Language: tc.pageLang}
			got := p.IsDefaultLanguage(tc.defaultCode)
			if got != tc.want {
				t.Errorf("IsDefaultLanguage(%q) with default %q = %v, want %v",
					tc.pageLang, tc.defaultCode, got, tc.want)
			}
		})
	}
}
