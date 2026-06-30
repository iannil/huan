package build

// pipeline_write.go: stage 7 — copy static assets + finalize stats.

import (
	"os"
	"path/filepath"

	"github.com/iannil/huan/internal/output"
)

// copyStaticAndFinalize mirrors the theme + project static dirs into
// publishDir (project overrides theme) and captures writer.Stats into
// the result. This is the last stage — after it, BuildSite returns.
func (p *pipeline) copyStaticAndFinalize() {
	themeName := DetectThemeName(p.opts.SourceDir)
	if themeName != "" {
		themeStatic := filepath.Join(p.opts.SourceDir, "themes", themeName, "static")
		if _, err := os.Stat(themeStatic); err == nil {
			if err := p.writer.CopyStatic(themeStatic); err != nil {
				p.logf("  WARN: theme static: %v\n", err)
			}
		}
	}
	projectStatic := filepath.Join(p.opts.SourceDir, "static")
	if err := p.writer.CopyStatic(projectStatic); err != nil {
		p.logf("  WARN: static: %v\n", err)
	}

	files, bytes := p.writer.Stats()
	p.result.FilesWritten = files
	p.result.BytesWritten = bytes

	p.logf("  Rendered:     %d pages\n", p.result.PagesRendered)
	p.logf("  Output:       %d files, %.1f KB\n", files, float64(bytes)/1024)
	if p.result.Errors > 0 {
		p.logf("  Errors:       %d\n", p.result.Errors)
	}
	p.logf("Build complete.\n")
}

// output import placeholder, same pattern as pipeline_feeds.go.
var _ = output.URLToFilePath
