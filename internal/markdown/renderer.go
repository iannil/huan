package markdown

import (
	"bytes"
	"fmt"
	gohtml "html"
	htmlstd "html"
	"io"
	"regexp"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/iannil/huan/internal/config"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// Renderer wraps goldmark for Markdown→HTML conversion.
type Renderer struct {
	gm goldmark.Markdown
}

// NewRenderer creates a goldmark-based Markdown renderer from config.
// Heading IDs (anchor links) are enabled by default to match Hugo's behavior.
// Fenced code blocks are syntax-highlighted using chroma (matching Hugo's output).
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
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			renderer.WithNodeRenderers(
				util.Prioritized(&chromaCodeBlockRenderer{
					style:        "monokai",
					guessSyntax:  true,
					wrapperClass: "highlight",
				}, 200),
			),
		),
	}

	return &Renderer{
		gm: goldmark.New(opts...),
	}
}

// Render converts Markdown source to HTML.
// After goldmark rendering, post-processors:
//   - rewrite heading IDs to match Hugo's behavior (preserving CJK characters
//     instead of falling back to "heading")
//   - normalize HTML entities in inline-code to numeric form (fenced code
//     blocks already use chroma which emits numeric entities natively)
func (r *Renderer) Render(src string) (string, error) {
	var buf bytes.Buffer
	if err := r.gm.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	out := rewriteHeadingIDs(buf.String())
	out = normalizeInlineCodeEntities(out)
	out = expandEmojiShortcodes(out)
	return out, nil
}

// ---------------------------------------------------------------------------
// Chroma-based fenced code block renderer (matches Hugo's highlight output)
// ---------------------------------------------------------------------------

// chromaCodeBlockRenderer renders fenced code blocks using chroma syntax
// highlighting, matching Hugo's exact HTML output structure:
//
//	<div class="highlight">
//	  <pre tabindex="0" class="chroma">
//	    <code class="language-LANG" data-lang="LANG">
//	      <span class="line"><span class="cl">...</span></span>
//	    </code>
//	  </pre>
//	</div>
type chromaCodeBlockRenderer struct {
	html.Config
	style        string
	guessSyntax  bool
	wrapperClass string
}

// RegisterFuncs tells goldmark to use this renderer for fenced code blocks.
func (r *chromaCodeBlockRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
}

// renderFencedCodeBlock renders a fenced code block with chroma highlighting.
func (r *chromaCodeBlockRenderer) renderFencedCodeBlock(
	w util.BufWriter, source []byte, node ast.Node, entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	n := node.(*ast.FencedCodeBlock)
	language := n.Language(source)
	langStr := ""
	if language != nil {
		langStr = string(language)
	}

	// Collect code content
	var buf bytes.Buffer
	for i := 0; i < n.Lines().Len(); i++ {
		line := n.Lines().At(i)
		buf.Write(line.Value(source))
	}
	code := buf.String()

	// Try to get a lexer
	var lexer chroma.Lexer
	if langStr != "" {
		lexer = lexers.Get(langStr)
	}
	if lexer == nil && r.guessSyntax {
		lexer = lexers.Analyse(code)
		if lexer == nil {
			lexer = lexers.Fallback
		}
		langStr = strings.ToLower(lexer.Config().Name)
	}

	style := styles.Get(r.style)
	if style == nil {
		style = styles.Fallback
	}

	// No lexer found: render as plain text (matching Hugo's no-lexer path)
	if lexer == nil {
		hugoWritePreStart(w, langStr, "")
		w.WriteString(gohtml.EscapeString(code))
		w.WriteString("</code></pre>")
		return ast.WalkContinue, nil
	}

	lexer = chroma.Coalesce(lexer)
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		// Fallback to plain rendering on tokenisation error
		hugoWritePreStart(w, langStr, "")
		w.WriteString(gohtml.EscapeString(code))
		w.WriteString("</code></pre>")
		return ast.WalkContinue, nil
	}

	// Write wrapper: <div class="highlight">
	fmt.Fprintf(w, `<div class="%s">`, r.wrapperClass)

	// Create chroma formatter with Hugo-compatible options:
	//   - WithClasses(true): use CSS classes instead of inline styles
	//     (matches pygmentsUseClasses=true in zhurongshuo config)
	//   - WithPreWrapper: custom pre/code wrapper matching Hugo's structure
	formatter := chromahtml.New(
		chromahtml.WithClasses(true),
		chromahtml.WithPreWrapper(&hugoPreWrapper{langStr}),
	)

	if err := formatter.Format(w, style, iterator); err != nil {
		w.WriteString("</div>")
		return ast.WalkContinue, err
	}

	w.WriteString("</div>")
	return ast.WalkContinue, nil
}

