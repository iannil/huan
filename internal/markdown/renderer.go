package markdown

import (
	"bytes"
	"fmt"
	htmlstd "html"
	"regexp"
	"strings"

	"github.com/iannil/huan/internal/config"
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
		extension.Footnote, // PHP Markdown Extra footnotes (matches Hugo default)
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
// After goldmark rendering, post-processors:
//   - rewrite heading IDs to match Hugo's behavior (preserving CJK characters
//     instead of falling back to "heading")
//   - normalize HTML entities in code/inline-code to numeric form (&#34; /
//     &#39;) to match Hugo's chroma output (goldmark emits named entities
//     like &quot; by default)
func (r *Renderer) Render(src string) (string, error) {
	var buf bytes.Buffer
	if err := r.gm.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	out := rewriteHeadingIDs(buf.String())
	out = normalizeCodeEntities(out)
	return out, nil
}

// codeEntityReplacements is applied inside <code> and <pre><code> regions to
// match Hugo's chroma entity encoding (numeric rather than goldmark's named).
// The replacement is context-scoped via regex to avoid touching entities in
// regular text, attributes, or JSON-LD <script> payloads.
//
// Goldmark emits:
//   - &quot; for " inside code
//   - raw ' inside code (no escaping)
//
// Hugo/chroma emits:
//   - &#34; for "
//   - &#39; for '
var (
	codeOpenRe  = regexp.MustCompile(`<(code|pre)[^>]*>`)
	codeCloseRe = regexp.MustCompile(`</(code|pre)>`)
)

// normalizeCodeEntities rewrites entity encoding inside <code>/<pre> regions
// from goldmark's named/raw form to Hugo's numeric form. Only the interior
// of code-bearing elements is touched so HTML attributes, JSON-LD <script>
// blocks, and regular paragraph text are unaffected.
func normalizeCodeEntities(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	pos := 0
	inCode := 0 // nested code depth (e.g. <pre><code>)
	for pos < len(s) {
		// Check for code/pre open tag
		if m := codeOpenRe.FindStringIndex(s[pos:]); m != nil {
			absStart := pos + m[0]
			absEnd := pos + m[1]
			// Flush preceding literal text unchanged
			sb.WriteString(s[pos:absStart])
			// Write tag verbatim
			sb.WriteString(s[absStart:absEnd])
			pos = absEnd
			inCode++
			continue
		}
		// Check for close tag if we're inside code
		if inCode > 0 {
			if m := codeCloseRe.FindStringIndex(s[pos:]); m != nil {
				absStart := pos + m[0]
				// Rewrite the interior region [pos, absStart) for entity encoding
				interior := s[pos:absStart]
				sb.WriteString(rewriteCodeInterior(interior))
				// Write close tag verbatim
				absEnd := pos + m[1]
				sb.WriteString(s[absStart:absEnd])
				pos = absEnd
				inCode--
				continue
			}
		}
		// No match — emit rest of string unchanged
		sb.WriteString(s[pos:])
		break
	}
	return sb.String()
}

// rewriteCodeInterior converts goldmark's entity encoding inside code to
// Hugo's numeric form: &quot; → &#34; and raw ' → &#39;. The "raw apostrophe"
// conversion matches chroma's behavior of escaping ' in code spans/blocks.
func rewriteCodeInterior(s string) string {
	// Replace &quot; first (named entity for ")
	t := strings.ReplaceAll(s, "&quot;", "&#34;")
	// Then escape raw ' as &#39;. Use a byte-by-byte scan to skip over
	// already-escaped sequences (e.g. &#39; itself, &amp;).
	var b strings.Builder
	b.Grow(len(t))
	for i := 0; i < len(t); i++ {
		c := t[i]
		if c == '\'' {
			b.WriteString("&#39;")
			continue
		}
		// Leave recognized entity sequences alone (don't escape ' inside them)
		if c == '&' {
			// Look ahead for a recognized entity (e.g. &#34; &amp; &#39; &lt; &gt;)
			end := strings.IndexByte(t[i:], ';')
			if end > 0 && end < 16 {
				candidate := t[i : i+end+1]
				if isHtmlEntity(candidate) {
					b.WriteString(candidate)
					i += end
					continue
				}
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// isHtmlEntity reports whether s looks like a recognized HTML entity reference
// (named or numeric). Used to avoid double-escaping apostrophes inside
// already-encoded entities like &#39; or &apos;.
func isHtmlEntity(s string) bool {
	if !strings.HasPrefix(s, "&") || !strings.HasSuffix(s, ";") {
		return false
	}
	inner := s[1 : len(s)-1]
	if inner == "" {
		return false
	}
	// Numeric: &#NN; or &#xNN;
	if strings.HasPrefix(inner, "#") {
		body := inner[1:]
		if body == "" {
			return false
		}
		if strings.HasPrefix(body, "x") || strings.HasPrefix(body, "X") {
			body = body[1:]
		}
		for _, r := range body {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
		return len(body) > 0
	}
	// Named: letters only
	for _, r := range inner {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
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
