package template

import (
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

func TestDraftBookRecursiveContentsFollowBuildPolicy(t *testing.T) {
	book := &content.Page{RelPath: "books/volume-5/sample/_index.md", Draft: true}
	intro := &content.Page{RelPath: "books/volume-5/sample/introduction.md", Draft: true}
	chapter := &content.Page{RelPath: "books/volume-5/sample/part-01/chapter-01.md", Draft: true}
	public := &content.Page{RelPath: "books/volume-5/sample/part-01/chapter-02.md"}
	_, err := content.BuildTree([]*content.Page{book, intro, chapter, public}, &config.Config{LanguageCode: "zh-cn"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lookup := map[*content.Page]*Context{
		intro: {Title: "intro"}, chapter: {Title: "draft chapter"}, public: {Title: "public chapter"},
	}
	for _, tc := range []struct {
		name   string
		drafts bool
		want   int
	}{{"preview", true, 3}, {"production", false, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &Context{}
			LinkPageRelationships(ctx, book, lookup, tc.drafts)
			if got := len(ctx.RegularPagesRecursive); got != tc.want {
				t.Fatalf("recursive book contents = %d, want %d", got, tc.want)
			}
		})
	}
}
