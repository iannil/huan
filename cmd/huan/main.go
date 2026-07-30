package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/theme"
	"github.com/spf13/cobra"
)

var sourceDir string

var rootCmd = &cobra.Command{
	Use:   "huan",
	Short: "A static site generator",
	Long:  "huan is a static site generator written in Go, designed to replace Hugo for zhurongshuo.com.",
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&sourceDir, "source", "s", ".", "source directory containing huan.yaml and content/")
}

func main() {

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

	// serveCmd is the deprecated alias for devCmd.
	// Kept for backward compatibility; removed in the next major version.
	serveCmd := &cobra.Command{
		Use:        "serve",
		Short:      "DEPRECATED: use 'huan dev' instead",
		Hidden:     true,
		Deprecated: "use 'huan dev' instead",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDev(cmd, args)
		},
	}

	rootCmd.AddCommand(buildCmd, serveCmd, newDeployCmd(), newPluginCmd(), newReleaseCmd(), newVersionCmd(), newEnvCmd(), newConfigCmd(), newListCmd(), newNewCmd(), newSyncCmd(), newTocCmd(), newExportCmd(), newTranslateCmd(), newThemeCmd())

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

	// Create plugin registry and theme manager
	reg, _ := newPluginRegistry(cfg, sourceDir, "")
	themeMgr := theme.NewManager(reg)
	if cfg.Theme != "" {
		if err := themeMgr.Activate(cfg.Theme); err != nil {
			fmt.Fprintf(os.Stderr, "huan: theme activate %q: %v\n", cfg.Theme, err)
		}
	}

	// Multi-language dispatch: when huan.yaml declares a languages: block,
	// route through BuildMultiSite which renders each language under its
	// baseURL prefix. Single-language configs use the existing BuildSite path.
	if cfg.IsMultiLanguage() {
		multiResult, err := build.BuildMultiSite(build.Options{
			SourceDir:        sourceDir,
			OutputDir:        outputDir,
			IncludeDrafts:    includeDrafts,
			IncludeFuture:    includeFuture,
			IncludeExpired:   includeExpired,
			BaseURLOverride:  baseURLOverride,
			MinifyOverride:   minifyOverride,
			PluginRegistry:   reg,
			ThemeManager:     themeMgr,
		})
		if err != nil {
			return err
		}
		fmt.Println(build.SummarizeMultiSite(multiResult))

		// Run image pipeline after build if configured
		if err := runImagePipeline(sourceDir, outputDir, reg); err != nil {
			return fmt.Errorf("after build: %w", err)
		}
		return nil
	}

	_, err = build.BuildSite(build.Options{
		SourceDir:        sourceDir,
		OutputDir:        outputDir,
		IncludeDrafts:    includeDrafts,
		IncludeFuture:    includeFuture,
		IncludeExpired:   includeExpired,
		BaseURLOverride:  baseURLOverride,
		MinifyOverride:   minifyOverride,
		PluginRegistry:   reg,
		ThemeManager:     themeMgr,
	})
	if err != nil {
		return err
	}

	// Run image pipeline after build if configured
	return runImagePipeline(sourceDir, outputDir, reg)
}

