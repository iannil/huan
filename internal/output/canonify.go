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
func Canonify(html string, opts CanonifyOptions) string {
	if opts.BaseURL == "" {
		return html
	}
	base := strings.TrimRight(opts.BaseURL, "/")

	// First: quoted values
	html = canonifyQuotedPattern.ReplaceAllStringFunc(html, func(match string) string {
		parts := canonifyQuotedPattern.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		// parts[2] looks like `/foo/bar"` - strip the leading / before prepending base
		path := strings.TrimPrefix(parts[2], `/`)
		if strings.HasPrefix(path, "/") { // protocol-relative //
			return match
		}
		return parts[1] + base + "/" + path
	})

	// Second: bare values (post-minify)
	html = canonifyBarePattern.ReplaceAllStringFunc(html, func(match string) string {
		parts := canonifyBarePattern.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		// parts[2] is the path starting with / - strip it before prepending base
		path := strings.TrimPrefix(parts[2], `/`)
		if strings.HasPrefix(path, "/") { // protocol-relative
			return match
		}
		return parts[1] + base + "/" + path
	})

	// Third: bare root path (href=/ followed by space or >)
	html = canonifyBareRootPattern.ReplaceAllString(html, "${1}" + base + "/${2}")

	// Fourth: inject Hugo generator meta tag (home page only)
	if opts.IsHome {
		html = injectGenerator(html)
	}

	// Finally: minify JSON-LD script contents (Hugo does this).
	html = minifyJSONLD(html)

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
