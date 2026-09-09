package main

import "testing"

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
	}
	for _, c := range cases {
		if got := sanitizeFilename(c.in); got != c.want {
			t.Errorf("%s: sanitizeFilename(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
