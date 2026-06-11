package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/novel_ttl/huan/internal/config"
	"github.com/novel_ttl/huan/internal/content"
	"github.com/novel_ttl/huan/internal/markdown"
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

	fmt.Printf("Building site: %s\n", cfg.Title)
	fmt.Printf("  Source:      %s\n", sourceDir)
	fmt.Printf("  Output:      %s\n", cfg.PublishDir)
	fmt.Printf("  BaseURL:     %s\n", cfg.BaseURL)

	// Load content
	contentDir := filepath.Join(sourceDir, "content")
	pages, err := content.LoadDir(contentDir)
	if err != nil {
		return fmt.Errorf("load content: %w", err)
	}
	fmt.Printf("  Pages loaded: %d\n", len(pages))

	// Load data files
	dataDir := filepath.Join(sourceDir, "data")
	data, err := content.LoadDataFiles(dataDir)
	if err != nil {
		return fmt.Errorf("load data: %w", err)
	}
	fmt.Printf("  Data files:   %d\n", len(data))

	// Render Markdown
	md := markdown.NewRenderer(&cfg.Markup)
	rendered := 0
	for _, p := range pages {
		if p.RawContent == "" {
			continue
		}
		html, err := md.Render(p.RawContent)
		if err != nil {
			return fmt.Errorf("render %s: %w", p.RelPath, err)
		}
		p.Content = template.HTML(html)
		rendered++
	}
	fmt.Printf("  Rendered:     %d\n", rendered)

	// Build content tree
	site, err := content.BuildTree(pages, cfg, sourceDir)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}
	site.Data = data

	// Print stats
	regular := 0
	sections := 0
	drafts := 0
	for _, p := range site.Pages {
		switch p.Kind {
		case "page":
			regular++
		case "section":
			sections++
		}
		if p.Draft {
			drafts++
		}
	}
	fmt.Printf("  Regular pages: %d\n", regular)
	fmt.Printf("  Sections:      %d\n", sections)
	fmt.Printf("  Drafts:        %d\n", drafts)

	// TODO: template rendering, output writing

	fmt.Println("Build complete.")
	return nil
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
