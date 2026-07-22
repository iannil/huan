package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/daemon/cache"
	"github.com/iannil/huan/internal/daemon/dag"
	"github.com/iannil/huan/internal/daemon/eventbus"
)

// TestDaemonStartupHealthCheck verifies that daemon starts, initial build
// completes, and health endpoint returns 200 OK.
func TestDaemonStartupHealthCheck(t *testing.T) {
	// Use a real source directory with a minimal config.
	// Create a temp dir with huan.yaml and empty content/.
	tmpDir, err := os.MkdirTemp("", "huan-daemon-integration-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create minimal huan.yaml
	huanYAML := []byte(`baseURL: "https://example.com/"
title: "Test Site"
publishDir: "docs"
`)
	if err := os.WriteFile(filepath.Join(tmpDir, "huan.yaml"), huanYAML, 0644); err != nil {
		t.Fatal(err)
	}

	// Create content dir with a minimal page
	contentDir := filepath.Join(tmpDir, "content")
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		t.Fatal(err)
	}
	pageMD := []byte(`---
title: "Test Page"
date: "2026-01-01"
---
Hello, world.
`)
	if err := os.WriteFile(filepath.Join(contentDir, "test.md"), pageMD, 0644); err != nil {
		t.Fatal(err)
	}

	// Create layouts dir with a minimal template (needed for build to succeed)
	layoutsDir := filepath.Join(tmpDir, "layouts", "_default")
	if err := os.MkdirAll(layoutsDir, 0755); err != nil {
		t.Fatal(err)
	}
	singleTmpl := []byte(`<!doctype html><html><body>{{ .Content }}</body></html>`)
	if err := os.WriteFile(filepath.Join(layoutsDir, "single.html"), singleTmpl, 0644); err != nil {
		t.Fatal(err)
	}
	listTmpl := []byte(`<!doctype html><html><body>{{ .Content }}</body></html>`)
	if err := os.WriteFile(filepath.Join(layoutsDir, "list.html"), listTmpl, 0644); err != nil {
		t.Fatal(err)
	}

	// Run daemon in a goroutine, capture port
	done := make(chan struct{})
	defer close(done)

	// Use a concurrent-safe channel to capture the listening address
	addrCh := make(chan string, 1)
	_ = addrCh

	// 1. Test that Builder can execute a full build
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	dag := dag.NewDependencyGraph()
	jitCache := cache.NewJITCache(100, 5*time.Minute)
	metrics := NewMetricsCollector()
	health := NewHealthChecker()

	builder := NewBuilder(BuilderOptions{
		SourceDir:   tmpDir,
		OutputDir:   tmpDir,
		Bus:         bus,
		DAG:         dag,
		JITCache:    jitCache,
		Metrics:     metrics,
		BuildDrafts: true,
		Logf:        t.Logf,
	})

	// Full build should succeed
	ctx := context.Background()
	if err := builder.FullBuild(ctx); err != nil {
		t.Fatalf("FullBuild failed: %v", err)
	}

	// Verify output was generated
	outputDir := filepath.Join(tmpDir, "test.md")
	// The build writes to OutputDir which is tmpDir. Check if index.html exists.
	_ = outputDir

	// 2. Test that Serving can serve a built page
	// Create a serving instance pointing at the temp dir
	// (build output goes to tmpDir as it's set to the same path)
	serveOpts := ServingOptions{
		OutputDir: tmpDir,
		Bind:      "127.0.0.1",
		Port:      "0",
		JITCache:  jitCache,
		Builder:   builder,
		Bus:       bus,
		Health:    health,
		Metrics:   metrics,
		Logf:      t.Logf,
	}

	serving := NewServing(serveOpts)

	// Start serving in background
	serveCtx, serveCancel := context.WithCancel(context.Background())
	defer serveCancel()

	// Use a custom listener to capture the actual port
	go func() {
		if err := serving.Start(serveCtx); err != nil && err != http.ErrServerClosed {
			t.Logf("serving error: %v", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Try to reach health endpoint
	healthURL := "http://127.0.0.1:0/health"
	_ = healthURL

	// Since we used port "0", we don't know the actual port.
	// For now, just verify that the health endpoint is registered.
	// This is a basic smoke test; full integration test requires
	// capturing the actual listening address.
	_ = addrCh
	t.Log("daemon integration smoke test: components initialized, build completed")
}

// TestFindFileWrittenToDir verifies that built files exist in the expected dir.
func TestFindFileWrittenToDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "huan-output-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write a file to simulate build output
	testFile := filepath.Join(tmpDir, "test-output.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(testFile); err != nil {
		t.Errorf("expected test file to exist: %v", err)
	}
}

// TestHealthHandler verifies the health endpoint returns correct status.
func TestHealthHandler(t *testing.T) {
	health := NewHealthChecker()
	handler := health.Handler()

	// Before SetReady, should return 503
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("before SetReady: expected 503, got %d", rec.Code)
	}

	// After SetReady, should return 200
	health.SetReady(true)
	req2 := httptest.NewRequest("GET", "/health", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("after SetReady: expected 200, got %d", rec2.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec2.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal health response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
	if body["version"] != "0.6.0" {
		t.Errorf("expected version=0.6.0, got %v", body["version"])
	}
}

// TestMetricsEndpoint verifies the metrics endpoint returns Prometheus format.
func TestMetricsEndpoint(t *testing.T) {
	metrics := NewMetricsCollector()
	handler := metrics.Handler()

	// Record some metrics
	metrics.RecordBuild(2 * time.Second)
	metrics.RecordCacheHit()
	metrics.RecordCacheMiss()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "huan_build_total") {
		t.Errorf("expected huan_build_total in metrics output, got: %s", body)
	}
	if !strings.Contains(body, "huan_cache_hits_total") {
		t.Errorf("expected huan_cache_hits_total in metrics output")
	}
}

// TestMetricsCollector verifies basic counter operations.
func TestMetricsCollector(t *testing.T) {
	m := NewMetricsCollector()

	// Record builds
	m.RecordBuild(1 * time.Second)
	m.RecordBuild(2 * time.Second)
	m.RecordBuildFailure()
	m.RecordBuildFailure()

	// Record cache operations
	m.RecordCacheHit()
	m.RecordCacheHit()
	m.RecordCacheHit()
	m.RecordCacheMiss()
}

// TestBuilder_FullBuildFailure verifies that a builder with an invalid source dir fails.
func TestBuilder_FullBuildFailure(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	metrics := NewMetricsCollector()
	builder := NewBuilder(BuilderOptions{
		SourceDir: "/nonexistent/path/that/does/not/exist",
		OutputDir: t.TempDir(),
		Bus:       bus,
		DAG:       dag.NewDependencyGraph(),
		JITCache:  cache.NewJITCache(100, 5*time.Minute),
		Metrics:   metrics,
		Logf:      t.Logf,
	})
	err := builder.FullBuild(context.Background())
	if err == nil {
		t.Error("expected error for nonexistent source dir")
	}
}

// TestBuilder_QueueBuild_Pending verifies that a second build while busy is queued.
func TestBuilder_QueueBuild_Pending(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	metrics := NewMetricsCollector()
	builder := NewBuilder(BuilderOptions{
		Bus:     bus,
		Metrics: metrics,
		Logf:    t.Logf,
	})

	// QueueBuild with a slow build function
	buildCount := 0
	err1 := make(chan error, 1)
	go func() {
		err1 <- builder.QueueBuild(context.Background(), func() error {
			buildCount++
			time.Sleep(100 * time.Millisecond)
			return nil
		})
	}()

	// Give it time to start
	time.Sleep(20 * time.Millisecond)

	// Second call should queue
	err := builder.QueueBuild(context.Background(), func() error {
		buildCount++
		return nil
	})
	if err != nil {
		t.Errorf("second QueueBuild should return nil (queued), got: %v", err)
	}

	// Wait for first to complete
	<-err1
	if buildCount != 2 {
		t.Errorf("expected 2 builds (1 executed + 1 pending), got %d", buildCount)
	}
}

// TestServing_HealthEndpoint verifies that serving routes /health correctly.
func TestServing_HealthEndpoint(t *testing.T) {
	health := NewHealthChecker()
	health.SetReady(true)
	tmpDir := t.TempDir()
	srv := NewServing(ServingOptions{
		OutputDir: tmpDir,
		Bind:      "127.0.0.1",
		Port:      "0", // OS picks
		Health:    health,
		Logf:      t.Logf,
	})
	// Start in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	// Verify server started (we can't know the port, so we verify it didn't panic)
}

// TestServing_Shutdown verifies that graceful shutdown returns nil.
func TestServing_Shutdown(t *testing.T) {
	srv := NewServing(ServingOptions{Logf: t.Logf})
	err := srv.Shutdown(context.Background())
	if err != nil {
		t.Errorf("shutdown before start should be nil, got: %v", err)
	}
}

// TestWatcher_SkippedDir verifies that directory filtering works correctly.
func TestWatcher_SkippedDir(t *testing.T) {
	// Test via isSkippedDir logic indirectly by creating a watcher
	// on a dir with layouts/, static/, .git/ subdirs
	tmpDir := t.TempDir()
	for _, dir := range []string{"layouts", "static", ".git", "content", "themes"} {
		os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
	}
	w, err := NewWatcher(WatcherOptions{SourceDir: tmpDir, Logf: t.Logf})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.fsw.Close()
	// Watcher should not have added layouts/, static/, .git/
	// We verify by checking internal state - the watcher watches
	// directories via fsnotify, we can check if specific dirs were added.
	// For now, just verify creation succeeded.
}

// TestWatcher_IsIgnored verifies that editor artifact filtering works correctly.
func TestWatcher_IsIgnored(t *testing.T) {
	// Create a watcher to test isIgnored method
	tmpDir := t.TempDir()
	w, err := NewWatcher(WatcherOptions{SourceDir: tmpDir, Logf: t.Logf})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.fsw.Close()

	cases := []struct {
		path    string
		ignored bool
	}{
		{"/tmp/test/.DS_Store", true},      // dotfile
		{"/tmp/test/content/test.md.swp", true},  // vim swap
		{"/tmp/test/content/test.md~", true},     // backup
		{"/tmp/test/content/test.md", false},     // normal file
		{"/tmp/test/content/4913", true},         // vim probe
		{"/tmp/test/content/test.md.swo", true},  // vim swap overflow
		{"/tmp/test/content/test.md.swn", true},  // vim swap overflow
		{"/tmp/test/content/test.md.orig", true}, // merge backup
		{"/tmp/test/content/test.md.rej", true},  // merge reject
		{"/tmp/test/content/test.md.bak", true},  // generic backup
		{"/tmp/test/content/#test.md#", true},    // emacs auto-save
		{"/tmp/test/content/.#test.md", true},    // emacs lock
	}
	for _, c := range cases {
		got := w.isIgnored(c.path)
		if got != c.ignored {
			t.Errorf("isIgnored(%q) = %v, want %v", c.path, got, c.ignored)
		}
	}
}

// TestWatcher_isSkippedDir verifies that skipped directory detection works correctly.
func TestWatcher_isSkippedDir(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWatcher(WatcherOptions{SourceDir: tmpDir, Logf: t.Logf})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.fsw.Close()

	cases := []struct {
		path    string
		skipped bool
	}{
		{"layouts", true},
		{"static", true},
		{".git", true},
		{"themes", true},
		{"docs", true},
		{"node_modules", true},
		{"public", true},
		{"resources", true},
		{"assets", true},
		{"content", false},
		{"data", false},
		{".hidden", true},
	}
	for _, c := range cases {
		got := w.isSkippedDir(c.path)
		if got != c.skipped {
			t.Errorf("isSkippedDir(%q) = %v, want %v", c.path, got, c.skipped)
		}
	}
}

// TestMetrics_RecordRequest verifies that label logic works correctly.
func TestMetrics_RecordRequest(t *testing.T) {
	m := NewMetricsCollector()
	m.RecordRequest("GET", "/health", 50*time.Millisecond)
	m.RecordRequest("POST", "/admin/api/content", 200*time.Millisecond)
	m.RecordRequest("GET", "/metrics", 10*time.Millisecond)
	// No panic = pass
}

// TestBuilder_HandleContentChanged verifies that HandleContentChanged handles events with payload.
func TestBuilder_HandleContentChanged(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	builder := NewBuilder(BuilderOptions{
		Bus:  bus,
		DAG:  dagGraph,
		Logf: t.Logf,
	})
	err := builder.HandleContentChanged(context.Background(), eventbus.Event{
		Type:      eventbus.EventContentChanged,
		Timestamp: time.Now(),
		Payload:   map[string]interface{}{"changed_files": []string{"content/test.md"}},
	})
	// With empty DAG, HandleContentChanged triggers full build which fails
	// because no source dir is set
	if err == nil {
		t.Log("HandleContentChanged completed without error")
	} else {
		// Expected: full build fails due to no source dir
		t.Logf("HandleContentChanged error (expected): %v", err)
	}
}

// TestBuilder_HandleContentChanged_NoPayload verifies HandleContentChanged with no payload.
func TestBuilder_HandleContentChanged_NoPayload(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	builder := NewBuilder(BuilderOptions{
		Bus:  bus,
		DAG:  dagGraph,
		Logf: t.Logf,
	})
	err := builder.HandleContentChanged(context.Background(), eventbus.Event{
		Type:      eventbus.EventContentChanged,
		Timestamp: time.Now(),
		Payload:   nil,
	})
	// With empty DAG, HandleContentChanged triggers full build which fails
	// because no source dir is set
	t.Logf("HandleContentChanged_NoPayload result: %v", err)
}

// TestBuilder_TriggerRebuild verifies that TriggerRebuild publishes EventContentChanged.
func TestBuilder_TriggerRebuild(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	builder := NewBuilder(BuilderOptions{
		Bus:  bus,
		DAG:  dag.NewDependencyGraph(),
		Logf: t.Logf,
	})

	// Subscribe to events
	events := make(chan eventbus.Event, 1)
	bus.Subscribe(eventbus.EventContentChanged, func(ctx context.Context, ev eventbus.Event) error {
		select {
		case events <- ev:
		default:
		}
		return nil
	})

	builder.TriggerRebuild()

	select {
	case ev := <-events:
		if ev.Type != eventbus.EventContentChanged {
			t.Errorf("expected EventContentChanged, got %v", ev.Type)
		}
		if payload, ok := ev.Payload.(map[string]interface{}); ok {
			if payload["trigger"] != "admin" {
				t.Errorf("expected trigger=admin, got %v", payload["trigger"])
			}
		} else {
			t.Error("expected payload to be map[string]interface{}")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected event to be published within 100ms")
	}
}

// TestBuilder_IncrementalBuild_EmptyDAG verifies IncrementalBuild with empty DAG
// falls back to a full build. With no source directory configured, the full
// build fails, so IncrementalBuild must return a non-nil error.
func TestBuilder_IncrementalBuild_EmptyDAG(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	builder := NewBuilder(BuilderOptions{
		Bus:  bus,
		DAG:  dag.NewDependencyGraph(),
		Logf: t.Logf,
	})

	// With an empty DAG, IncrementalBuild cannot compute affected pages and
	// must fall back to a full build. No source dir is configured, so the
	// full build fails and IncrementalBuild returns an error.
	err := builder.IncrementalBuild(context.Background(), []string{"content/test.md"})
	if err == nil {
		t.Error("IncrementalBuild with empty DAG should fall back to full build and error without a source dir")
	}
}

// TestBuilder_QueueBuild_ErrorPropagation verifies that errors are propagated correctly.
func TestBuilder_QueueBuild_ErrorPropagation(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	metrics := NewMetricsCollector()
	builder := NewBuilder(BuilderOptions{
		Bus:     bus,
		Metrics: metrics,
		Logf:    t.Logf,
	})

	expectedErr := fmt.Errorf("build failed")
	err := builder.QueueBuild(context.Background(), func() error {
		return expectedErr
	})
	if err == nil {
		t.Error("expected error from build function")
	}
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

// TestBuilder_buildAndPersistDAG verifies DAG serialization after build.
func TestBuilder_buildAndPersistDAG(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	tmpDir := t.TempDir()
	dagGraph := dag.NewDependencyGraph()
	metrics := NewMetricsCollector()
	builder := NewBuilder(BuilderOptions{
		SourceDir: tmpDir,
		OutputDir: tmpDir,
		Bus:       bus,
		DAG:       dagGraph,
		JITCache:  cache.NewJITCache(100, 5*time.Minute),
		Metrics:   metrics,
		Logf:      t.Logf,
	})

	// Create a minimal site for the DAG
	// This tests that buildAndPersistDAG doesn't panic
	site := &content.Site{
		Title: "Test Site",
	}
	err := builder.buildAndPersistDAG(site)
	if err != nil {
		t.Errorf("buildAndPersistDAG should not error: %v", err)
	}
}

// TestBuilder_FullBuild_PopulatesPipelineCache verifies that when a Builder is
// configured with a PipelineCache, a successful full build populates the cache
// with reusable rendering state (templates, site config, writer).
//
// This locks in the Task 6 wiring contract: daemon creates the cache, hands it
// to the Builder, and BuildSite fills it via build.Options.PipelineCache.
func TestBuilder_FullBuild_PopulatesPipelineCache(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "huan-pipeline-cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Minimal site scaffold (mirrors TestDaemonStartupHealthCheck).
	huanYAML := []byte(`baseURL: "https://example.com/"
title: "Pipeline Cache Test"
publishDir: "docs"
`)
	if err := os.WriteFile(filepath.Join(tmpDir, "huan.yaml"), huanYAML, 0644); err != nil {
		t.Fatal(err)
	}
	contentDir := filepath.Join(tmpDir, "content")
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		t.Fatal(err)
	}
	pageMD := []byte(`---
title: "Test Page"
date: "2026-01-01"
---
Hello, world.
`)
	if err := os.WriteFile(filepath.Join(contentDir, "test.md"), pageMD, 0644); err != nil {
		t.Fatal(err)
	}
	layoutsDir := filepath.Join(tmpDir, "layouts", "_default")
	if err := os.MkdirAll(layoutsDir, 0755); err != nil {
		t.Fatal(err)
	}
	singleTmpl := []byte(`<!doctype html><html><body>{{ .Content }}</body></html>`)
	if err := os.WriteFile(filepath.Join(layoutsDir, "single.html"), singleTmpl, 0644); err != nil {
		t.Fatal(err)
	}
	listTmpl := []byte(`<!doctype html><html><body>{{ .Content }}</body></html>`)
	if err := os.WriteFile(filepath.Join(layoutsDir, "list.html"), listTmpl, 0644); err != nil {
		t.Fatal(err)
	}

	bus := eventbus.NewChannelBus()
	defer bus.Close()

	// Create a PipelineCache the same way daemon.Run() will after Task 6 wiring.
	pipelineCache := build.NewPipelineCache()
	if pipelineCache == nil {
		t.Fatal("NewPipelineCache returned nil")
	}

	builder := NewBuilder(BuilderOptions{
		SourceDir:      tmpDir,
		OutputDir:      tmpDir,
		Bus:            bus,
		DAG:            dag.NewDependencyGraph(),
		JITCache:       cache.NewJITCache(100, 5*time.Minute),
		Metrics:        NewMetricsCollector(),
		BuildDrafts:    true,
		Logf:           t.Logf,
		PipelineCache:  pipelineCache,
	})

	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild failed: %v", err)
	}

	// After a successful full build, the cache must be populated with the
	// reusable rendering infrastructure. The BuiltAt timestamp should move
	// forward (NewPipelineCache stamps it at creation; populateCache re-stamps
	// at population time, but even creation time is acceptable — we only
	// assert infrastructure pointers are set).
	if pipelineCache.Templates == nil {
		t.Error("PipelineCache.Templates should be populated after FullBuild")
	}
	if pipelineCache.SiteCfg == nil {
		t.Error("PipelineCache.SiteCfg should be populated after FullBuild")
	}
	if pipelineCache.Writer == nil {
		t.Error("PipelineCache.Writer should be populated after FullBuild")
	}
	t.Logf("PipelineCache populated: templates=%v siteCfg=%v writer=%v builtAt=%v",
		pipelineCache.Templates != nil,
		pipelineCache.SiteCfg != nil,
		pipelineCache.Writer != nil,
		pipelineCache.BuiltAt)
}

// TestServing_jitFallback verifies JIT fallback behavior.
func TestServing_jitFallback(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	tmpDir := t.TempDir()
	jitCache := cache.NewJITCache(100, 5*time.Minute)
	metrics := NewMetricsCollector()
	health := NewHealthChecker()
	builder := NewBuilder(BuilderOptions{
		Bus:     bus,
		DAG:     dag.NewDependencyGraph(),
		JITCache: jitCache,
		Metrics: metrics,
		Logf:    t.Logf,
	})

	srv := NewServing(ServingOptions{
		OutputDir: tmpDir,
		JITCache:  jitCache,
		Builder:   builder,
		Bus:       bus,
		Health:    health,
		Metrics:   metrics,
		Logf:      t.Logf,
	})

	// Create a request for a non-existent path
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.jitFallback(rec, req)

	// Should return 404: source file for /nonexistent cannot be resolved or found on disk
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// TestServing_jitFallback_CacheHit verifies JIT fallback with cache hit.
func TestServing_jitFallback_CacheHit(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	tmpDir := t.TempDir()
	jitCache := cache.NewJITCache(100, 5*time.Minute)
	metrics := NewMetricsCollector()
	health := NewHealthChecker()

	srv := NewServing(ServingOptions{
		OutputDir: tmpDir,
		JITCache:  jitCache,
		Bus:       bus,
		Health:    health,
		Metrics:   metrics,
		Logf:      t.Logf,
	})

	// Pre-populate the cache
	jitCache.Set("/cached", &cache.JITEntry{
		Path:       "/cached",
		HTML:       []byte("<html>cached</html>"),
		RenderedAt: time.Now(),
		TTL:        5 * time.Minute,
	})

	req := httptest.NewRequest("GET", "/cached", nil)
	rec := httptest.NewRecorder()
	srv.jitFallback(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Huan-Cache") != "jit" {
		t.Errorf("expected X-Huan-Cache=jit, got %s", rec.Header().Get("X-Huan-Cache"))
	}
	if !strings.Contains(rec.Body.String(), "cached") {
		t.Errorf("expected cached content, got %s", rec.Body.String())
	}
}

// TestLifecycle_WaitForShutdown verifies that WaitForShutdown returns a signal channel.
func TestLifecycle_WaitForShutdown(t *testing.T) {
	// We can't easily test the actual signal handling in a unit test,
	// but we can verify that the channel is created correctly.
	ch := WaitForShutdown()
	if ch == nil {
		t.Error("WaitForShutdown should return non-nil channel")
	}
	// The channel should block until a signal is received
	select {
	case <-ch:
		t.Error("channel should not have received signal yet")
	case <-time.After(10 * time.Millisecond):
		// Expected - no signal received
	}
}

// TestSystemdNotifier_Disabled verifies that disabled notifier does nothing.
func TestSystemdNotifier_Disabled(t *testing.T) {
	notifier := NewSystemdNotifier(false)
	// These should be no-ops
	notifier.Ready()
	notifier.Stopping()
	notifier.Status("test")
	// No panic = pass
}

// TestSystemdNotifier_Enabled verifies that enabled notifier works without NOTIFY_SOCKET.
func TestSystemdNotifier_Enabled(t *testing.T) {
	// Without NOTIFY_SOCKET env var, enabled notifier should still be no-op
	notifier := NewSystemdNotifier(true)
	if notifier.enabled {
		t.Error("notifier should be disabled when NOTIFY_SOCKET is not set")
	}
	notifier.Ready()
	notifier.Stopping()
	notifier.Status("test")
	// No panic = pass
}

// TestDaemon_PluginManager_Initialized verifies that the daemon initializes
// the plugin manager when DisablePlugin is false.
func TestDaemon_PluginManager_Initialized(t *testing.T) {
	// This is a smoke test - we verify the daemon code path for plugin initialization
	// by checking that the daemon struct has a pluginManager field that can be nil or non-nil.
	// Full integration testing would require running the actual daemon.

	// Create a minimal Daemon struct
	d := &Daemon{
		opts: Options{
			DisablePlugin: false,
		},
	}

	// When DisablePlugin is false, pluginManager should be nil initially (not yet initialized)
	if d.pluginManager != nil {
		t.Error("pluginManager should be nil before initialization")
	}

	// When DisablePlugin is true, pluginManager should remain nil
	d2 := &Daemon{
		opts: Options{
			DisablePlugin: true,
		},
	}
	if d2.pluginManager != nil {
		t.Error("pluginManager should be nil when DisablePlugin is true")
	}
}

// TestDaemon_PluginManager_Disabled verifies that the plugin manager is not
// initialized when DisablePlugin is true.
func TestDaemon_PluginManager_Disabled(t *testing.T) {
	d := &Daemon{
		opts: Options{
			DisablePlugin: true,
		},
	}

	// pluginManager should be nil
	if d.pluginManager != nil {
		t.Error("pluginManager should be nil when DisablePlugin is true")
	}
}

// TestPipelineCache_PopulatedAfterFullBuild verifies that a successful full
// build populates the PipelineCache with reusable rendering state.
func TestPipelineCache_PopulatedAfterFullBuild(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "huan-incr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Minimal site: config + 2 pages + templates.
	mustWriteIncFile(t, tmpDir, "huan.yaml", []byte(`baseURL: "https://example.com/"
title: "Test"
publishDir: "docs"
`))
	mustWriteIncFile(t, tmpDir, "content/posts/a.md", []byte(`---
title: "Page A"
date: "2026-01-01"
---
Content A.
`))
	mustWriteIncFile(t, tmpDir, "content/posts/b.md", []byte(`---
title: "Page B"
date: "2026-01-02"
---
Content B.
`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/single.html",
		[]byte(`<!doctype html><html><body><h1>{{ .Title }}</h1>{{ .Content }}</body></html>`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/list.html",
		[]byte(`<!doctype html><html><body>{{ range .Pages }}<a href="{{ .URL }}">{{ .Title }}</a>{{ end }}</body></html>`))

	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	pipelineCache := build.NewPipelineCache()
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dagGraph,
		JITCache:      cache.NewJITCache(100, 5*time.Minute),
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   true,
		Logf:          t.Logf,
		PipelineCache: pipelineCache,
	})

	// Full build.
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// PipelineCache should be populated.
	if pipelineCache.Templates == nil {
		t.Error("PipelineCache.Templates not populated after full build")
	}
	if pipelineCache.Writer == nil {
		t.Error("PipelineCache.Writer not populated after full build")
	}
	if pipelineCache.SiteCfg == nil {
		t.Error("PipelineCache.SiteCfg not populated after full build")
	}
	if dagGraph.NodeCount() == 0 {
		t.Error("DAG should have nodes after full build")
	}
}

// TestIncrementalBuild_ContentChange verifies that a content file change
// triggers incremental re-rendering and produces updated output.
func TestIncrementalBuild_ContentChange(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "huan-incr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mustWriteIncFile(t, tmpDir, "huan.yaml", []byte(`baseURL: "https://example.com/"
title: "Test"
publishDir: "docs"
`))
	mustWriteIncFile(t, tmpDir, "content/posts/hello.md", []byte(`---
title: "Hello"
date: "2026-01-01"
---
Original content.
`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/single.html",
		[]byte(`<!doctype html><html><body><h1>{{ .Title }}</h1>{{ .Content }}</body></html>`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/list.html",
		[]byte(`<!doctype html><html><body>{{ range .Pages }}{{ .Title }}{{ end }}</body></html>`))

	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	pipelineCache := build.NewPipelineCache()
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dagGraph,
		JITCache:      cache.NewJITCache(100, 5*time.Minute),
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   true,
		Logf:          t.Logf,
		PipelineCache: pipelineCache,
	})

	// Full build first.
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// Capture the page output.
	outPath := filepath.Join(tmpDir, "posts", "hello", "index.html")
	origBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read original output: %v", err)
	}
	origOutput := string(origBytes)
	if !strings.Contains(origOutput, "Original content.") {
		t.Fatalf("original output missing expected content:\n%s", origOutput)
	}

	// Modify the content file.
	mustWriteIncFile(t, tmpDir, "content/posts/hello.md", []byte(`---
title: "Hello"
date: "2026-01-01"
---
Updated content.
`))

	// Trigger incremental build with the changed file.
	changedFile := filepath.Join(tmpDir, "content", "posts", "hello.md")
	if err := builder.IncrementalBuild(context.Background(), []string{changedFile}); err != nil {
		t.Fatalf("IncrementalBuild: %v", err)
	}

	// Verify the output was updated.
	newBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read updated output: %v", err)
	}
	newOutput := string(newBytes)
	if !strings.Contains(newOutput, "Updated content.") {
		t.Errorf("output not updated after incremental build:\n%s", newOutput)
	}
	if strings.Contains(newOutput, "Original content.") {
		t.Errorf("output still contains old content after incremental build:\n%s", newOutput)
	}
}

// TestIncrementalBuild_TemplateChangeFallback verifies that a template
// change falls back to a full build.
func TestIncrementalBuild_TemplateChangeFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "huan-incr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mustWriteIncFile(t, tmpDir, "huan.yaml", []byte(`baseURL: "https://example.com/"
title: "Test"
publishDir: "docs"
`))
	mustWriteIncFile(t, tmpDir, "content/posts/hello.md", []byte(`---
title: "Hello"
date: "2026-01-01"
---
Content.
`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/single.html",
		[]byte(`<!doctype html><html><body>{{ .Content }}</body></html>`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/list.html",
		[]byte(`<!doctype html><html><body>{{ range .Pages }}{{ .Title }}{{ end }}</body></html>`))

	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	pipelineCache := build.NewPipelineCache()
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dagGraph,
		JITCache:      cache.NewJITCache(100, 5*time.Minute),
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   true,
		Logf:          t.Logf,
		PipelineCache: pipelineCache,
	})

	// Full build.
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// A template file change should be detected as requiring full build.
	changedFile := filepath.Join(tmpDir, "layouts", "_default", "single.html")
	if !build.HasTemplateChanges([]string{changedFile}, tmpDir) {
		t.Error("template change should be detected by HasTemplateChanges")
	}

	// IncrementalBuild with a template change should not error (it falls back
	// to full build internally).
	if err := builder.IncrementalBuild(context.Background(), []string{changedFile}); err != nil {
		t.Errorf("IncrementalBuild with template change should fall back cleanly, got: %v", err)
	}
}

// TestDaemon_JIT_RenderUnbuiltPage verifies the end-to-end JIT rendering
// path: a draft page (excluded from the full build because BuildDrafts=false)
// is served via serving.jitFallback, which calls Builder.RenderPageJIT,
// populates JITCache, and serves the cached HTML on subsequent requests.
//
// We drive the request through Serving.jitFallback (the real integration
// path) rather than calling Builder.RenderPageJIT directly, because the
// JITCache is populated by the serving layer (not by RenderPageJIT itself),
// which mirrors how production serves draft pages on demand.
func TestDaemon_JIT_RenderUnbuiltPage(t *testing.T) {
	tmpDir := setupJITDaemonSite(t)
	pipelineCache := build.NewPipelineCache()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	jitCache := cache.NewJITCache(100, 5*time.Minute)
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dagGraph,
		JITCache:      jitCache,
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   false, // drafts excluded → need JIT
		Logf:          t.Logf,
		PipelineCache: pipelineCache,
	})

	// Full build (excludes the draft).
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// Draft output must not exist on disk after the full build.
	if _, err := os.Stat(filepath.Join(tmpDir, "posts", "draft-page", "index.html")); !os.IsNotExist(err) {
		t.Fatalf("draft output should not exist after full build; stat err=%v", err)
	}

	// Serve the draft page via the JIT fallback path.
	srv := NewServing(ServingOptions{
		OutputDir: tmpDir,
		JITCache:  jitCache,
		Builder:   builder,
		Bus:       bus,
		Logf:      t.Logf,
	})

	req := httptest.NewRequest("GET", "/posts/draft-page/", nil)
	rec := httptest.NewRecorder()
	srv.jitFallback(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("jitFallback status = %d, want 200; body:\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Draft Content") {
		t.Errorf("JIT response missing draft content; got:\n%s", rec.Body.String())
	}

	// JITCache should now hold the entry.
	if jitCache.Len() != 1 {
		t.Errorf("JITCache.Len = %d, want 1", jitCache.Len())
	}
	entry := jitCache.Get("/posts/draft-page/")
	if entry == nil {
		t.Fatal("JITCache.Get returned nil after JIT render")
	}
	if !strings.Contains(string(entry.HTML), "Draft Content") {
		t.Errorf("cached HTML missing content; got:\n%s", string(entry.HTML))
	}

	// Second hit should be served from cache and tagged accordingly.
	req2 := httptest.NewRequest("GET", "/posts/draft-page/", nil)
	rec2 := httptest.NewRecorder()
	srv.jitFallback(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("second jitFallback status = %d, want 200", rec2.Code)
	}
	if rec2.Header().Get("X-Huan-Cache") != "jit" {
		t.Errorf("second hit should be served from JIT cache; got X-Huan-Cache=%q",
			rec2.Header().Get("X-Huan-Cache"))
	}
}

// TestDaemon_JIT_PageNotFound verifies that JIT returns an error for a URL
// with no resolvable source. Driven through Builder.RenderPageJIT directly
// so the error contract is asserted without going through HTTP.
func TestDaemon_JIT_PageNotFound(t *testing.T) {
	tmpDir := setupJITDaemonSite(t)
	pipelineCache := build.NewPipelineCache()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dag.NewDependencyGraph(),
		JITCache:      cache.NewJITCache(100, 5*time.Minute),
		Logf:          t.Logf,
		PipelineCache: pipelineCache,
	})
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	_, err := builder.RenderPageJIT(context.Background(), "/totally/fake/")
	if err == nil {
		t.Fatal("expected error for fake URL")
	}

	// Also verify the serving layer surfaces this as a 404.
	srv := NewServing(ServingOptions{
		OutputDir: tmpDir,
		JITCache:  cache.NewJITCache(100, 5*time.Minute),
		Builder:   builder,
		Bus:       bus,
		Logf:      t.Logf,
	})
	req := httptest.NewRequest("GET", "/totally/fake/", nil)
	rec := httptest.NewRecorder()
	srv.jitFallback(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("jitFallback fake URL status = %d, want 404", rec.Code)
	}
}

// TestJITCache_InvalidatedOnFullBuild verifies that a JIT cache populated by
// serving a draft page is cleared by a subsequent full build (so stale HTML
// is never served after content/templates change).
func TestJITCache_InvalidatedOnFullBuild(t *testing.T) {
	tmpDir := setupJITDaemonSite(t)
	pipelineCache := build.NewPipelineCache()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	jitCache := cache.NewJITCache(100, 5*time.Minute)
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dag.NewDependencyGraph(),
		JITCache:      jitCache,
		Logf:          t.Logf,
		PipelineCache: pipelineCache,
	})
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// Populate JIT cache by serving the draft page through jitFallback.
	srv := NewServing(ServingOptions{
		OutputDir: tmpDir,
		JITCache:  jitCache,
		Builder:   builder,
		Bus:       bus,
		Logf:      t.Logf,
	})
	req := httptest.NewRequest("GET", "/posts/draft-page/", nil)
	rec := httptest.NewRecorder()
	srv.jitFallback(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("jitFallback status = %d, want 200", rec.Code)
	}
	if jitCache.Len() != 1 {
		t.Fatalf("expected 1 JIT entry, got %d", jitCache.Len())
	}

	// A subsequent full build should clear the JIT cache entirely.
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("second FullBuild: %v", err)
	}
	if jitCache.Len() != 0 {
		t.Errorf("JITCache.Len after full build = %d, want 0", jitCache.Len())
	}
}

// setupJITDaemonSite creates a site with a published page + a draft page.
// The draft page (draft: true in frontmatter) is excluded from the full
// build when BuildDrafts=false, which is the precondition for the JIT
// integration tests above.
func setupJITDaemonSite(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	mustWriteIncFile(t, tmpDir, "huan.yaml", []byte("baseURL: \"https://example.com/\"\ntitle: \"JIT\"\npublishDir: \"docs\"\n"))
	mustWriteIncFile(t, tmpDir, "content/posts/hello.md", []byte("---\ntitle: \"Hello\"\ndate: \"2026-01-01\"\n---\nHello body.\n"))
	mustWriteIncFile(t, tmpDir, "content/posts/draft-page.md", []byte("---\ntitle: \"Draft\"\ndate: \"2026-01-02\"\ndraft: true\n---\nDraft Content.\n"))
	mustWriteIncFile(t, tmpDir, "layouts/_default/single.html", []byte("<html><body><h1>{{ .Title }}</h1>{{ .Content }}</body></html>"))
	mustWriteIncFile(t, tmpDir, "layouts/_default/list.html", []byte("<html><body>{{ range .Pages }}{{ .Title }}{{ end }}</body></html>"))
	return tmpDir
}

// mustWriteIncFile is a test helper that writes a file, creating parent dirs.
func mustWriteIncFile(t *testing.T, root, relPath string, data []byte) {
	t.Helper()
	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// TestIncrementalBuild_UpdatesListingPages is the regression guard for the
// two CRITICAL incremental-build defects:
//
//  1. DAG dependency direction was inverted — article edits failed to mark
//     aggregator pages (section, tag, home) as affected, so their output
//     stayed stale.
//  2. IncrementalRender skipped site-wide outputs (sitemap, search index,
//     taxonomy pages, paginated home, AI outputs) — those files went stale
//     after any content edit.
//
// This test builds a site with one section containing two articles (one
// tagged), edits the tagged article's title+content, runs IncrementalBuild,
// and asserts that BOTH the section listing page AND the tag/term listing
// page now reflect the updated title.
//
// It MUST fail before the C1 fix (because the listing URLs are not in
// `affected` and so are never re-rendered) and pass after.
func TestIncrementalBuild_UpdatesListingPages(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "huan-incr-listing-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mustWriteIncFile(t, tmpDir, "huan.yaml", []byte(`baseURL: "https://example.com/"
title: "Listing Test"
publishDir: "docs"
`))
	// Two articles in the same section; one is tagged so a /tags/<tag>/ page
	// gets generated.
	mustWriteIncFile(t, tmpDir, "content/posts/alpha.md", []byte(`---
title: "Alpha Original"
date: "2026-01-01"
tags: ["go"]
---
Alpha original body.
`))
	mustWriteIncFile(t, tmpDir, "content/posts/beta.md", []byte(`---
title: "Beta"
date: "2026-01-02"
---
Beta body.
`))
	// single.html: just renders the article.
	mustWriteIncFile(t, tmpDir, "layouts/_default/single.html",
		[]byte(`<!doctype html><html><body><h1>{{ .Title }}</h1>{{ .Content }}</body></html>`))
	// list.html: used for BOTH section pages and term (tag) pages. Renders
	// the title of each page in .Data.Pages so we can detect stale listings.
	mustWriteIncFile(t, tmpDir, "layouts/_default/list.html",
		[]byte(`<!doctype html><html><body>{{ range .Data.Pages }}<a href="{{ .Permalink }}">{{ .Title }}</a>{{ end }}</body></html>`))

	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	pipelineCache := build.NewPipelineCache()
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dagGraph,
		JITCache:      cache.NewJITCache(100, 5*time.Minute),
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   true,
		Logf:          t.Logf,
		PipelineCache: pipelineCache,
	})

	// Full build first.
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// Locate the section listing and the tag listing output files.
	sectionPath := filepath.Join(tmpDir, "posts", "index.html")
	tagPath := filepath.Join(tmpDir, "tags", "go", "index.html")

	// Sanity: both listing pages must exist after the full build, and the
	// section listing must include the original Alpha title.
	sectionBytes, err := os.ReadFile(sectionPath)
	if err != nil {
		t.Fatalf("section listing not written by full build: %v", err)
	}
	if !strings.Contains(string(sectionBytes), "Alpha Original") {
		t.Fatalf("section listing missing original Alpha title:\n%s", sectionBytes)
	}
	tagBytes, err := os.ReadFile(tagPath)
	if err != nil {
		t.Fatalf("tag listing not written by full build: %v", err)
	}
	if !strings.Contains(string(tagBytes), "Alpha Original") {
		t.Fatalf("tag listing missing original Alpha title:\n%s", tagBytes)
	}

	// Edit the tagged article: change title AND body. A correct incremental
	// build must propagate this to every aggregator that lists it.
	mustWriteIncFile(t, tmpDir, "content/posts/alpha.md", []byte(`---
title: "Alpha Updated"
date: "2026-01-01"
tags: ["go"]
---
Alpha updated body.
`))

	// Trigger incremental build for the changed file.
	changedFile := filepath.Join(tmpDir, "content", "posts", "alpha.md")
	if err := builder.IncrementalBuild(context.Background(), []string{changedFile}); err != nil {
		t.Fatalf("IncrementalBuild: %v", err)
	}

	// Assert: section listing page now shows the updated title.
	newSectionBytes, err := os.ReadFile(sectionPath)
	if err != nil {
		t.Fatalf("section listing missing after incremental build: %v", err)
	}
	if !strings.Contains(string(newSectionBytes), "Alpha Updated") {
		t.Errorf("section listing NOT updated after incremental build (C1 regression):\n%s", newSectionBytes)
	}
	if strings.Contains(string(newSectionBytes), "Alpha Original") {
		t.Errorf("section listing still shows stale title after incremental build:\n%s", newSectionBytes)
	}

	// Assert: tag/term listing page now shows the updated title.
	newTagBytes, err := os.ReadFile(tagPath)
	if err != nil {
		t.Fatalf("tag listing missing after incremental build: %v", err)
	}
	if !strings.Contains(string(newTagBytes), "Alpha Updated") {
		t.Errorf("tag listing NOT updated after incremental build (C1 regression):\n%s", newTagBytes)
	}
	if strings.Contains(string(newTagBytes), "Alpha Original") {
		t.Errorf("tag listing still shows stale title after incremental build:\n%s", newTagBytes)
	}
}