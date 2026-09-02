package content

import "testing"

func TestSmokeZhurongshuo(t *testing.T) {
	t.Skip("manual smoke: remove Skip to run against the real repo")
	c, err := Discover("/Users/rong.zhu/Code/zhurong/zhurongshuo", "books")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, v := range c.Volumes {
		total += len(v.Books)
	}
	// data/books.yaml lists 25 slugs, but content/books/ currently contains
	// only 14 book directories (volume-5 absent; several volumes incomplete).
	// Discover mirrors toc.go and skips entries whose dir is missing.
	if total != 14 {
		t.Fatalf("expected 14 on-disk books, got %d", total)
	}
}
