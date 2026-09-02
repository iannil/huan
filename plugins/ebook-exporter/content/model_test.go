package content

import "testing"

func TestLangString(t *testing.T) {
	if Lang("zh").String() != "zh" || LangEN.String() != "en" {
		t.Fatal("lang string mismatch")
	}
}

func TestSectionOrder(t *testing.T) {
	// Assembly order must be: introduction, parts (in part-XX order),
	// epilogue, appendix — mirroring huan toc.go's writeBookToc.
	b := BookEntry{Slug: "demo", TitleZH: "Demo", TitleEN: "Demo"}
	b.Sections = []Section{
		{Type: "part", ID: "part-02", Title: "第二部"},
		{Type: "introduction", Title: "引言"},
		{Type: "appendix", Title: "附录"},
		{Type: "part", ID: "part-01", Title: "第一部"},
		{Type: "epilogue", Title: "结语"},
	}
	got := b.OrderedSections()
	want := []string{"introduction", "part-01", "part-02", "epilogue", "appendix"}
	for i := range want {
		if got[i].Type != want[i] && !(want[i] == "part-01" && got[i].ID == "part-01") && !(want[i] == "part-02" && got[i].ID == "part-02") {
			t.Fatalf("order[%d]: want %s, got %+v", i, want[i], got[i])
		}
	}
}
