package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
	"github.com/iannil/huan-plugin-ebook-exporter/render"
	"github.com/iannil/huan-plugin-ebook-exporter/style"
	"github.com/iannil/huan/pkg/plugin"
)

// Config is the ebook-exporter configuration from huan.yaml
// plugins.ebook_exporter.*.
type Config struct {
	// OutputDir is the export root (relative to the project source dir).
	OutputDir string
	// FontsDir is an optional extra directory searched for CJK fonts.
	FontsDir string
	// Cover controls whether a cover page is generated (reserved; V1
	// backends always emit a title page).
	Cover bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		OutputDir: "developer/export",
		Cover:     true,
	}
}

// ParseConfig parses a raw config map into Config, erroring on wrong types.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}
	if v, ok := raw["output_dir"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("output_dir: expected string, got %T", v)
		}
		cfg.OutputDir = s
	}
	if v, ok := raw["fonts_dir"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("fonts_dir: expected string, got %T", v)
		}
		cfg.FontsDir = s
	}
	if v, ok := raw["cover"]; ok {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("cover: expected bool, got %T", v)
		}
		cfg.Cover = b
	}
	return cfg, nil
}

// ConfigSchema returns the plugin.Schema for config validation.
func (c *Config) ConfigSchema() plugin.Schema {
	return plugin.Schema{Fields: []plugin.FieldSchema{
		{Key: "output_dir", Type: "string", Required: false, Default: "developer/export", Description: "ebook 输出根目录（相对项目根）"},
		{Key: "fonts_dir", Type: "string", Required: false, Description: "额外的 CJK 字体搜索目录"},
		{Key: "cover", Type: "bool", Required: false, Default: true, Description: "是否生成封面页"},
	}}
}

// EbookExporter is the pkg/plugin.Exporter implementation that batches
// books/practices through the EPUB/PDF/DOCX backends.
type EbookExporter struct {
	cfg Config
}

// New creates an EbookExporter from parsed config.
func New(cfg *Config) *EbookExporter {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &EbookExporter{cfg: *cfg}
}

// Name returns the plugin name (yaml key under plugins:).
func (p *EbookExporter) Name() string { return "ebook_exporter" }

// PluginMetadata returns the plugin metadata.
func (p *EbookExporter) PluginMetadata() plugin.PluginMeta {
	return plugin.PluginMeta{
		Version:    "0.1.0",
		Author:     "huan team",
		Tags:       []string{"ebook", "epub", "pdf", "docx"},
		IsOfficial: true,
	}
}

// allFormats is the full format expansion of Format "all".
var allFormats = []string{"epub", "pdf", "docx"}

// unit is one schedulable export job: an aggregate BookEntry (individual
// book, volume concatenation, or complete bundle) exported per language.
type unit struct {
	// kind is "books" | "practices" (used in the output path).
	kind string
	// dirName is "individual" | "volumes" | "complete".
	dirName string
	// baseName is the output filename stem (slug, volume-N, kind-complete).
	baseName string
	// agg is the (possibly synthesized) book to render; read-only.
	agg *content.BookEntry
	// mdPaths are the source files feeding the manifest content hash.
	mdPaths []string
}

// normalizeReq fills empty request fields with defaults and canonicalizes
// aliases ("seasons" -> "volumes").
func normalizeReq(req plugin.ExportRequest) plugin.ExportRequest {
	switch req.Type {
	case "", "all":
		req.Type = "all"
	}
	switch req.Format {
	case "":
		req.Format = "all"
	}
	switch req.Level {
	case "":
		req.Level = "all"
	case "seasons":
		req.Level = "volumes"
	}
	if req.Jobs <= 0 {
		req.Jobs = runtime.NumCPU() - 1
		if req.Jobs < 1 {
			req.Jobs = 1
		}
	}
	return req
}

// wantLevel reports whether level includes dirName granularity.
func wantLevel(level, dirName string) bool { return level == "all" || level == dirName }

// formatsFor expands req.Format into the concrete format list.
func formatsFor(format string) []string {
	if format == "all" {
		return allFormats
	}
	return []string{format}
}

// unitLangs returns the languages to export: zh always; en only when an EN
// sidecar exists. Aggregates (empty Slug) never warn.
func unitLangs(b *content.BookEntry, res *plugin.ExportResult) []content.Lang {
	langs := []content.Lang{content.LangZH}
	if b.HasEN {
		langs = append(langs, content.LangEN)
	} else if b.Slug != "" {
		res.Warnings = append(res.Warnings, b.Slug+": no .en.md sidecars, EN skipped")
	}
	return langs
}

// outPath builds <outRoot>/<format>/<kind>/<dirName>/<base>[-<lang>].<ext>.
func outPath(outRoot, kind, dirName, base string, lang content.Lang, format string) string {
	name := base
	if lang != content.LangZH {
		name += "-" + string(lang)
	}
	return filepath.Join(outRoot, format, kind, dirName, name+"."+format)
}

