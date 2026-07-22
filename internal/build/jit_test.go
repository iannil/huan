package build

import "testing"

func TestResolveSourceFromURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"home", "/", "_index.md"},
		{"section", "/posts/", "posts/_index.md"},
		{"simple page", "/posts/hello/", "posts/hello.md"},
		{"nested page", "/posts/2026/new-year/", "posts/2026/new-year.md"},
		{"deep page", "/books/v1/ch1/", "books/v1/ch1.md"},
		{"explicit _index", "/posts/_index/", "posts/_index.md"},
		{"no trailing slash", "/posts/hello", "posts/hello.md"},
		{"leading+trailing slash stripped", "/posts/hello/", "posts/hello.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveSourceFromURL(tc.url)
			if got != tc.want {
				t.Errorf("resolveSourceFromURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}
