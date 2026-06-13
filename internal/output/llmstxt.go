package output

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iannil/huan/internal/config"
)

// GenerateLlmsTxt writes /llms.txt to the output directory.
// If layouts/llms.txt exists, it's used as-is; otherwise a default is
// generated from the site config.
func GenerateLlmsTxt(outputDir, sourceDir string, cfg *config.Config) error {
	// Check for user-provided template
	templatePath := filepath.Join(sourceDir, "layouts", "llms.txt")
	if data, err := os.ReadFile(templatePath); err == nil {
		return os.WriteFile(filepath.Join(outputDir, "llms.txt"), data, 0o644)
	}

	// Auto-generate default
	content := defaultLlmsTxt(cfg)
	return os.WriteFile(filepath.Join(outputDir, "llms.txt"), []byte(content), 0o644)
}

func defaultLlmsTxt(cfg *config.Config) string {
	var buf strings.Builder

	buf.WriteString("# ")
	buf.WriteString(cfg.Title)
	buf.WriteString("\n\n")

	if cfg.Params.Description != "" {
		buf.WriteString("> ")
		buf.WriteString(cfg.Params.Description)
		buf.WriteString("\n\n")
	}

	if cfg.Params.SubTitle != "" {
		buf.WriteString(cfg.Params.SubTitle)
		buf.WriteString("\n\n")
	}

	if len(cfg.Params.MainSections) > 0 {
		buf.WriteString("## Content\n\n")
		for _, section := range cfg.Params.MainSections {
			url := cfg.BaseURL + section + "/"
			fmt.Fprintf(&buf, "- [%s](%s)\n", section, url)
		}
		buf.WriteString("\n")
	}

	buf.WriteString("## Links\n\n")
	fmt.Fprintf(&buf, "- [Home](%s)\n", cfg.BaseURL)
	if cfg.Sitemap.Filename != "" {
		fmt.Fprintf(&buf, "- [Sitemap](%s%s)\n", cfg.BaseURL, cfg.Sitemap.Filename)
	}

	return buf.String()
}
