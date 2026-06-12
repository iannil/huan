package main

import (
	"context"
	"fmt"
	"os"

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

	// Serve uses a temp directory, never the real publishDir (docs/).
	tmpDir, err := os.MkdirTemp("", "huan-serve-*")
	if err != nil {
		return fmt.Errorf("mkdtemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	includeDrafts, _ := cmd.Flags().GetBool("buildDrafts")

	fmt.Printf("Building site: %s\n", cfg.Title)
	fmt.Printf("  Source:      %s\n", sourceDir)
	fmt.Printf("  Output:      %s\n", tmpDir)

	if _, err := build.BuildSite(build.Options{
		SourceDir:     sourceDir,
		OutputDir:     tmpDir,
		IncludeDrafts: includeDrafts,
	}); err != nil {
		return err
	}

	fmt.Printf("Serving at:  http://%s:%s/\n", bind, port)
	fmt.Println("Press Ctrl+C to stop")

	srv := serve.New(serve.ServerOptions{
		OutputDir: tmpDir,
		Bind:      bind,
		Port:      port,
		Logf:      func(format string, a ...any) { fmt.Printf(format, a...) },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return srv.Run(ctx)
}
