package build

// pipeline_write.go: stage 7 — copy static assets + finalize stats.

import (
	"io/fs"
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
	// .so theme plugins embed their assets (Assets() fs.FS) rather than
	// shipping a themes/<name>/static dir, so they need a separate write pass.
	p.writeThemePluginAssets()

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

	// Invoke OnOutputWritten hooks
	p.runOnOutputWritten()
}

// writeThemePluginAssets writes the active .so theme plugin's embedded static
// assets — returned by ThemePlugin.Assets() — into publishDir under
// theme/<name>/, matching the "/theme/<name>/..." URLs the theme templates
// reference. Disk-based themes (themes/<name>/static) are handled by the
// caller; this covers plugin themes whose assets are embedded in the .so.
// A nil theme manager or no active theme is a no-op.
func (p *pipeline) writeThemePluginAssets() {
	if p.themeManager == nil {
		return
	}
	tp := p.themeManager.Active()
	if tp == nil {
		return
	}
	name := p.themeManager.ActiveName()
	if name == "" {
		return
	}
	assets := tp.Assets()
	if assets == nil {
		return
	}

	base := "theme/" + name
	err := fs.WalkDir(assets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		return p.writer.WriteBytesPath(base+"/"+path, data)
	})
	if err != nil {
		p.logf("  WARN: theme plugin assets: %v\n", err)
	}
}

// output import placeholder, same pattern as pipeline_feeds.go.
var _ = output.URLToFilePath
