package build

// asset_gate.go: post-write integrity gates for static assets.
//
// Two failure modes motivated these gates:
//
//  1. A page references an asset that never reached the language's output
//     (template typo, misconfigured staticRoot, an option that skipped the
//     copy). The page ships unstyled and nothing in the build complains.
//  2. One language output carries fewer shared static files than another, so
//     the "parallel" language versions stop corresponding to each other.
//
// Both run after the static copy stage and are fatal by default: publishing a
// site with missing stylesheets is worse than a red build. HUAN_ASSET_CHECK=off
// downgrades them to warnings for the rare site that serves assets elsewhere.

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// assetFileExts lists extensions treated as static file references. Page
// routes (trailing slash, no extension) are deliberately excluded: they may be
// served by _redirects and are out of scope for the asset gate.
var assetFileExts = map[string]struct{}{
	".avif": {}, ".bmp": {}, ".css": {}, ".csv": {}, ".eot": {}, ".epub": {},
	".flac": {}, ".gif": {}, ".htm": {}, ".html": {}, ".ico": {}, ".jpeg": {},
	".jpg": {}, ".js": {}, ".json": {}, ".map": {}, ".m4a": {}, ".mjs": {},
	".mov": {}, ".mp3": {}, ".mp4": {}, ".ogg": {}, ".otf": {}, ".pdf": {},
	".png": {}, ".svg": {}, ".ttf": {}, ".txt": {}, ".wav": {}, ".wasm": {},
	".webm": {}, ".webmanifest": {}, ".webp": {}, ".woff": {}, ".woff2": {},
	".xml": {}, ".zip": {},
}

// assetRefs collects root-relative asset URLs observed while canonify rewrites
// rendered pages. Page renders run concurrently, so access is mutex-guarded.
type assetRefs struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func newAssetRefs() *assetRefs {
	return &assetRefs{seen: make(map[string]struct{})}
}

// record is the canonify OnRewrite callback. Non-asset URLs are dropped
// immediately so the map only holds references this gate judges.
func (a *assetRefs) record(urlPath string) {
	if a == nil || !looksLikeAsset(urlPath) {
		return
	}
	a.mu.Lock()
	a.seen[urlPath] = struct{}{}
	a.mu.Unlock()
}

// sorted returns the recorded references in a deterministic order.
func (a *assetRefs) sorted() []string {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.seen))
	for p := range a.seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// looksLikeAsset reports whether a root-relative URL points at a static file.
// Query strings and fragments are ignored when judging the extension.
func looksLikeAsset(urlPath string) bool {
	if !strings.HasPrefix(urlPath, "/") || strings.HasPrefix(urlPath, "//") {
		return false
	}
	clean := urlPath
	if i := strings.IndexAny(clean, "?#"); i >= 0 {
		clean = clean[:i]
	}
	ext := strings.ToLower(filepath.Ext(clean))
	if ext == "" {
		return false
	}
	_, ok := assetFileExts[ext]
	return ok
}

// assetCheckDisabled reports whether HUAN_ASSET_CHECK=off downgrades the gates
// to warnings.
func assetCheckDisabled() bool {
	v := strings.TrimSpace(os.Getenv("HUAN_ASSET_CHECK"))
	return strings.EqualFold(v, "off") || strings.EqualFold(v, "warn") || v == "0"
}

