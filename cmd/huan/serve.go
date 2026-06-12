package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/novel_ttl/huan/internal/build"
	"github.com/novel_ttl/huan/internal/config"
	"github.com/novel_ttl/huan/internal/serve"
	"github.com/spf13/cobra"
)

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	port, _ := cmd.Flags().GetString("port")
	bind, _ := cmd.Flags().GetString("bind")
	disableLR, _ := cmd.Flags().GetBool("disableLiveReload")
	disableWatch, _ := cmd.Flags().GetBool("disableWatch")
	debounce, _ := cmd.Flags().GetDuration("debounce")
	includeDrafts, _ := cmd.Flags().GetBool("buildDrafts")

	// LiveReload URL options
	lrURL := ""
	injectLR := false
	if !disableLR {
		injectLR = true
		lrURL = "ws://" + bind + ":" + port + "/livereload"
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

	fmt.Printf("Building site: %s\n", cfg.Title)
	fmt.Printf("  Source:      %s\n", sourceDir)
	fmt.Printf("  Output:      %s\n", tmpDir)

	if _, err := build.BuildSite(buildOpts); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watcher if not disabled
	if !disableWatch {
		watcher, err := serve.NewWatcher(serve.WatcherOptions{
			SourceDir: sourceDir,
			Debounce:  debounce,
			OnChange: func() {
				fmt.Println("[watch] change detected, rebuilding...")
				start := time.Now()
				if _, err := build.BuildSite(buildOpts); err != nil {
					fmt.Printf("[watch] rebuild error: %v\n", err)
					return
				}
				fmt.Printf("[watch] rebuild complete in %v\n", time.Since(start))
				if hub != nil {
					hub.BroadcastReload()
				}
				},
		})
		if err != nil {
			fmt.Printf("WARNING: file watcher unavailable: %v\n", err)
			fmt.Println("WARNING: use --disableWatch to suppress this message")
		} else {
			go watcher.Run(ctx) //nolint:errcheck
		}
	}

	fmt.Printf("Serving at:  http://%s:%s/\n", bind, port)
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
