package build

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestTimingsRecordsFailure(t *testing.T) {
	c := NewTimings()
	want := errors.New("stage failed")
	if err := c.Measure("en", "load content", func() error { return want }); err != want {
		t.Fatalf("error = %v", err)
	}
	got := c.Entries()
	if len(got) != 1 || !got[0].Failed || got[0].Scope != "en" {
		t.Fatalf("entries = %+v", got)
	}
}
func TestNilTimingsRunsStage(t *testing.T) {
	var c *Timings
	called := false
	if err := c.Measure("site", "load", func() error { called = true; return nil }); err != nil || !called {
		t.Fatal("nil collector must preserve execution")
	}
}

func TestTimingsConcurrentRecordAndCopy(t *testing.T) {
	c := NewTimings()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Record("en", "hook", time.Second, false) }()
	}
	wg.Wait()
	got := c.Entries()
	if len(got) != 100 {
		t.Fatalf("entries = %d", len(got))
	}
	got[0].Stage = "changed"
	if c.Entries()[0].Stage != "hook" {
		t.Fatal("Entries aliases collector")
	}
}
func TestTimingsBuildStages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "huan.yaml", "title: Timing fixture\nbaseURL: https://example.org/\nlanguageCode: en\n")
	writeFile(t, dir, "content/post.md", "---\ntitle: Post\n---\nHello\n")
	writeFile(t, dir, "layouts/_default/single.html", "<html><body>{{ .Content }}</body></html>")
	c := NewTimings()
	if _, err := BuildSite(Options{SourceDir: dir, OutputDir: filepath.Join(dir, "public"), Timings: c, Logf: func(string, ...any) {}}); err != nil {
		t.Fatal(err)
	}
	stages := map[string]bool{}
	for _, e := range c.Entries() {
		stages[e.Stage] = true
	}
	for _, stage := range []string{"load config", "load content", "markdown", "tree + taxonomy", "templates", "setup templates + writer/cleanup", "contexts", "pages render + write", "feeds + specials", "static copy", "static + finalize"} {
		if !stages[stage] {
			t.Errorf("missing %s: %v", stage, stages)
		}
	}
}
func TestTimingsReport(t *testing.T) {
	c := NewTimings()
	c.Record("en", "load", time.Second, true)
	var got string
	c.Report(func(format string, args ...any) { got += fmt.Sprintf(format, args...) })
	if got != "[timings] en/load 1s failed\n" {
		t.Fatalf("report=%q", got)
	}
}

func TestTimingsPageFailurePreservesResult(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "huan.yaml", "title: Timing fixture\nbaseURL: https://example.org/\n")
	writeFile(t, dir, "content/post.md", "---\ntitle: Post\n---\nHello\n")
	writeFile(t, dir, "layouts/_default/single.html", "{{ .DoesNotExist }}")
	c := NewTimings()
	result, err := BuildSite(Options{SourceDir: dir, OutputDir: filepath.Join(dir, "public"), Timings: c, Logf: func(string, ...any) {}})
	if err != nil || result.Errors == 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, entry := range c.Entries() {
		if entry.Stage == "pages render + write" {
			if !entry.Failed {
				t.Fatal("page failure not marked")
			}
			return
		}
	}
	t.Fatal("missing page stage")
}