// hugoPreWrapper implements chroma's html.PreWrapper interface to produce
// Hugo-compatible <pre> and <code> tags. It strips the mode class (e.g. "dark")
// that chroma v2 adds to the PreWrapper styleAttr, since Hugo's older chroma
// version does not produce this class.
type hugoPreWrapper struct {
	language string
}

// Start writes the opening <pre> and <code> tags, matching Hugo's WritePreStart:
//
//	<pre tabindex="0" [styleAttr]><code [class="language-LANG" data-lang="LANG"]>
func (p *hugoPreWrapper) Start(code bool, styleAttr string) string {
	// Chroma v2 appends a mode class like "dark" to the styleAttr for the
	// PreWrapper (e.g. ` class="chroma dark"`). Hugo's chroma version does
	// not have this feature, so we strip the mode class to match Hugo output.
	styleAttr = stripModeClass(styleAttr)
	var sb strings.Builder
	hugoWritePreStart(&sb, p.language, styleAttr)
	return sb.String()
}

// End writes the closing </code></pre> tags.
func (p *hugoPreWrapper) End(code bool) string {
	return "</code></pre>"
}

// hugoWritePreStart writes Hugo-compatible <pre><code> opening tags:
//
//	<pre tabindex="0" [styleAttr]><code [class="language-LANG" data-lang="LANG"]>
func hugoWritePreStart(w io.Writer, language, styleAttr string) {
	fmt.Fprintf(w, `<pre tabindex="0"%s>`, styleAttr)
	w.Write([]byte("<code"))
	if language != "" {
		fmt.Fprintf(w, ` class="language-%s"`, language)
		fmt.Fprintf(w, ` data-lang="%s"`, language)
	}
	w.Write([]byte(">"))
}

// stripModeClass removes the mode class (e.g. "dark", "light") that chroma v2
// appends to the PreWrapper styleAttr. Chroma v2 generates:
//
//	` class="chroma dark"`
//
// but Hugo's older chroma only produces:
//
//	` class="chroma"`
//
// This function strips the mode suffix to match Hugo's output byte-for-byte.
var modeClassRe = regexp.MustCompile(` class="chroma\s+\w+"`)

func stripModeClass(styleAttr string) string {
	return modeClassRe.ReplaceAllString(styleAttr, ` class="chroma"`)
}

// ---------------------------------------------------------------------------
// Inline code entity normalization
// ---------------------------------------------------------------------------

var (
	inlineCodeOpenRe  = regexp.MustCompile(`<code[^>]*>`)
	inlineCodeCloseRe = regexp.MustCompile(`</code>`)
)

// normalizeInlineCodeEntities rewrites entity encoding inside standalone
// <code> tags (inline code) from goldmark's named/raw form to Hugo's numeric
// form. Fenced code blocks are rendered by chroma and already use numeric
// entities, so only inline code needs this post-processing.
func normalizeInlineCodeEntities(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	pos := 0
	for pos < len(s) {
		// Check for <code> open tag
		if m := inlineCodeOpenRe.FindStringIndex(s[pos:]); m != nil {
			absStart := pos + m[0]
			absEnd := pos + m[1]
			// Flush preceding literal text unchanged
			sb.WriteString(s[pos:absStart])
			// Write tag verbatim
			sb.WriteString(s[absStart:absEnd])
			pos = absEnd
			// Find the matching </code>
			if cm := inlineCodeCloseRe.FindStringIndex(s[pos:]); cm != nil {
				closeStart := pos + cm[0]
				interior := s[pos:closeStart]
				sb.WriteString(rewriteCodeInterior(interior))
				closeEnd := pos + cm[1]
				sb.WriteString(s[closeStart:closeEnd])
				pos = closeEnd
				continue
			}
		}
		// No match — emit rest of string unchanged
		sb.WriteString(s[pos:])
		break
	}
	return sb.String()
}

// rewriteCodeInterior converts goldmark's entity encoding inside inline code to
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

