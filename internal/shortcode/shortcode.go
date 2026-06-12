// Package shortcode provides Hugo-compatible shortcode registration and expansion.
package shortcode

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/iannil/huan/internal/content"
)

// Context is what a shortcode handler receives.
type Context struct {
	Params map[string]string
	Inner  string // content between {{< x >}}...{{< /x >}}
	Args   []string
	Page   *content.Page
	Site   *content.Site
}

// Handler is a function that renders a shortcode.
type Handler func(ctx *Context) (string, error)

// Registry holds all registered shortcode handlers.
type Registry struct {
	handlers map[string]Handler
}

// NewRegistry creates a new shortcode registry with built-in shortcodes registered.
func NewRegistry() *Registry {
	r := &Registry{handlers: map[string]Handler{}}
	r.registerBuiltins()
	return r
}

// Register adds or replaces a shortcode handler.
func (r *Registry) Register(name string, handler Handler) {
	r.handlers[name] = handler
}

// Has reports whether a shortcode is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.handlers[name]
	return ok
}

// registerBuiltins registers the built-in shortcodes used by the site.
func (r *Registry) registerBuiltins() {
	r.Register("redact", RedactHandler)
	r.Register("audio", AudioHandler)
	r.Register("img", ImgHandler)
}

// shortcodeRe matches Hugo shortcode invocations:
//   {{< name args >}}           - inline, no body
//   {{< name args >}}...{{< /name >}}  - block, with body
//   {{% name args %}}           - inline, processed as markdown
var shortcodeOpenRe = regexp.MustCompile(`\{\{[<%]\s*(\w+)([^>/%]*?)[>/%]\s*\}\}`)
var shortcodeCloseRe = regexp.MustCompile(`\{\{[<%]\s*/(\w+)\s*[>/%]\s*\}\}`)

// Expand processes all shortcodes in the given markdown body.
// It handles both inline (self-closing) and block (with inner content) forms.
// Unknown shortcodes are left in place untouched.
func (r *Registry) Expand(body string, page *content.Page, site *content.Site) (string, error) {
	if !strings.Contains(body, "{{") {
		return body, nil
	}

	// Process block shortcodes first (those with matching close tags)
	result, err := r.expandBlock(body, page, site)
	if err != nil {
		return "", err
	}

	// Then process remaining inline shortcodes
	result, err = r.expandInline(result, page, site)
	if err != nil {
		return "", err
	}

	return result, nil
}

// expandBlock handles shortcodes with open/close tags: {{< name >}}...{{< /name >}}
func (r *Registry) expandBlock(body string, page *content.Page, site *content.Site) (string, error) {
	// Loop until no more block shortcodes are found
	for {
		loc := shortcodeOpenRe.FindStringSubmatchIndex(body)
		if loc == nil {
			break
		}

		name := body[loc[2]:loc[3]]

		// Find matching close tag
		closePattern := fmt.Sprintf(`\{\{[<%%]\s*/%s\s*[>/%%]\s*\}\}`, regexp.QuoteMeta(name))
		closeRe, err := regexp.Compile(closePattern)
		if err != nil {
			return "", err
		}

		closeLoc := closeRe.FindStringIndex(body[loc[1]:])
		if closeLoc == nil {
			// No close tag, treat as inline - skip
			break
		}

		openStart := loc[0]
		openEnd := loc[1]
		closeStart := loc[1] + closeLoc[0]
		closeEnd := loc[1] + closeLoc[1]

		inner := body[openEnd:closeStart]

		// Parse args from open tag
		argsStr := strings.TrimSpace(body[loc[4]:loc[5]])
		params := parseParams(argsStr)

		handler, ok := r.handlers[name]
		if !ok {
			// Unknown shortcode, leave it
			break
		}

		output, err := handler(&Context{
			Params: params,
			Inner:  inner,
			Page:   page,
			Site:   site,
		})
		if err != nil {
			return "", fmt.Errorf("shortcode %s: %w", name, err)
		}

		body = body[:openStart] + output + body[closeEnd:]
	}

	return body, nil
}

// expandInline handles self-closing shortcodes: {{< name args >}}
func (r *Registry) expandInline(body string, page *content.Page, site *content.Site) (string, error) {
	for {
		loc := shortcodeOpenRe.FindStringSubmatchIndex(body)
		if loc == nil {
			break
		}

		name := body[loc[2]:loc[3]]

		// Skip if this is actually a close tag (caught by regex)
		if strings.HasPrefix(body[loc[2]:loc[3]], "/") {
			break
		}

		openStart := loc[0]
		openEnd := loc[1]

		handler, ok := r.handlers[name]
		if !ok {
			// Unknown shortcode, leave it untouched and break
			break
		}

		argsStr := strings.TrimSpace(body[loc[4]:loc[5]])
		params := parseParams(argsStr)

		output, err := handler(&Context{
			Params: params,
			Page:   page,
			Site:   site,
		})
		if err != nil {
			return "", fmt.Errorf("shortcode %s: %w", name, err)
		}

		body = body[:openStart] + output + body[openEnd:]
	}

	return body, nil
}

// parseParams parses shortcode arguments into a key=value map.
// Supports both named (key="value") and positional args.
// Positional args are stored under keys "0", "1", etc.
func parseParams(s string) map[string]string {
	if s == "" {
		return map[string]string{}
	}

	params := map[string]string{}
	pos := 0

	// Tokenize, respecting quotes
	tokens := tokenizeArgs(s)
	for _, tok := range tokens {
		if idx := strings.Index(tok, "="); idx >= 0 {
			key := strings.TrimSpace(tok[:idx])
			val := strings.Trim(strings.TrimSpace(tok[idx+1:]), `"'`)
			params[key] = val
		} else {
			params[fmt.Sprintf("%d", pos)] = strings.Trim(tok, `"'`)
			pos++
		}
	}

	return params
}

// tokenizeArgs splits an args string into key="value" or positional tokens,
// respecting double-quoted values.
func tokenizeArgs(s string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			inQuote = !inQuote
			current.WriteByte(c)
		case ' ', '\t':
			if inQuote {
				current.WriteByte(c)
			} else if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}
