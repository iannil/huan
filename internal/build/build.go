package build

// build.go: BuildSite — the orchestrator. Stage methods live in
// pipeline_*.go and operate on a *pipeline struct (see pipeline.go).

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

// Options controls a single BuildSite invocation.
type Options struct {
	SourceDir        string
	OutputDir        string // absolute path
	IncludeDrafts    bool
	IncludeFuture    bool
	IncludeExpired   bool
	InjectLiveReload bool   // serve-only; when true, LiveReloadURL must be set
	LiveReloadURL    string // empty disables injection
	BaseURLOverride  string // serve-only; overrides cfg.BaseURL for dev server
	MinifyOverride   *bool  // nil = use config Minify; non-nil = force this value
	Logf             func(format string, args ...any)

	// CfgOverride skips config.Load(opts.SourceDir) and uses this Config
	// directly. Used internally by BuildMultiSite to build per-language
	// variants without re-reading huan.yaml.
	CfgOverride *config.Config

	// PageFilter, when non-nil, excludes pages where the function returns
	// false. Used internally by BuildMultiSite to build per-language subsets.
	PageFilter func(*content.Page) bool

	// AvailableTranslations, when non-nil, maps each page's language-neutral
	// RelPath to the set of language codes with an actual sidecar file.
	// Used by template context to filter hreflang output to only languages
	// that actually exist for each page (prevents hreflang=en → 404).
	AvailableTranslations map[string]map[string]bool
}

// Result reports what happened during the build.
type Result struct {
	PagesRendered int
	FilesWritten  int
	BytesWritten  int64
	Errors        int
	Duration      time.Duration
}

func (o *Options) logf() func(string, ...any) {
	if o.Logf == nil {
		return func(format string, args ...any) { fmt.Printf(format, args...) }
	}
	return o.Logf
}

// BuildSite renders the full site from SourceDir into OutputDir, orchestrating
// 7 stages via the pipeline type (see pipeline.go and pipeline_*.go):
//
//  1. loadConfig (pipeline.go)           — cfg + serve overrides + i18n inject
//  2. loadContent (pipeline.go)          — content/ + data/ + stale-i18n check
//  3. renderMarkdownAndTree (pipeline.go)— per-page goldmark + tree + taxonomies
//  4. setupTemplatesAndWriter (setup)    — templates + i18n bundle + writer
//  5. buildContexts (setup)              — siteCtx + per-page ctx + links
//  6. renderPages (render)               — HTML + Markdown mirror + section RSS
//  7. renderFeedsAndSpecials (feeds)     — taxonomy/pagination/404/sitemap/etc.
//  8. copyStaticAndFinalize (write)      — static assets + stats
//
// Any stage error aborts the build immediately. Errors during per-page render
// are accumulated into Result.Errors instead.
func BuildSite(opts Options) (*Result, error) {
	start := time.Now()
	p := newPipeline(opts)

	stages := []struct {
		name string
		fn   func() error
	}{
		{"load config", p.loadConfig},
		{"load content", p.loadContent},
		{"render markdown + tree", p.renderMarkdownAndTree},
		{"setup templates + writer", p.setupTemplatesAndWriter},
	}
	for _, s := range stages {
		if err := s.fn(); err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
	}

	// buildContexts is infallible (no error return path).
	p.buildContexts()

	// render + postprocess stages: errors counted into Result.Errors,
	// not propagated. Build continues so partial output is produced.
	p.renderPages()
	p.renderFeedsAndSpecials()
	p.copyStaticAndFinalize()

	p.result.Duration = time.Since(start)
	return p.result, nil
}

// InjectLiveReload inserts the livereload <script> before </head>.
// Falls back to before </body> if </head> is absent.
// If neither is present, appends the script at the end.
func InjectLiveReload(html, wsURL string) string {
	host := hostFromURL(wsURL)
	port := portFromURL(wsURL)
	tag := `<script src="http://` + host + `:` + port +
		`/livereload.js?mindelay=10&v=2" data-livereload-port="` + port +
		`" data-livereload-host="` + host + `"></script>`
	if idx := strings.Index(html, "</head>"); idx >= 0 {
		return html[:idx] + tag + html[idx:]
	}
	if idx := strings.Index(html, "</body>"); idx >= 0 {
		return html[:idx] + tag + html[idx:]
	}
	return html + tag
}

func portFromURL(wsURL string) string {
	u, err := url.Parse(wsURL)
	if err != nil || u.Port() == "" {
		return "1313"
	}
	return u.Port()
}

func hostFromURL(wsURL string) string {
	u, err := url.Parse(wsURL)
	if err != nil || u.Hostname() == "" {
		return "localhost"
	}
	return u.Hostname()
}