// ---------------------------------------------------------------------------
// Heading ID rewriting
// ---------------------------------------------------------------------------

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
// Matches Hugo's Goldmark heading ID generation:
//   - Lowercases ASCII letters, preserves digits
//   - Preserves CJK ideographs, Hiragana, Katakana, and other Unicode letters
//     (e.g. Greek π, Cyrillic) — Hugo's slugify treats all Unicode letters as word chars
//   - Preserves underscore (Hugo treats _ as a word char, not a separator)
//   - Replaces whitespace and hyphens with single dash
//   - Drops other punctuation/symbols without collapsing surrounding separators
//   - Does NOT trim leading/trailing dashes (Hugo keeps trailing dashes)
func hugoSlugify(s string) string {
	s = htmlstd.UnescapeString(s)

	// Hugo's Goldmark extension generates heading IDs from the last text segment
	// when the heading contains soft breaks (newlines). For multi-line headings
	// (e.g. inside blockquotes), only the last non-empty line contributes to the ID.
	if idx := strings.LastIndex(s, "\n"); idx >= 0 {
		lastLine := strings.TrimRight(s[idx+1:], " \t")
		if lastLine != "" {
			s = lastLine
		}
	}

	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r + 32)
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			sb.WriteRune(r)
		case r == '_':
			sb.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n' || r == '-':
			sb.WriteByte('-')
		case r >= 0x4E00 && r <= 0x9FFF, // CJK Unified Ideographs
			r >= 0x3040 && r <= 0x309F, // Hiragana
			r >= 0x30A0 && r <= 0x30FF: // Katakana
			sb.WriteRune(r)
		case unicode.IsLetter(r):
			sb.WriteRune(unicode.ToLower(r))
		default:
		}
	}
	return sb.String()
}

// stripHTMLTags removes inline HTML tags (e.g., <code>) from heading text.
var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

func stripHTMLTags(s string) string {
	return htmlTagPattern.ReplaceAllString(s, "")
}

// ---------------------------------------------------------------------------
// Emoji shortcode expansion
// ---------------------------------------------------------------------------

// hugoEmojiMap maps emoji shortcodes to their HTML numeric entity form,
// matching Hugo's built-in emoji rendering. Only shortcodes that appear
// in the content are included.
var hugoEmojiMap = map[string]string{
	":white_check_mark:": "&#9989;",   // ✅ U+2705
	":warning:":          "&#9888;&#xfe0f;", // ⚠️ U+26A0 U+FE0F
	":x:":                "&#x274c;",        // ❌ U+274C
}

var emojiShortcodeRe = regexp.MustCompile(`:[a-z0-9_]+:`)

// expandEmojiShortcodes replaces known emoji shortcodes with their HTML entity
// equivalents, but only outside <pre> and <code> blocks (where shortcodes are
// literal text).
func expandEmojiShortcodes(s string) string {
	// Build set of protected regions (inside <pre>...</pre> and <code>...</code>)
	var protected []struct{ start, end int }
	for _, tag := range []string{"pre", "code"} {
		open := "<" + tag
		closeTag := "</" + tag + ">"
		pos := 0
		for pos < len(s) {
			idx := strings.Index(s[pos:], open)
			if idx < 0 {
				break
			}
			absOpen := pos + idx
			closeIdx := strings.Index(s[absOpen:], closeTag)
			if closeIdx < 0 {
				break
			}
			absClose := absOpen + closeIdx + len(closeTag)
			protected = append(protected, struct{ start, end int }{absOpen, absClose})
			pos = absClose
		}
	}

	// If nothing to replace, return early
	if len(protected) == 0 {
		return s
	}

	// Find all shortcode matches and filter out those in protected regions
	replacements := emojiShortcodeRe.FindAllStringIndex(s, -1)
	if len(replacements) == 0 {
		return s
	}

	// Build result
	var sb strings.Builder
	sb.Grow(len(s))
	lastEnd := 0
	for _, m := range replacements {
		start, end := m[0], m[1]
		candidate := s[start:end]
		replacement, ok := hugoEmojiMap[candidate]
		if !ok {
			continue
		}
		// Check if this match is inside a protected region
		inProtected := false
		for _, p := range protected {
			if start >= p.start && end <= p.end {
				inProtected = true
				break
			}
		}
		if inProtected {
			continue
		}
		sb.WriteString(s[lastEnd:start])
		sb.WriteString(replacement)
		lastEnd = end
	}
	sb.WriteString(s[lastEnd:])
	return sb.String()
}