// renderUnit renders one unit in one language into one format.
func renderUnit(book *content.BookEntry, lang content.Lang, format, outPath, fontPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	switch format {
	case "epub":
		opts := render.EPUBOptions{EmbedFont: fontPath != "", FontPath: fontPath}
		return render.RenderEPUB(book, lang, outPath, opts)
	case "pdf":
		// PDF requires a CJK font; missing font is a per-item failure.
		if fontPath == "" {
			return fmt.Errorf("pdf: no CJK font found (set fonts_dir config)")
		}
		return render.RenderPDF(book, lang, outPath, render.PDFOptions{FontPath: fontPath})
	case "docx":
		return render.RenderDOCX(book, lang, outPath, render.DOCXOptions{})
	}
	return fmt.Errorf("unknown format: %s", format)
}

// bookMDPaths lists every markdown source file of a book (zh + en sidecars),
// de-duplicated, for manifest hashing.
func bookMDPaths(b *content.BookEntry) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, sec := range b.Sections {
		for _, ch := range sec.Chapters {
			add(ch.SourcePath)
			add(ch.ENPath)
		}
	}
	return out
}

// booksMDPaths aggregates bookMDPaths across many books.
func booksMDPaths(books []content.BookEntry) []string {
	var out []string
	for i := range books {
		out = append(out, bookMDPaths(&books[i])...)
	}
	return out
}

// separatorSection builds a part-type section used as a book-title divider
// when concatenating books into volume/complete aggregates.
func separatorSection(id, title string) content.Section {
	return content.Section{Type: "part", ID: id, Title: title}
}

// aggregateBook concatenates the given books into one synthetic BookEntry:
// each book contributes a title separator section followed by its own
// ordered sections.
//
// V1 design note: we do NOT demote chapter headings inside the aggregate.
// The Render* backends render chapter titles as H1 regardless, so aggregated
// volumes present each contributing book as a separator page (part section)
// followed by its chapters at their normal level. This keeps individual and
// volume rendering visually consistent and avoids adding a TitleLevel knob
// to the three backends in V1.
func aggregateBook(vol *content.VolumeEntry, books []content.BookEntry, title string) *content.BookEntry {
	agg := &content.BookEntry{TitleZH: title}
	for _, b := range books {
		agg.Sections = append(agg.Sections, separatorSection("book-"+b.Slug, b.TitleZH))
		agg.Sections = append(agg.Sections, b.OrderedSections()...)
		agg.HasEN = agg.HasEN || b.HasEN
	}
	if vol != nil {
		agg.VolumeNumber = vol.Number
		agg.VolumeName = vol.Name
	}
	return agg
}

// filterBooks returns the books of the collection matching req.Volume (0 =
// all volumes) and req.Slug ("" = all).
func filterBooks(col *content.Collection, req plugin.ExportRequest) []content.BookEntry {
	var out []content.BookEntry
	for _, vol := range col.Volumes {
		if req.Volume != 0 && vol.Number != req.Volume {
			continue
		}
		for _, b := range vol.Books {
			if req.Slug != "" && b.Slug != req.Slug {
				continue
			}
			out = append(out, b)
		}
	}
	return out
}

// expandUnits turns a discovered collection into export units honoring
// req.Level / req.Slug / req.Volume filters.
func expandUnits(kind string, col *content.Collection, req plugin.ExportRequest) []*unit {
	var units []*unit
	books := filterBooks(col, req)

	if wantLevel(req.Level, "individual") {
		for i := range books {
			b := books[i]
			units = append(units, &unit{
				kind:     kind,
				dirName:  "individual",
				baseName: b.Slug,
				agg:      &books[i],
				mdPaths:  bookMDPaths(&books[i]),
			})
		}
	}
	// Aggregates are skipped when the request pins a single slug: a one-book
	// "volume"/"complete" bundle adds no value and would double-render.
	if req.Slug == "" {
		if wantLevel(req.Level, "volumes") {
			for _, vol := range col.Volumes {
				if req.Volume != 0 && vol.Number != req.Volume {
					continue
				}
				volBooks := filterBooks(&content.Collection{Volumes: []content.VolumeEntry{vol}}, req)
				if len(volBooks) == 0 {
					continue
				}
				units = append(units, &unit{
					kind:     kind,
					dirName:  "volumes",
					baseName: fmt.Sprintf("volume-%d", vol.Number),
					agg:      aggregateBook(&vol, volBooks, fmt.Sprintf("%s合集", vol.Name)),
					mdPaths:  booksMDPaths(volBooks),
				})
			}
		}
		if wantLevel(req.Level, "complete") && len(books) > 0 {
			units = append(units, &unit{
				kind:     kind,
				dirName:  "complete",
				baseName: kind + "-complete",
				agg:      aggregateBook(nil, books, kind+"-complete"),
				mdPaths:  booksMDPaths(books),
			})
		}
	}
	return units
}

