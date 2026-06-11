package markdown

import (
	"bytes"
	"fmt"
	htmlstd "html"
	"regexp"
	"strings"

	"github.com/novel_ttl/huan/internal/config"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// Renderer wraps goldmark for Markdown→HTML conversion.
type Renderer struct {
	gm goldmark.Markdown
}

// NewRenderer creates a goldmark-based Markdown renderer from config.
// Heading IDs (anchor links) are enabled by default to match Hugo's behavior.
func NewRenderer(cfg *config.MarkupConfig) *Renderer {
	extensions := []goldmark.Extender{
		extension.GFM,      // GitHub Flavored Markdown (tables, strikethrough, etc.)
		extension.Linkify,  // Auto-detect bare URLs
		extension.TaskList, // GitHub-style task lists
	}
	typoEnabled := cfg == nil || cfg.Goldmark.Extensions.Typographer
	if typoEnabled {
		extensions = append(extensions, extension.Typographer)
	}

	opts := []goldmark.Option{
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	}

	if cfg != nil && cfg.Goldmark.Renderer.Unsafe {
		opts = append(opts, goldmark.WithRendererOptions(html.WithUnsafe()))
	}

	return &Renderer{
		gm: goldmark.New(opts...),
	}
}

// Render converts Markdown source to HTML.
// After goldmark rendering, a post-processor rewrites heading IDs to match
// Hugo's behavior (preserving CJK characters instead of falling back to "heading").
func (r *Renderer) Render(src string) (string, error) {
	var buf bytes.Buffer
	if err := r.gm.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return rewriteHeadingIDs(buf.String()), nil
}

// headingPattern matches <h1>-<h6> tags with an id attribute.
var headingPattern = regexp.MustCompile(`(?s)<(h[1-6])\s+id="([^"]+)">(.*?)</(h[1-6])>`)

// rewriteHeadingIDs replaces goldmark's auto-generated heading IDs with
// Hugo-style IDs that preserve CJK and other Unicode characters.
func rewriteHeadingIDs(html string) string {
	seen := map[string]int{}
	return headingPattern.ReplaceAllStringFunc(html, func(match string) string {
		parts := headingPattern.FindStringSubmatch(match)
		if parts == nil || parts[1] != parts[4] {
			return match
		}
		tag := parts[1]
		currentID := parts[2]
		text := parts[3]

		// Hugo re-derives IDs from heading text, preserving CJK. We always
		// recompute (goldmark's IDs strip CJK, producing things like "a" for
		// "附录A").
		newID := hugoSlugify(strings.TrimSpace(stripHTMLTags(text)))
		if newID == "" {
			newID = currentID // fallback if text has no slugifiable chars
		}
		// Deduplicate: append -1, -2, etc. on collision
		if n, exists := seen[newID]; exists {
			n++
			seen[newID] = n
			newID = fmt.Sprintf("%s-%d", newID, n-1)
		} else {
			seen[newID] = 1
		}

		return "<" + tag + ` id="` + newID + `">` + text + "</" + tag + ">"
	})
}

// hugoSlugify converts a heading text to a Hugo-style ID.
// Lowercases ASCII letters, preserves CJK ideographs, drops most punctuation,
// replaces whitespace with -. Consecutive separators (e.g. " · " between CJK)
// are kept distinct (matching Hugo's Blackfriday behavior of emitting "--").
// HTML entities like &quot; &amp; are dropped (their literal chars would be
// escaped HTML, not the original characters).
func hugoSlugify(s string) string {
	// Unescape common HTML entities so they're handled as their literal chars
	// would be (e.g., &quot; → " → dropped).
	s = htmlstd.UnescapeString(s)

	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r + 32)
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			sb.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n' || r == '-' || r == '_':
			sb.WriteByte('-')
		case r >= 0x4E00 && r <= 0x9FFF, // CJK Unified Ideographs
			r >= 0x3040 && r <= 0x309F, // Hiragana
			r >= 0x30A0 && r <= 0x30FF: // Katakana
			sb.WriteRune(r)
		default:
			// Drop punctuation/symbols but DO NOT collapse surrounding separators.
		}
	}
	return strings.Trim(sb.String(), "-")
}

// stripHTMLTags removes inline HTML tags (e.g., <code>) from heading text.
var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

func stripHTMLTags(s string) string {
	return htmlTagPattern.ReplaceAllString(s, "")
}