// checkReferencedAssets verifies that every asset-like root-relative
// reference seen in this language's rendered pages exists inside this
// language's output. A missing stylesheet or script fails the build; the
// language must be self-contained so the language versions correspond.
func (p *pipeline) checkReferencedAssets() error {
	refs := p.assetRefs.sorted()
	if len(refs) == 0 {
		return nil
	}
	var missing []string
	for _, ref := range refs {
		clean := ref
		if i := strings.IndexAny(clean, "?#"); i >= 0 {
			clean = clean[:i]
		}
		clean = strings.TrimPrefix(unescapeURLPath(clean), "/")
		full := filepath.Join(p.opts.OutputDir, filepath.FromSlash(clean))
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			missing = append(missing, ref)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	p.result.MissingAssets = len(missing)
	if assetCheckDisabled() {
		p.logf("  WARN: %d referenced assets are missing from %s: %s (HUAN_ASSET_CHECK=off)\n",
			len(missing), p.languageLabel(), strings.Join(limitList(missing, 5), ", "))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "asset check failed for %s: %d referenced asset(s) are not in the output\n", p.languageLabel(), len(missing))
	fmt.Fprintf(&b, "  output: %s\n", p.opts.OutputDir)
	for _, m := range limitList(missing, 20) {
		fmt.Fprintf(&b, "  - %s\n", m)
	}
	if len(missing) > 20 {
		fmt.Fprintf(&b, "  ... and %d more\n", len(missing)-20)
	}
	b.WriteString("\nEvery language output must carry the assets its own pages reference, otherwise the\n")
	b.WriteString("published pages lose their styling. Fix the reference, or copy the file into\n")
	b.WriteString("static/ (and, for language-local files, static/<staticRoot>/).\n")
	b.WriteString("\nSet HUAN_ASSET_CHECK=off to downgrade this to a warning.")
	return fmt.Errorf("%s", b.String())
}

// checkStaticParity verifies that this language output carries every shared
// static/ file it is supposed to own, honoring staticRoot remapping and
// staticExclude. It keeps the language outputs corresponding file-for-file.
func (p *pipeline) checkStaticParity() error {
	expected, err := expectedStaticFiles(p.opts.SourceDir, p.opts.StaticRoot, p.opts.StaticExclude)
	if err != nil {
		return err
	}
	if len(expected) == 0 {
		return nil
	}
	var missing []string
	for _, rel := range expected {
		full := filepath.Join(p.opts.OutputDir, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			missing = append(missing, rel)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	p.result.MissingStatic = len(missing)
	if assetCheckDisabled() {
		p.logf("  WARN: %d shared static files missing from %s: %s (HUAN_ASSET_CHECK=off)\n",
			len(missing), p.languageLabel(), strings.Join(limitList(missing, 5), ", "))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "static parity check failed for %s: %d shared static file(s) are missing\n",
		p.languageLabel(), len(missing))
	fmt.Fprintf(&b, "  output: %s\n", p.opts.OutputDir)
	for _, m := range limitList(missing, 20) {
		fmt.Fprintf(&b, "  - %s\n", m)
	}
	if len(missing) > 20 {
		fmt.Fprintf(&b, "  ... and %d more\n", len(missing)-20)
	}
	b.WriteString("\nEvery language output must contain the same shared static tree, so the language\n")
	b.WriteString("versions of the site correspond. A language may only differ through its own\n")
	b.WriteString("staticRoot / staticExclude declarations.\n")
	b.WriteString("\nSet HUAN_ASSET_CHECK=off to downgrade this to a warning.")
	return fmt.Errorf("%s", b.String())
}

// expectedStaticFiles lists output-relative slash paths that copyStatic is
// required to write for this language. Mirrors the copy rules exactly:
// shared tree minus excludes, plus the staticRoot subtree flattened to the
// output root.
func expectedStaticFiles(sourceDir, staticRoot string, excludes []string) ([]string, error) {
	root := filepath.Join(sourceDir, "static")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	rootPrefix := ""
	if staticRoot != "" {
		rootPrefix = strings.Trim(staticRoot, "/") + "/"
	}
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rootPrefix != "" && strings.HasPrefix(rel, rootPrefix) {
			out = append(out, strings.TrimPrefix(rel, rootPrefix))
			return nil
		}
		for _, ex := range excludes {
			if ex != "" && strings.HasPrefix(rel, ex) {
				return nil
			}
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// languageLabel names this build's language in diagnostics.
func (p *pipeline) languageLabel() string {
	if p.cfg != nil && p.cfg.LanguageCode != "" {
		return "language " + p.cfg.LanguageCode
	}
	return "the default language"
}

// limitList returns at most n items, for diagnostics.
func limitList(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

// unescapeURLPath decodes percent-escapes so "/css/a%20b.css" matches the file
// name on disk. Invalid escapes are returned unchanged.
func unescapeURLPath(p string) string {
	if !strings.Contains(p, "%") {
		return p
	}
	if unescaped, err := url.PathUnescape(p); err == nil {
		return unescaped
	}
	return p
}
