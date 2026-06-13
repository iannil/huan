package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/config"
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
	buildCmd.Flags().BoolP("buildFuture", "F", false, "include content with publishDate in the future")
	buildCmd.Flags().BoolP("buildExpired", "E", false, "include expired content")
	buildCmd.Flags().StringP("destination", "d", "", "filesystem path to write files to (overrides publishDir)")
	buildCmd.Flags().StringP("baseURL", "b", "", "hostname to the root (overrides baseURL)")
	buildCmd.Flags().Bool("minify", false, "minify output (overrides config)")

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the development server",
		RunE:  runServe,
	}

	serveCmd.Flags().String("port", "1313", "port to serve on")
	serveCmd.Flags().String("bind", "127.0.0.1", "interface to bind")
	serveCmd.Flags().BoolP("buildDrafts", "D", false, "include draft content")
	serveCmd.Flags().Bool("disableLiveReload", false, "disable browser auto-refresh")
	serveCmd.Flags().Duration("debounce", 400*time.Millisecond, "file change debounce delay")
	serveCmd.Flags().Bool("disableWatch", false, "do not watch files for changes")

	rootCmd.AddCommand(buildCmd, serveCmd, newDeployCmd(), newPluginCmd(), newReleaseCmd(), newVersionCmd(), newEnvCmd(), newConfigCmd(), newListCmd(), newNewCmd())

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
	if dest, _ := cmd.Flags().GetString("destination"); dest != "" {
		outputDir = dest
	}

	includeDrafts, _ := cmd.Flags().GetBool("buildDrafts")
	includeFuture, _ := cmd.Flags().GetBool("buildFuture")
	includeExpired, _ := cmd.Flags().GetBool("buildExpired")

	var minifyOverride *bool
	if cmd.Flags().Changed("minify") {
		m, _ := cmd.Flags().GetBool("minify")
		minifyOverride = &m
	}

	var baseURLOverride string
	if bu, _ := cmd.Flags().GetString("baseURL"); bu != "" {
		baseURLOverride = bu
	}

	_, err = build.BuildSite(build.Options{
		SourceDir:        sourceDir,
		OutputDir:        outputDir,
		IncludeDrafts:    includeDrafts,
		IncludeFuture:    includeFuture,
		IncludeExpired:   includeExpired,
		BaseURLOverride:  baseURLOverride,
		MinifyOverride:   minifyOverride,
	})
	return err
}

