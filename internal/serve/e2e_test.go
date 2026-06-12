package serve

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestE2EServeWatchReload exercises the full pipeline:
// - Start HTTP server with livereload hub attached
// - Connect a WS client (simulating browser)
// - Modify a file in the watched dir
// - Verify the client receives a reload message
//
// This doesn't go through BuildSite — it tests that the serve-side wiring
// (Watcher + Hub + Server) correctly connects file changes to WS broadcasts
// when the controller code wires them together as runServe does.
func TestE2EServeWatchReload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e in short mode")
	}

	// Output dir serves static files (simulating build output)
	outDir, err := os.MkdirTemp("", "huan-e2e-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outDir)

	// Source dir is watched for changes
	srcDir, err := os.MkdirTemp("", "huan-e2e-src-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)

	// Write a minimal HTML file in output
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte("<html><head></head><body>hello</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wire up the pieces exactly as runServe does
	hub := NewHub()
	server := New(ServerOptions{
		OutputDir: outDir,
		Bind:      "127.0.0.1",
		Port:      "0",
		Hub:       hub,
		Logf:      func(format string, a ...any) { t.Logf(format, a...) },
	})
	addrCh := make(chan string, 1)
	server.addrCh = addrCh

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go server.Run(ctx) //nolint:errcheck

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start")
	}

	// Start watcher with OnChange → BroadcastReload (exactly as runServe does)
	watcher, err := NewWatcher(WatcherOptions{
		SourceDir: srcDir,
		Debounce:  50 * time.Millisecond,
		OnChange: func() {
			hub.BroadcastReload()
		},
		Logf: func(format string, a ...any) { t.Logf(format, a...) },
	})
	if err != nil {
		t.Fatal(err)
	}
	go watcher.Run(ctx) //nolint:errcheck

	// Give watcher time to install hooks
	time.Sleep(100 * time.Millisecond)

	// Connect a WS client (simulating browser)
	wsURL := "ws://" + addr + "/livereload"
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", wsURL, err)
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 16)

	// Read hello
	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if !strings.Contains(string(msg), `"hello"`) {
		t.Errorf("hello msg = %s, want contains 'hello'", string(msg))
	}

	// Modify a file in srcDir to trigger watcher
	if err := os.WriteFile(filepath.Join(srcDir, "trigger.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Expect reload within reasonable time
	readCtx, readCancel := context.WithTimeout(ctx, 3*time.Second)
	defer readCancel()
	_, msg, err = c.Read(readCtx)
	if err != nil {
		t.Fatalf("expected reload within 3s, got error: %v", err)
	}
	if !strings.Contains(string(msg), `"reload"`) {
		t.Errorf("reload msg = %s, want 'reload'", string(msg))
	}
}
