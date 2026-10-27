package render

import "testing"

func TestEPUBRootLinksUseOrigin(t *testing.T) {
	if got := epubResolveLinks(`<a href="/posts/test/">test</a>`, nil, "https://press.example/sub/"); got != `<a href="https://press.example/posts/test/">test</a>` {
		t.Fatal(got)
	}
}

func TestEPUBRootLinks(t *testing.T) {
	targets := map[string]string{"/books/volume-5/book/part-01/chapter-01/": "part-01-ch01-2.xhtml"}
	input := `<a href="/books/volume-5/book/part-01/chapter-01/#test">内部</a><a href="/posts/test/">外部</a>`
	want := `<a href="part-01-ch01-2.xhtml#test">内部</a><a href="/posts/test/">外部</a>`
	if got := epubResolveLinks(input, targets); got != want {
		t.Fatalf("got %s", got)
	}
	if got := epubSourceURL("/tmp/site/content/books/volume-5/book/part-01/chapter-01.md"); got != "/books/volume-5/book/part-01/chapter-01/" {
		t.Fatal(got)
	}
}
