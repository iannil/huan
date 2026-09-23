// Frozen pre-optimization implementation: differential oracle for byte compatibility.
package output

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// legacyCanonifyQuotedPattern matches href="/..." and src="/..." with double quotes.
// legacyCanonifyBarePattern matches href=/... and src=/... without quotes (after minify).
// legacyCanonifyBareRootPattern matches href=/ and src=/ (bare root path).
var legacyCanonifyQuotedPattern = regexp.MustCompile(`((?:href|src)\s*=\s*")(/[^"]*")`)
var legacyCanonifyBarePattern = regexp.MustCompile(`((?:href|src)=)(/[^\s"/>]+)`)
var legacyCanonifyBareRootPattern = regexp.MustCompile(`((?:href|src)=)/([\s>])`)
var legacyCanonifyCodeRegionPattern = regexp.MustCompile(`(?s)<(?:code|pre)(?:\s[^>]*)?>.*?</(?:code|pre)>`)

// legacyCanonify rewrites root-relative URLs in href/src attributes to absolute URLs.
// Mirrors Hugo's canonifyURLs = true behavior.
//
// Handles both quoted and unquoted (post-minify) attribute values.
// Skips protocol-relative URLs (//example.com) and absolute http(s) URLs.
//
// Skips <code> and <pre> regions: URLs inside code blocks are raw text content
// (e.g., a code sample showing `<link href=/api/foo />`), not actual HTML
// attributes. Hugo's canonifyURLs does not rewrite inside code/pre either.
func legacyCanonify(html string, opts CanonifyOptions) string {
	if opts.BaseURL == "" {
		return html
	}
	base := strings.TrimRight(opts.BaseURL, "/")

	// Segmented rewrite: walk HTML, apply patterns only outside code/pre.
	html = legacyApplyCanonifyOutsideCode(html, base)

	// Inject Hugo generator meta tag (home page only)
	if opts.IsHome {
		html = legacyInjectGenerator(html)
	}

	// Finally: minify JSON-LD script contents (Hugo does this).
	html = legacyMinifyJSONLD(html)

	return html
}

// legacyApplyCanonifyOutsideCode splits the HTML into segments separated by
// <code>/<pre> regions, applies canonify to OUTSIDE segments only, and emits
// inside segments verbatim. Inside code/pre, content is escaped text
// representing source code samples — URLs in there are not real HTML
// attributes and must not be rewritten.
//
// Implementation: use a combined regex to split at code/pre boundaries. The
// split preserves the tags themselves (they end up in the "outside" segments
// at their boundaries, which is fine since the bare/quoted patterns don't
// match the tags themselves).
func legacyApplyCanonifyOutsideCode(html, base string) string {
	var sb strings.Builder
	lastEnd := 0
	for _, m := range legacyCanonifyCodeRegionPattern.FindAllStringIndex(html, -1) {
		start, end := m[0], m[1]
		// legacyCanonify the outside text before this region.
		if start > lastEnd {
			sb.WriteString(legacyCanonifySegment(html[lastEnd:start], base))
		}
		// Emit the code/pre region verbatim.
		sb.WriteString(html[start:end])
		lastEnd = end
	}
	// legacyCanonify any trailing outside text.
	if lastEnd < len(html) {
		sb.WriteString(legacyCanonifySegment(html[lastEnd:], base))
	}
	return sb.String()
}

// legacyCanonifySegment applies the three canonify patterns to a code-free HTML segment.
func legacyCanonifySegment(html, base string) string {
	html = legacyRewriteCanonifyMatches(html, base, legacyCanonifyQuotedPattern)
	html = legacyRewriteCanonifyMatches(html, base, legacyCanonifyBarePattern)

	html = legacyCanonifyBareRootPattern.ReplaceAllString(html, "${1}"+base+"/${2}")
	return html
}

// legacyRewriteCanonifyMatches rewrites matching attributes from capture offsets
// produced by one regex scan. Both patterns expose the prefix and URL in
// groups 1 and 2 respectively.
func legacyRewriteCanonifyMatches(html, base string, pattern *regexp.Regexp) string {
	matches := pattern.FindAllStringSubmatchIndex(html, -1)
	if len(matches) == 0 {
		return html
	}

	var sb strings.Builder
	lastEnd := 0
	changed := false
	for _, match := range matches {
		pathStart, pathEnd := match[4], match[5]
		// A second leading slash is a protocol-relative URL.
		if pathStart < 0 || pathEnd <= pathStart+1 || html[pathStart+1] == '/' {
			continue
		}
		if !changed {
			sb.Grow(len(html) + len(matches)*(len(base)+1))
			changed = true
		}
		sb.WriteString(html[lastEnd:match[0]])
		sb.WriteString(html[match[2]:match[3]])
		sb.WriteString(base)
		sb.WriteByte('/')
		sb.WriteString(html[pathStart+1 : pathEnd])
		lastEnd = match[1]
	}
	if !changed {
		return html
	}
	sb.WriteString(html[lastEnd:])
	return sb.String()
}

// legacyInjectGenerator inserts `<meta name=generator content="Hugo X.Y">` immediately
// after the opening <head> tag, matching Hugo's behavior (home page only).
func legacyInjectGenerator(html string) string {
	headIdx := strings.Index(html, "<head>")
	if headIdx < 0 {
		return html
	}
	insertAt := headIdx + len("<head>")
	gen := `<meta name=generator content="Hugo 0.160.1">`
	return html[:insertAt] + gen + html[insertAt:]
}

// legacyJsonLDPattern matches <script type="application/ld+json">...</script> blocks.
// After tdewolff's HTML minifier, the type attribute may be unquoted.
var legacyJsonLDPattern = regexp.MustCompile(`(?s)(<script[^>]*type=["']?application/ld\+json["']?[^>]*>)(.*?)(</script>)`)

// legacyPercentEncodedLowerPattern matches %xx sequences (lowercase hex) in URLs.
var legacyPercentEncodedLowerPattern = regexp.MustCompile(`(%[0-9a-f]{2})`)

// legacyUppercasePercentEncoding converts %xx (lowercase hex) to %XX (uppercase hex).
// Go's html/template emits lowercase percent-encoding in URLs; Hugo uses
// uppercase. Applied to URL attributes only (href/src).
func legacyUppercasePercentEncoding(html string) string {
	return legacyPercentEncodedLowerPattern.ReplaceAllStringFunc(html, func(s string) string {
		return strings.ToUpper(s)
	})
}

// single-line minified JSON, matching Hugo's output. It preserves field order
// by stripping whitespace rather than re-encoding.
func legacyMinifyJSONLD(html string) string {
	return legacyJsonLDPattern.ReplaceAllStringFunc(html, func(match string) string {
		parts := legacyJsonLDPattern.FindStringSubmatch(match)
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
		compact := legacyCompactJSONPreservingOrder(body)
		return parts[1] + compact + parts[3]
	})
}

// legacyCompactJSONPreservingOrder strips insignificant whitespace from a JSON string
// while preserving field order and original string escaping.
func legacyCompactJSONPreservingOrder(s string) string {
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