// Export runs the batch. See pkg/plugin.Exporter for the contract:
// per-item results even on partial failure; a non-nil error only when the
// export cannot proceed at all.
func (p *EbookExporter) Export(ctx context.Context, req plugin.ExportRequest) (plugin.ExportResult, error) {
	req = normalizeReq(req)
	outRoot := p.cfg.OutputDir
	if !filepath.IsAbs(outRoot) {
		outRoot = filepath.Join(req.SourceDir, p.cfg.OutputDir)
	}

	// Font lookup once per batch; EPUB embedding and PDF rendering share it.
	// A missing font only affects items that need it (per-item failure),
	// except for PDF-only requests of zh content where every unit fails.
	fontPath, ferr := style.FindCJKFont(p.cfg.FontsDir)
	if ferr != nil {
		fontPath = ""
	}

	// Resolve kinds and discover collections. A missing content root for a
	// requested kind is a hard error (cannot start).
	kinds := []string{"books", "practices"}
	if req.Type != "all" {
		kinds = []string{req.Type}
	}
	var units []*unit
	for _, kind := range kinds {
		kindRoot := filepath.Join(req.SourceDir, "content", kind)
		if _, err := os.Stat(kindRoot); err != nil {
			return plugin.ExportResult{}, fmt.Errorf("content/%s not found under %s", kind, req.SourceDir)
		}
		col, err := content.Discover(req.SourceDir, kind)
		if err != nil {
			return plugin.ExportResult{}, fmt.Errorf("discover %s: %w", kind, err)
		}
		units = append(units, expandUnits(kind, col, req)...)
	}

	res := plugin.ExportResult{}
	manifest := LoadManifest(outRoot)

	// Phase 1: incremental skip — cheap, sequential, no rendering.
	type job struct {
		u     *unit
		langs []content.Lang
	}
	var pending []job
	for _, u := range units {
		langs := unitLangs(u.agg, &res)
		allSkipped := true
		for _, lang := range langs {
			key := u.baseName + "." + string(lang)
			if req.Force || manifest.Entries[key] != ComputeHash(u.mdPaths) {
				allSkipped = false
				break
			}
		}
		if allSkipped {
			for _, lang := range langs {
				for _, f := range formatsFor(req.Format) {
					res.Skipped = append(res.Skipped, plugin.ExportItem{
						Path:   outPath(outRoot, u.kind, u.dirName, u.baseName, lang, f),
						Lang:   string(lang),
						Format: f,
						Slug:   u.agg.Slug,
					})
				}
			}
			continue
		}
		pending = append(pending, job{u: u, langs: langs})
	}

	// Phase 2: parallel rendering. A buffered semaphore channel bounds
	// concurrency at req.Jobs; results land in fixed slots so ordering is
	// deterministic regardless of completion order.
	type jobResult struct {
		items []plugin.ExportItem
		fails []plugin.ExportFailure
		// hashes per unit: one hash shared by all langs of the unit.
		hash string
	}
	results := make([]*jobResult, len(pending))
	sem := make(chan struct{}, req.Jobs)
	var wg sync.WaitGroup
	for i, j := range pending {
		wg.Add(1)
		go func(idx int, j job) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			jr := &jobResult{hash: ComputeHash(j.u.mdPaths)}
			for _, lang := range j.langs {
				for _, f := range formatsFor(req.Format) {
					out := outPath(outRoot, j.u.kind, j.u.dirName, j.u.baseName, lang, f)
					if err := renderUnit(j.u.agg, lang, f, out, fontPath); err != nil {
						jr.fails = append(jr.fails, plugin.ExportFailure{
							Item: plugin.ExportItem{Path: out, Lang: string(lang), Format: f, Slug: j.u.agg.Slug},
							Err:  err.Error(),
						})
						continue
					}
					jr.items = append(jr.items, plugin.ExportItem{
						Path: out, Lang: string(lang), Format: f, Slug: j.u.agg.Slug,
					})
				}
			}
			results[idx] = jr
		}(i, j)
	}
	wg.Wait()

	// Phase 3: collect in unit order; record manifest hashes for units with
	// at least one success in every language (partial failure -> re-export
	// next run).
	for i, jr := range results {
		if jr == nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: cancelled before render", pending[i].u.baseName))
			continue
		}
		res.Succeeded = append(res.Succeeded, jr.items...)
		res.Failed = append(res.Failed, jr.fails...)
		if len(jr.fails) == 0 {
			for _, lang := range pending[i].langs {
				manifest.Entries[pending[i].u.baseName+"."+string(lang)] = jr.hash
			}
		}
	}
	if err := SaveManifest(outRoot, manifest); err != nil {
		res.Warnings = append(res.Warnings, "save manifest: "+err.Error())
	}
	return res, nil
}
