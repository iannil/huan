package output

import (
	"encoding/json"
	"regexp"
	"strings"
)

// CanonifyOptions controls how canonifyURLs rewrites paths.
type CanonifyOptions struct {
	BaseURL string // e.g., "https://zhurongshuo.com/"
	IsHome  bool   // if true, inject Hugo generator meta
}

// Keep the root regex for unusual base URLs containing regexp replacement expansions.
var canonifyBareRootPattern = regexp.MustCompile(`((?:href|src)=)/([\s>])`)
var canonifyCodeRegionPattern = regexp.MustCompile(`(?s)<(?:code|pre)(?:\s[^>]*)?>.*?</(?:code|pre)>`)

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
	if !strings.Contains(html, "href") && !strings.Contains(html, "src") {
		return html
	}
	if !strings.Contains(html, "<code") && !strings.Contains(html, "<pre") {
		return canonifySegment(html, base)
	}
	regions := canonifyCodeRegionPattern.FindAllStringIndex(html, -1)
	if len(regions) == 0 {
		return canonifySegment(html, base)
	}
	var sb strings.Builder
	lastEnd, copied := 0, 0
	changed := false
	rewrite := func(start, end int) {
		segment := html[start:end]
		out := canonifySegment(segment, base)
		if out != segment {
			if !changed {
				sb.Grow(len(html))
				changed = true
			}
			sb.WriteString(html[copied:start])
			sb.WriteString(out)
			copied = end
		}
	}
	for _, region := range regions {
		rewrite(lastEnd, region[0])
		lastEnd = region[1]
	}
	rewrite(lastEnd, len(html))
	if !changed {
		return html
	}
	sb.WriteString(html[copied:])
	return sb.String()
}

// Preserve the original three passes: a quoted value can contain a bare
// attribute, and the base URL inserted by one pass can match a later pass.
func canonifySegment(html, base string) string {
	html = rewriteCanonifyAttributes(html, base, canonifyQuoted)
	html = rewriteCanonifyAttributes(html, base, canonifyBare)
	if strings.Contains(base, "$") {
		// ReplaceAllString historically expands $ captures in the base URL.
		return canonifyBareRootPattern.ReplaceAllString(html, "${1}"+base+"/${2}")
	}
	return rewriteCanonifyAttributes(html, base, canonifyRoot)
}

type canonifyMode uint8

const (
	canonifyQuoted canonifyMode = iota
	canonifyBare
	canonifyRoot
)

// rewriteCanonifyAttributes scans exactly the old regex language, without an
// HTML parser (which would change malformed markup and attribute boundaries).
// Modes are quoted URLs, bare non-root URLs, and bare root URLs respectively.
func rewriteCanonifyAttributes(html, base string, mode canonifyMode) string {
	var sb strings.Builder
	copied := 0
	changed := false
	for pos := 0; pos < len(html); {
		offset := strings.IndexAny(html[pos:], "hs")
		if offset < 0 {
			break
		}
		start := pos + offset
		pos = start + 1
		cursor := start
		switch {
		case strings.HasPrefix(html[start:], "href"):
			cursor += 4
		case strings.HasPrefix(html[start:], "src"):
			cursor += 3
		default:
			continue
		}
		if mode == canonifyQuoted {
			for cursor < len(html) && canonifySpace(html[cursor]) {
				cursor++
			}
		}
		if cursor >= len(html) || html[cursor] != '=' {
			continue
		}
		cursor++
		if mode == canonifyQuoted {
			for cursor < len(html) && canonifySpace(html[cursor]) {
				cursor++
			}
			if cursor >= len(html) || html[cursor] != '"' {
				continue
			}
			cursor++
		}
		if cursor >= len(html) || html[cursor] != '/' {
			continue
		}
		pathStart := cursor
		cursor++
		switch mode {
		case canonifyQuoted:
			end := strings.IndexByte(html[cursor:], '"')
			if end < 0 {
				// No later quoted match can end either. Avoid rescanning a
				// long malformed value for every subsequent attribute name.
				pos = len(html)
				continue
			}
			cursor += end + 1
			pos = cursor
			if html[pathStart+1] == '/' {
				continue
			}
		case canonifyBare:
			for cursor < len(html) && !canonifySpace(html[cursor]) && html[cursor] != '"' && html[cursor] != '/' && html[cursor] != '>' {
				cursor++
			}
			if cursor == pathStart+1 {
				continue
			}
			pos = cursor
		case canonifyRoot:
			if cursor >= len(html) || (!canonifySpace(html[cursor]) && html[cursor] != '>') {
				continue
			}
			cursor++
			pos = cursor
		}
		if !changed {
			sb.Grow(len(html) + len(base))
			changed = true
		}
		sb.WriteString(html[copied:pathStart])
		sb.WriteString(base)
		sb.WriteString(html[pathStart:cursor])
		copied = cursor
	}
	if !changed {
		return html
	}
	sb.WriteString(html[copied:])
	return sb.String()
}

// Go regexp \s is ASCII whitespace, deliberately excluding vertical tab.
func canonifySpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
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
	if !strings.Contains(html, "application/ld+json") {
		return html
	}
	matches := jsonLDPattern.FindAllStringSubmatchIndex(html, -1)
	var sb strings.Builder
	copied := 0
	changed := false
	for _, match := range matches {
		bodyStart, bodyEnd := match[4], match[5]
		original := html[bodyStart:bodyEnd]
		body := strings.TrimSpace(original)
		if body == "" {
			continue
		}
		compact := compactJSONPreservingOrder(body)
		if compact == original {
			continue
		}
		// Keep Unmarshal rather than json.Valid: the old implementation leaves
		// overflowing JSON numbers such as 1e999 unchanged.
		var data interface{}
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			continue
		}
		if !changed {
			sb.Grow(len(html))
			changed = true
		}
		sb.WriteString(html[copied:bodyStart])
		sb.WriteString(compact)
		copied = bodyEnd
	}
	if !changed {
		return html
	}
	sb.WriteString(html[copied:])
	return sb.String()
}

// compactJSONPreservingOrder strips insignificant whitespace from a JSON string
// while preserving field order and original string escaping.
func compactJSONPreservingOrder(s string) string {
	var out strings.Builder
	changed := false
	inString := false
	i := 0
	for i < len(s) {
		c := s[i]
		if inString {
			if c == '\\' && i+1 < len(s) {
				if changed {
					out.WriteString(s[i : i+2])
				}
				i += 2
				continue
			}
			if c == '"' {
				inString = false
			}
			if changed {
				out.WriteByte(c)
			}
			i++
			continue
		}
		switch c {
		case '"':
			inString = true
			if changed {
				out.WriteByte(c)
			}
		case ' ', '\t', '\n', '\r':
			if !changed {
				out.Grow(len(s))
				out.WriteString(s[:i])
				changed = true
			}
		default:
			if changed {
				out.WriteByte(c)
			}
		}
		i++
	}
	if !changed {
		return s
	}
	return out.String()
}
