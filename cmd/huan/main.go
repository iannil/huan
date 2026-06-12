package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/novel_ttl/huan/internal/build"
	"github.com/novel_ttl/huan/internal/config"
	"github.com/spf13/cobra"
)

var sourceDir string

func main() {
	rootCmd := &cobra.Command{
		Use:   "huan",
		Short: "A static site generator",
		Long:  "huan is a static site generator written in Go, designed to replace Hugo for zhurongshuo.com.",
	}

	rootCmd.PersistentFlags().StringVarP(&sourceDir, "source", "s", ".", "source directory containing huan.yaml and content/")

	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build the site",
		RunE:  runBuild,
	}
	buildCmd.Flags().BoolP("buildDrafts", "D", false, "include draft content")

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the development server",
		RunE:  runServe,
	}

	serveCmd.Flags().String("port", "1313", "port to serve on")

	rootCmd.AddCommand(buildCmd, serveCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runBuild(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	outputDir := filepath.Join(sourceDir, cfg.PublishDir)

	includeDrafts, _ := cmd.Flags().GetBool("buildDrafts")
	_, err = build.BuildSite(build.Options{
		SourceDir:     sourceDir,
		OutputDir:     outputDir,
		IncludeDrafts: includeDrafts,
	})
	return err
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	port, _ := cmd.Flags().GetString("port")

	fmt.Printf("Serving site: %s\n", cfg.Title)
	fmt.Printf("  Source:      %s\n", sourceDir)
	fmt.Printf("  Output:      %s\n", cfg.PublishDir)
	fmt.Printf("  URL:         http://localhost:%s\n", port)

	// TODO: build, then serve via HTTP
	fmt.Println("Serve not yet implemented.")
	return nil
}
