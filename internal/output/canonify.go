package output

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// CanonifyOptions controls how canonifyURLs rewrites paths.
type CanonifyOptions struct {
	BaseURL   string // e.g., "https://zhurongshuo.com/"
	IsHome    bool   // if true, inject Hugo generator meta
}

// canonifyQuotedPattern matches href="/..." and src="/..." with double quotes.
// canonifyBarePattern matches href=/... and src=/... without quotes (after minify).
// canonifyBareRootPattern matches href=/ and src=/ (bare root path).
var canonifyQuotedPattern = regexp.MustCompile(`((?:href|src)\s*=\s*")(/[^"]*")`)
var canonifyBarePattern = regexp.MustCompile(`((?:href|src)=)(/[^\s"/>]+)`)
var canonifyBareRootPattern = regexp.MustCompile(`((?:href|src)=)/([\s>])`)

// Canonify rewrites root-relative URLs in href/src attributes to absolute URLs.
// Mirrors Hugo's canonifyURLs = true behavior.
//
// Handles both quoted and unquoted (post-minify) attribute values.
// Skips protocol-relative URLs (//example.com) and absolute http(s) URLs.
//
// Skips <code> and <pre> regions: URLs inside code blocks are raw text content
// (e.g., a code sample showing `<link href=/api/foo />`), not actual HTML
// attributes. Hugo's canonifyURLs does not rewrite inside code/pre either.
func Canonify(html string, opts CanonifyOptions) string {
	if opts.BaseURL == "" {
		return html
	}
	base := strings.TrimRight(opts.BaseURL, "/")

	// Segmented rewrite: walk HTML, apply patterns only outside code/pre.
	html = applyCanonifyOutsideCode(html, base)

	// Inject Hugo generator meta tag (home page only)
	if opts.IsHome {
		html = injectGenerator(html)
	}

	// Finally: minify JSON-LD script contents (Hugo does this).
	html = minifyJSONLD(html)

	return html
}

// codeOpenRe matches opening <code> or <pre> tags (with any attributes, post-minify).
// codeCloseRe matches the corresponding close tags.
var (
	codeRegionOpenRe  = regexp.MustCompile(`<(?:code|pre)(?:\s[^>]*)?>`)
	codeRegionCloseRe = regexp.MustCompile(`</(?:code|pre)>`)
)

// applyCanonifyOutsideCode splits the HTML into segments separated by
// <code>/<pre> regions, applies canonify to OUTSIDE segments only, and emits
// inside segments verbatim. Inside code/pre, content is escaped text
// representing source code samples — URLs in there are not real HTML
// attributes and must not be rewritten.
//
// Implementation: use a combined regex to split at code/pre boundaries. The
// split preserves the tags themselves (they end up in the "outside" segments
// at their boundaries, which is fine since the bare/quoted patterns don't
// match the tags themselves).
func applyCanonifyOutsideCode(html, base string) string {
	// regionRe matches a full <code>...</code> or <pre>...</pre> block (greedy
	// is OK because we treat anything inside as "raw text" — if a code block
	// contains another open tag, that's literal text).
	regionRe := regexp.MustCompile(`(?s)<(?:code|pre)(?:\s[^>]*)?>.*?</(?:code|pre)>`)

	var sb strings.Builder
	lastEnd := 0
	for _, m := range regionRe.FindAllStringIndex(html, -1) {
		start, end := m[0], m[1]
		// Canonify the outside text before this region.
		if start > lastEnd {
			sb.WriteString(canonifySegment(html[lastEnd:start], base))
		}
		// Emit the code/pre region verbatim.
		sb.WriteString(html[start:end])
		lastEnd = end
	}
	// Canonify any trailing outside text.
	if lastEnd < len(html) {
		sb.WriteString(canonifySegment(html[lastEnd:], base))
	}
	return sb.String()
}

// canonifySegment applies the three canonify patterns to a code-free HTML segment.
func canonifySegment(html, base string) string {
	html = canonifyQuotedPattern.ReplaceAllStringFunc(html, func(match string) string {
		parts := canonifyQuotedPattern.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		path := strings.TrimPrefix(parts[2], `/`)
		if strings.HasPrefix(path, "/") {
			return match
		}
		return parts[1] + base + "/" + path
	})

	html = canonifyBarePattern.ReplaceAllStringFunc(html, func(match string) string {
		parts := canonifyBarePattern.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		path := strings.TrimPrefix(parts[2], `/`)
		if strings.HasPrefix(path, "/") {
			return match
		}
		return parts[1] + base + "/" + path
	})

	html = canonifyBareRootPattern.ReplaceAllString(html, "${1}"+base+"/${2}")
	return html
}

// injectGenerator inserts `<meta name=generator content="Hugo X.Y">` immediately
// after the opening <head> tag, matching Hugo's behavior (home page only).
func injectGenerator(html string) string {
	headIdx := strings.Index(html, "<head>")
	if headIdx < 0 {
		return html
	}
	insertAt := headIdx + len("<head>")
	gen := `<meta name=generator content="Hugo 0.160.1">`
	return html[:insertAt] + gen + html[insertAt:]
}

// jsonLDPattern matches <script type="application/ld+json">...</script> blocks.
// After tdewolff's HTML minifier, the type attribute may be unquoted.
var jsonLDPattern = regexp.MustCompile(`(?s)(<script[^>]*type=["']?application/ld\+json["']?[^>]*>)(.*?)(</script>)`)

// percentEncodedLowerPattern matches %xx sequences (lowercase hex) in URLs.
var percentEncodedLowerPattern = regexp.MustCompile(`(%[0-9a-f]{2})`)

// uppercasePercentEncoding converts %xx (lowercase hex) to %XX (uppercase hex).
// Go's html/template emits lowercase percent-encoding in URLs; Hugo uses
// uppercase. Applied to URL attributes only (href/src).
func uppercasePercentEncoding(html string) string {
	return percentEncodedLowerPattern.ReplaceAllStringFunc(html, func(s string) string {
		return strings.ToUpper(s)
	})
}
// single-line minified JSON, matching Hugo's output. It preserves field order
// by stripping whitespace rather than re-encoding.
func minifyJSONLD(html string) string {
	return jsonLDPattern.ReplaceAllStringFunc(html, func(match string) string {
		parts := jsonLDPattern.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		body := strings.TrimSpace(parts[2])
		if body == "" {
			return match
		}

		// Validate JSON first; if invalid, leave it untouched.
		var data interface{}
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			return match
		}

		// Compact whitespace-only removal: collapse runs of whitespace to a single
		// space inside strings, and remove all whitespace between tokens.
		compact := compactJSONPreservingOrder(body)
		return parts[1] + compact + parts[3]
	})
}

// compactJSONPreservingOrder strips insignificant whitespace from a JSON string
// while preserving field order and original string escaping.
func compactJSONPreservingOrder(s string) string {
	var out bytes.Buffer
	inString := false
	i := 0
	for i < len(s) {
		c := s[i]
		if inString {
			if c == '\\' && i+1 < len(s) {
				out.WriteByte(c)
				out.WriteByte(s[i+1])
				i += 2
				continue
			}
			if c == '"' {
				inString = false
			}
			out.WriteByte(c)
			i++
			continue
		}
		switch c {
		case '"':
			inString = true
			out.WriteByte(c)
		case ' ', '\t', '\n', '\r':
			// skip whitespace outside strings
		default:
			out.WriteByte(c)
		}
		i++
	}
	return out.String()
}
