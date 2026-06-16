package audit

import (
	"strings"
	"testing"
)

const enPageHTML = `<!DOCTYPE html><html lang="en"><head><title>x</title>
<style>.a{color:red}</style></head><body>
<nav>祝融说。 Home About</nav>
<div class="content"><div class="post_page"><div class="post">
<div class="post_title post_detail_title"><h1>Hello World</h1></div>
<div class="post_content markdown">
<p>This is a fully translated English paragraph about software.</p>
<pre><code>// 设置回调 in code is fine
fmt.Println("你好")</code></pre>
<p>Another English sentence with an inline <code>配置</code> token.</p>
</div></div></div></div>
<footer>祝融说。 Copyright</footer>
<script>console.log("脚本里的中文不算")</script>
</body></html>`

const enPageLeakHTML = `<html lang="en"><body>
<div class="post_title"><h1>Title</h1></div>
<div class="post_content markdown">
<p>This paragraph mixes English with 大量未翻译的中文内容在正文里出现而且占比很高确实是问题。</p>
<p>更多中文段落继续出现这里几乎都是中文了完全没有翻译过来的样子很明显。</p>
</div></body></html>`

const zhPageHTML = `<html lang="zh-cn"><body>
<div class="post_title"><h1>你好世界</h1></div>
<div class="post_content markdown"><p>这是一段完整的中文正文，讲述软件相关的内容。</p></div>
</body></html>`

func TestParse_ExtractsLangTitleProseExcludingCode(t *testing.T) {
	p, err := Parse(strings.NewReader(enPageHTML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Lang != "en" {
		t.Errorf("Lang = %q, want en", p.Lang)
	}
	if p.Title != "Hello World" {
		t.Errorf("Title = %q, want Hello World", p.Title)
	}
	// Code-embedded CJK must be excluded from prose.
	if strings.Contains(p.Prose, "设置回调") || strings.Contains(p.Prose, "你好") || strings.Contains(p.Prose, "配置") {
		t.Errorf("prose should exclude <pre>/<code> CJK, got: %q", p.Prose)
	}
	if !strings.Contains(p.Prose, "fully translated English paragraph") {
		t.Errorf("prose missing English body: %q", p.Prose)
	}
}

func TestCheckEnglish_CleanPagePasses(t *testing.T) {
	p, _ := Parse(strings.NewReader(enPageHTML))
	if f := CheckEnglish("/en/x/", p.Prose, 0.2); f != nil {
		t.Errorf("clean English page flagged: %+v", f)
	}
}

func TestCheckEnglish_LeakFlagged(t *testing.T) {
	p, _ := Parse(strings.NewReader(enPageLeakHTML))
	f := CheckEnglish("/en/y/", p.Prose, 0.2)
	if f == nil {
		t.Fatal("expected EnglishHasChinese finding, got nil")
	}
	if f.Kind != KindEnglishHasChinese {
		t.Errorf("Kind = %q, want %q", f.Kind, KindEnglishHasChinese)
	}
	if f.Evidence == "" {
		t.Error("expected non-empty CJK evidence snippet")
	}
}

func TestCheckChinese_RealChinesePasses(t *testing.T) {
	p, _ := Parse(strings.NewReader(zhPageHTML))
	if f := CheckChinese("/x/", p.Prose); f != nil {
		t.Errorf("real Chinese page flagged: %+v", f)
	}
}

func TestCheckChinese_EnglishProseFlagged(t *testing.T) {
	prose := "This Chinese page is actually entirely written in English which is clearly a mistake " +
		"for a zh-cn page that should contain real Chinese prose instead of these plain English words here."
	f := CheckChinese("/z/", prose)
	if f == nil {
		t.Fatal("expected ChineseLooksEnglish finding, got nil")
	}
	if f.Kind != KindChineseLooksEnglish {
		t.Errorf("Kind = %q, want %q", f.Kind, KindChineseLooksEnglish)
	}
}

func TestCheckLanguage_ShortProseSkipped(t *testing.T) {
	if f := CheckEnglish("/en/short/", "短", 0.2); f != nil {
		t.Errorf("short prose should be skipped, got %+v", f)
	}
}

func TestComputeMissingEN(t *testing.T) {
	sources := []SourceEntry{
		{RelPath: "products/a.md", HasBody: true, HasEN: true},   // ok
		{RelPath: "products/b.md", HasBody: true, HasEN: false},  // missing
		{RelPath: "products/_index.md", HasBody: false, HasEN: false}, // expected (listing)
		{RelPath: "books/c.md", HasBody: true, HasEN: false},     // missing
	}
	got := ComputeMissingEN(sources)
	if len(got) != 2 {
		t.Fatalf("got %d missing, want 2: %+v", len(got), got)
	}
	// sorted by Ref: books/c.md before products/b.md
	if got[0].Ref != "books/c.md" || got[1].Ref != "products/b.md" {
		t.Errorf("unexpected/unsorted refs: %q, %q", got[0].Ref, got[1].Ref)
	}
	for _, f := range got {
		if f.Kind != KindMissingEN {
			t.Errorf("Kind = %q, want %q", f.Kind, KindMissingEN)
		}
	}
}

func TestComputeOrphanEN(t *testing.T) {
	got := ComputeOrphanEN([]string{"z/late.en.md", "a/early.en.md"})
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Ref != "a/early.en.md" {
		t.Errorf("not sorted: %q", got[0].Ref)
	}
}
