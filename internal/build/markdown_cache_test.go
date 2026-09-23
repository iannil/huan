package build

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/markdown"
	"github.com/iannil/huan/internal/shortcode"
)

func markdownTestPipeline(t testing.TB, c *MarkdownCache, cfg config.MarkupConfig) *pipeline {
	t.Helper()
	hash, err := markdownConfigDigest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &pipeline{opts: Options{MarkdownCache: c}, cfg: &config.Config{Markup: cfg}, md: markdown.NewRenderer(&cfg), markupHash: hash, scRegistry: shortcode.NewRegistry()}
}

func TestMarkdownCacheHitAndFreshPage(t *testing.T) {
	c := NewMarkdownCache(1 << 20)
	p := markdownTestPipeline(t, c, config.MarkupConfig{})
	first := &content.Page{RawContent: "Hello **world**.\n\n<!--more-->\n\nRest.", RelPath: "old.md"}
	if err := p.renderPageMarkdown(first); err != nil {
		t.Fatal(err)
	}
	wantContent, wantSummary := first.Content, first.Summary
	first.Content, first.Summary, first.Plain, first.WordCount = "mutated", "mutated", "mutated", 999
	next := &content.Page{RawContent: first.RawContent, RelPath: "new.md"}
	if err := p.renderPageMarkdown(next); err != nil {
		t.Fatal(err)
	}
	if next.Content != wantContent || next.Summary != wantSummary || next.Plain != "Hello world.\n\nRest.\n" || next.WordCount != 3 || next.RelPath != "new.md" {
		t.Fatalf("cached page = %+v", next)
	}
	if s := c.Stats(); s.Hits != 1 || s.Misses != 1 || s.Entries != 1 {
		t.Fatalf("stats = %+v", s)
	}
}

func TestMarkdownCacheInvalidation(t *testing.T) {
	c := NewMarkdownCache(1 << 20)
	cfg := config.MarkupConfig{}
	p := markdownTestPipeline(t, c, cfg)
	render := func(body string) *content.Page {
		pg := &content.Page{RawContent: body}
		if err := p.renderPageMarkdown(pg); err != nil {
			t.Fatal(err)
		}
		return pg
	}
	old := render(`"hello"`)
	changed := render(`"changed"`)
	if old.Content == changed.Content {
		t.Fatal("body edit returned stale HTML")
	}
	cfg.Goldmark.Extensions.Typographer = true
	p = markdownTestPipeline(t, c, cfg)
	if got := render(`"hello"`); got.Content == old.Content {
		t.Fatal("config edit returned stale HTML")
	}
	cfg.Goldmark.Renderer.Unsafe = true
	p = markdownTestPipeline(t, c, cfg)
	render(`"hello"`)
	if s := c.Stats(); s.Misses != 4 || s.Hits != 0 {
		t.Fatalf("full config not keyed: %+v", s)
	}
}

func TestMarkdownCacheShortcodesAlwaysRun(t *testing.T) {
	c := NewMarkdownCache(1 << 20)
	p := markdownTestPipeline(t, c, config.MarkupConfig{})
	value, calls := "first", 0
	p.scRegistry.Register("dynamic", func(*shortcode.Context) (string, error) { calls++; return value, nil })
	render := func(body string) *content.Page {
		pg := &content.Page{RawContent: body}
		if err := p.renderPageMarkdown(pg); err != nil {
			t.Fatal(err)
		}
		return pg
	}
	a := render("{{< dynamic >}}")
	render("{{< dynamic >}}")
	value = "second"
	b := render("{{< dynamic >}}")
	if a.Content == b.Content || calls != 3 {
		t.Fatalf("dynamic output stale: calls=%d, %s", calls, b.Content)
	}
	// Identical expanded content can have different raw summary marker positions.
	p.scRegistry.Register("same", func(*shortcode.Context) (string, error) { return "text\n\n<!--more-->\n\nrest", nil })
	shortcodeSummary := render("{{< same >}}")
	rawSummary := render("text\n\n<!--more-->\n\nrest")
	if shortcodeSummary.Content != rawSummary.Content || shortcodeSummary.Summary == rawSummary.Summary {
		t.Fatal("raw summary marker was not distinguished from identical expanded content")
	}
	if s := c.Stats(); s.Hits != 1 || s.Misses != 4 {
		t.Fatalf("stats=%+v", s)
	}
	p.scRegistry.Register("dynamic", func(*shortcode.Context) (string, error) { return "", errors.New("unavailable") })
	if err := p.renderPageMarkdown(&content.Page{RawContent: "{{< dynamic >}}"}); err == nil {
		t.Fatal("cached entry hid shortcode error")
	}
}

func TestMarkdownCacheLRUAndBounds(t *testing.T) {
	value := markdownValue{HTML: "abc", Plain: "abc", Summary: "abc", WordCount: 1}
	c := NewMarkdownCache(2 * value.size())
	a, b, d := markdownKey{Raw: [32]byte{1}}, markdownKey{Raw: [32]byte{2}}, markdownKey{Raw: [32]byte{3}}
	c.put(a, value)
	c.put(b, value)
	if _, ok := c.get(a); !ok {
		t.Fatal("missing a")
	}
	c.put(d, value)
	if _, ok := c.get(b); ok {
		t.Fatal("least recently used entry retained")
	}
	if _, ok := c.get(a); !ok {
		t.Fatal("recent entry evicted")
	}
	c.put(b, markdownValue{HTML: strings.Repeat("x", 4096)})
	if s := c.Stats(); s.Entries != 2 || s.Bytes > s.MaxBytes || s.Evictions != 1 {
		t.Fatalf("stats=%+v", s)
	}
	c.Clear()
	if s := c.Stats(); s.Bytes != 0 || s.Entries != 0 {
		t.Fatalf("clear stats=%+v", s)
	}
	for _, capacity := range []int64{0, -1, 1} {
		disabled := NewMarkdownCache(capacity)
		disabled.put(a, markdownValue{})
		if disabled.Stats().Entries != 0 {
			t.Fatal("tiny/disabled cache stored empty entry")
		}
	}
}

func TestMarkdownCacheConcurrent(t *testing.T) {
	c := NewMarkdownCache(4096)
	p := markdownTestPipeline(t, c, config.MarkupConfig{})
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				pg := &content.Page{RawContent: fmt.Sprintf("# Heading %d\n\nBody %d", n, i%3)}
				if err := p.renderPageMarkdown(pg); err != nil {
					t.Error(err)
				}
				if pg.WordCount != 4 {
					t.Errorf("word count=%d", pg.WordCount)
				}
				if i%7 == 0 {
					c.Clear()
				}
				if s := c.Stats(); s.Bytes > s.MaxBytes {
					t.Errorf("unbounded %+v", s)
				}
			}
		}(n)
	}
	wg.Wait()
}

func BenchmarkMarkdownCache(b *testing.B) {
	body := strings.Repeat("## Heading\n\nA paragraph with **bold**, `code`, 中文 and [a link](https://example.org).\n\n", 40)
	for _, mode := range []string{"disabled", "cold", "warm"} {
		b.Run(mode, func(b *testing.B) {
			var c *MarkdownCache
			if mode != "disabled" {
				c = NewMarkdownCache(16 << 20)
			}
			p := markdownTestPipeline(b, c, config.MarkupConfig{})
			if err := p.renderPageMarkdown(&content.Page{RawContent: body}); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if mode == "cold" {
					c.Clear()
				}
				if err := p.renderPageMarkdown(&content.Page{RawContent: body}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
