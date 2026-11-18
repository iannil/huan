package desktop

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestDiagnosticsConcurrentAddSnapshot(t *testing.T) {
	d := NewDiagnostics()
	var wg sync.WaitGroup
	// Distinct per-call messages: Add dedupes identical (level, component,
	// msg) entries, so identical adds would collapse and the count assertion
	// below could no longer detect lost updates under parallel Add/Snapshot.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				d.Add(LevelWarn, "plugin", fmt.Sprintf("x-%d-%d", i, j))
				_ = d.Snapshot()
			}
		}(i)
	}
	wg.Wait()
	if got := len(d.Snapshot()); got != 400 {
		t.Fatalf("want 400 items, got %d", got)
	}
}

func TestDiagnosticsResetAndHasErrors(t *testing.T) {
	d := NewDiagnostics()
	if d.HasErrors() {
		t.Fatal("fresh diagnostics must have no errors")
	}
	d.Add(LevelWarn, "plugin", "w1")
	d.Add(LevelError, "serve", "e1")
	if !d.HasErrors() {
		t.Fatal("want HasErrors after error add")
	}
	d.Reset()
	if len(d.Snapshot()) != 0 || d.HasErrors() {
		t.Fatal("Reset must clear items and errors")
	}
}

func TestAdd_DeduplicatesIdenticalItems(t *testing.T) {
	d := NewDiagnostics()
	d.Add(LevelWarn, "plugin", "a.so: bad")
	d.Add(LevelWarn, "plugin", "a.so: bad") // identical — skipped
	d.Add(LevelWarn, "plugin", "b.so: bad") // distinct — collected
	items := d.Snapshot()
	if len(items) != 2 {
		t.Fatalf("dedup: got %d items, want 2", len(items))
	}
}

func TestDiagSummary(t *testing.T) {
	msg, detail := diagSummary(nil)
	if msg != "本次启动未发现问题" || detail != "" {
		t.Fatalf("empty summary = %q / %q", msg, detail)
	}
	items := []DiagItem{
		{Level: LevelError, Component: "serve", Message: "listen: busy"},
		{Level: LevelWarn, Component: "plugin", Message: "broken.so: bad format"},
	}
	msg, detail = diagSummary(items)
	if msg != "本次启动发现 2 个问题" {
		t.Errorf("message = %q", msg)
	}
	for _, want := range []string{"[错误] serve: listen: busy", "[警告] plugin: broken.so: bad format"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail missing %q, got:\n%s", want, detail)
		}
	}
}

func TestNotifyTitle(t *testing.T) {
	if got := notifyTitle(0, 2); got != "huan 已启动，有 2 个警告" {
		t.Errorf("warn-only title = %q", got)
	}
	if got := notifyTitle(1, 2); got != "huan 启动存在问题（1 个错误，2 个警告）" {
		t.Errorf("error title = %q", got)
	}
}

func TestTruncMsg(t *testing.T) {
	if got := truncMsg("short", 10); got != "short" {
		t.Errorf("short passthrough = %q", got)
	}
	long := strings.Repeat("字", 120)
	got := truncMsg(long, 100)
	if !strings.HasPrefix(got, strings.Repeat("字", 100)) || !strings.HasSuffix(got, "…") {
		t.Errorf("truncMsg cut wrong: len=%d suffix=%q", len([]rune(got)), got[len(got)-3:])
	}
	if len([]rune(got)) != 101 {
		t.Errorf("want 101 runes (100 + ellipsis), got %d", len([]rune(got)))
	}
	// silence unused import in edge builds
	_ = fmt.Sprint
}
