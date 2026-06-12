package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/novel_ttl/huan/internal/build"
	"github.com/novel_ttl/huan/internal/serve"
	"github.com/spf13/cobra"
)

func runServe(cmd *cobra.Command, args []string) error {
	port, _ := cmd.Flags().GetString("port")
	bind, _ := cmd.Flags().GetString("bind")
	disableLR, _ := cmd.Flags().GetBool("disableLiveReload")
	disableWatch, _ := cmd.Flags().GetBool("disableWatch")
	debounce, _ := cmd.Flags().GetDuration("debounce")
	includeDrafts, _ := cmd.Flags().GetBool("buildDrafts")

	// For browser-facing URLs, wildcard binds (0.0.0.0, ::) aren't reachable
	// from the browser; use localhost. Same-machine clients hit localhost,
	// LAN clients override via their own hostname.
	browserHost := bind
	if bind == "0.0.0.0" || bind == "::" {
		browserHost = "localhost"
	}

	// LiveReload URL options
	lrURL := ""
	injectLR := false
	if !disableLR {
		injectLR = true
		lrURL = "ws://" + browserHost + ":" + port + "/livereload"
	}

	// Create hub if LiveReload enabled
	var hub *serve.LiveReloadHub
	if !disableLR {
		hub = serve.NewHub()
	}

	// Serve uses a temp directory, never the real publishDir (docs/).
	tmpDir, err := os.MkdirTemp("", "huan-serve-*")
	if err != nil {
		return fmt.Errorf("mkdtemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Build options shared between initial build and rebuilds
	buildOpts := build.Options{
		SourceDir:        sourceDir,
		OutputDir:        tmpDir,
		IncludeDrafts:    includeDrafts,
		InjectLiveReload: injectLR,
		LiveReloadURL:    lrURL,
		Logf:             func(format string, a ...any) { fmt.Printf(format, a...) },
	}

	if _, err := build.BuildSite(buildOpts); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watcher if not disabled.
	// BuildSite is not safe for concurrent calls (mutates package-level
	// template/i18n state, writes to the same OutputDir). Serialize rebuilds:
	// if a build is in flight, set rebuildPending so we do one trailing rebuild
	// after the current one finishes — coalescing any burst of edits.
	//
	// Rebuilds build into a sibling staging dir (tmpDir + ".next"), then
	// atomically swap it into tmpDir. This serves the OLD content during the
	// multi-second rebuild window (no 404s), and atomically replaces with the
	// new content on success — so deleted source files don't linger as stale
	// pages. On rebuild failure we leave tmpDir untouched.
	var (
		rebuildBusy   atomic.Bool
		rebuildPended atomic.Bool
	)
	nextDir := tmpDir + ".next"
	doRebuild := func() {
		if !rebuildBusy.CompareAndSwap(false, true) {
			// Already building; mark pending and let the in-flight build pick it up.
			rebuildPended.Store(true)
			return
		}
		for {
			fmt.Println("[watch] change detected, rebuilding...")
			start := time.Now()
			_ = os.RemoveAll(nextDir) // clear any leftover from previous failed rebuild
			buildOpts.OutputDir = nextDir
			if _, err := build.BuildSite(buildOpts); err != nil {
				_ = os.RemoveAll(nextDir)
				buildOpts.OutputDir = tmpDir // restore so next iteration re-stages correctly
				fmt.Printf("[watch] rebuild error: %v\n", err)
				if hub != nil {
					hub.BroadcastAlert(fmt.Sprintf("huan rebuild error: %v", err))
				}
				break
			}
			if err := build.SwapBuildDir(tmpDir, nextDir); err != nil {
				// Extremely unlikely (rename failure on same filesystem).
				// Fall back to keeping the new build in nextDir and serving old.
				_ = os.RemoveAll(nextDir)
				buildOpts.OutputDir = tmpDir
				fmt.Printf("[watch] swap failed, kept old build: %v\n", err)
			}
			buildOpts.OutputDir = tmpDir
			fmt.Printf("[watch] rebuild complete in %v\n", time.Since(start))
			if hub != nil {
				hub.BroadcastReload()
			}
			if !rebuildPended.CompareAndSwap(true, false) {
				break // no pending rebuild, exit loop
			}
			// Pending rebuild queued — loop and rebuild again.
		}
		rebuildBusy.Store(false)
	}

	if !disableWatch {
		watcher, err := serve.NewWatcher(serve.WatcherOptions{
			SourceDir: sourceDir,
			Debounce:  debounce,
			OnChange:  doRebuild,
		})
		if err != nil {
			fmt.Printf("WARNING: file watcher unavailable: %v\n", err)
			fmt.Println("WARNING: use --disableWatch to suppress this message")
		} else {
			go watcher.Run(ctx) //nolint:errcheck
		}
	}

	fmt.Println("Press Ctrl+C to stop")

	srv := serve.New(serve.ServerOptions{
		OutputDir: tmpDir,
		Bind:      bind,
		Port:      port,
		Hub:       hub,
		Logf:      func(format string, a ...any) { fmt.Printf(format, a...) },
	})
	return srv.Run(ctx)
}
