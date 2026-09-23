package build

import (
	"strings"
	"testing"
)

func TestMarkdownDiskPromotionAvoidsDuplicateStringAllocations(t *testing.T) {
	requireMarkdownDiskWrites(t)
	c, err := NewPersistentMarkdownCache(1<<20, t.TempDir(), "owned-promotion", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	key := markdownKey{Raw: [32]byte{71}}
	want := markdownValue{HTML: strings.Repeat("html", 256), Plain: strings.Repeat("plain", 256), Summary: strings.Repeat("summary", 256), WordCount: 256}
	c.put(key, want)
	cloning := testing.AllocsPerRun(20, func() {
		c.Clear()
		value, ok := c.disk.load(key)
		if !ok {
			t.Fatal("disk fixture missing")
		}
		c.putMemory(key, value)
	})
	promotion := testing.AllocsPerRun(20, func() {
		c.Clear()
		got, ok := c.get(key)
		if !ok || got != want {
			t.Fatal("disk promotion changed output")
		}
	})
	if promotion > cloning-3 {
		t.Fatalf("promotion allocated %.0f times versus %.0f for cloning; want to avoid three duplicate strings", promotion, cloning)
	}
	// Replacing a returned value's fields must not change the promoted cache.
	page, _ := c.get(key)
	page.HTML, page.Plain, page.Summary = "changed", "changed", "changed"
	if got, ok := c.get(key); !ok || got != want {
		t.Fatal("returned value mutation leaked into cache")
	}
}
