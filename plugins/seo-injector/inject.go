package main

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// InjectOptions carries parameters for a single HTML injection.
type InjectOptions struct {
	DescriptionMaxLength int
	DefaultOGImage       string
	InjectOG             bool
	InjectTwitter        bool
	PageURL              string // absolute URL of this page
	PageKind             string // "page" | "section" | "home" | "taxonomy" | "term"
	PageTitle            string // page title (already known)
}

// setDefaults fills zero-valued fields with defaults.
func (o *InjectOptions) setDefaults() {
	if o.DescriptionMaxLength <= 0 {
		o.DescriptionMaxLength = 160
	}
	// InjectOG and InjectTwitter default to true
	if !o.InjectOG {
		o.InjectOG = true
	}
	if !o.InjectTwitter {
		o.InjectTwitter = true
	}
}

// htmlAnalysis holds the result of a single html.Parse pass over a document:
// existing meta tag identifiers, title text, and the first body node. Body
// text is extracted only when missing descriptions actually need it. The
// analysis is local to one document; no state is retained between builds.
type htmlAnalysis struct {
	existing map[string]bool
	title    string
	body     *html.Node
}

// analyzeHTML parses src once and collects meta tag identifiers (whole
// document), the first non-empty <title> text, and the first <body> node.
// Keeping html.Parse preserves its correction of malformed HTML and the
// existing whole-document meta-tag lookup semantics.
func analyzeHTML(src string) *htmlAnalysis {
	a := &htmlAnalysis{existing: make(map[string]bool)}
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return a
	}

	// Single walk: collect meta identifiers and the title.
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "meta":
				if name := getAttr(n, "name"); name != "" {
					a.existing[name] = true
				}
				if prop := getAttr(n, "property"); prop != "" {
					a.existing[prop] = true
				}
			case "title":
				if a.title == "" && n.FirstChild != nil {
					a.title = strings.TrimSpace(n.FirstChild.Data)
				}
			case "body":
				if a.body == nil {
					a.body = n
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return a
}

// plainText follows the original DOM walk, including ignored subtrees and
// trimmed text-node boundaries. Callers requesting descriptions invoke it once.
func (a *htmlAnalysis) plainText() string {
	if a.body == nil {
		return ""
	}
	var buf strings.Builder
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "style", "script", "nav", "header", "footer":
				return
			}
		}
		if n.Type == html.TextNode {
			if text := strings.TrimSpace(n.Data); text != "" {
				if buf.Len() > 0 {
					buf.WriteString(" ")
				}
				buf.WriteString(text)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(a.body)
	return strings.TrimSpace(buf.String())
}

// InjectHTML scans HTML <head>, checks existing tags, and injects missing ones.
// Returns modified HTML. If src has no <head>, returns src unchanged.
func InjectHTML(src string, opts *InjectOptions) (string, error) {
	return injectHTML(src, opts, nil)
}

// injectHTML is InjectHTML with an optional precomputed analysis (nil = parse).
func injectHTML(src string, opts *InjectOptions, a *htmlAnalysis) (string, error) {
	if opts == nil {
		return src, nil
	}
	opts.setDefaults()

	if a == nil {
		a = analyzeHTML(src)
	}
	existing := a.existing

	// Plain text of <body>, extracted at most once per document.
	descText := ""
	descDone := false
	description := func() string {
		if !descDone {
			descDone = true
			if bodyText := a.plainText(); bodyText != "" {
				descText = TruncateToWordBoundary(bodyText, opts.DescriptionMaxLength)
			}
		}
		return descText
	}

	// Build missing tags
	var tags []string

	// description
	if _, has := existing["description"]; !has {
		if desc := description(); desc != "" {
			tags = append(tags, fmt.Sprintf(`<meta name="description" content="%s">`, html.EscapeString(desc)))
		}
	}

	if opts.InjectOG {
		// og:title
		if _, has := existing["og:title"]; !has && opts.PageTitle != "" {
			tags = append(tags, fmt.Sprintf(`<meta property="og:title" content="%s">`, html.EscapeString(opts.PageTitle)))
		}

		// og:description
		if _, has := existing["og:description"]; !has {
			if desc := description(); desc != "" {
				tags = append(tags, fmt.Sprintf(`<meta property="og:description" content="%s">`, html.EscapeString(desc)))
			}
		}

		// og:url
		if _, has := existing["og:url"]; !has && opts.PageURL != "" {
			tags = append(tags, fmt.Sprintf(`<meta property="og:url" content="%s">`, html.EscapeString(opts.PageURL)))
		}

		// og:type
		if _, has := existing["og:type"]; !has {
			ogType := "website"
			if opts.PageKind == "page" {
				ogType = "article"
			}
			tags = append(tags, fmt.Sprintf(`<meta property="og:type" content="%s">`, ogType))
		}

		// og:image
		if _, has := existing["og:image"]; !has && opts.DefaultOGImage != "" {
			tags = append(tags, fmt.Sprintf(`<meta property="og:image" content="%s">`, html.EscapeString(opts.DefaultOGImage)))
		}
	}

	if opts.InjectTwitter {
		// twitter:card
		if _, has := existing["twitter:card"]; !has {
			tags = append(tags, `<meta name="twitter:card" content="summary_large_image">`)
		}

		// twitter:title
		if _, has := existing["twitter:title"]; !has && opts.PageTitle != "" {
			tags = append(tags, fmt.Sprintf(`<meta name="twitter:title" content="%s">`, html.EscapeString(opts.PageTitle)))
		}

		// twitter:description
		if _, has := existing["twitter:description"]; !has {
			if desc := description(); desc != "" {
				tags = append(tags, fmt.Sprintf(`<meta name="twitter:description" content="%s">`, html.EscapeString(desc)))
			}
		}
	}

	if len(tags) == 0 {
		return src, nil
	}

	// Inject before </head>
	headClose := strings.Index(src, "</head>")
	if headClose < 0 {
		return src, nil
	}

	comment := "\n<!-- huan seo-injector -->\n"
	injection := comment + strings.Join(tags, "\n") + "\n"
	return src[:headClose] + injection + src[headClose:], nil
}

// ExtractExistingTags returns a set of already-present meta tag identifiers.
// Key for name-based: name attribute value. Key for property-based: property attribute value.
func ExtractExistingTags(htmlSrc string) map[string]bool {
	return analyzeHTML(htmlSrc).existing
}

// ExtractPlainText extracts all text content from <body> of an HTML document.
func ExtractPlainText(htmlSrc string) string {
	return analyzeHTML(htmlSrc).plainText()
}

// TruncateToWordBoundary truncates text to maxLen characters at the last word boundary.
func TruncateToWordBoundary(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	// Find last space before maxLen
	trimmed := text[:maxLen]
	if idx := strings.LastIndex(trimmed, " "); idx > 0 {
		return text[:idx]
	}
	// No word boundary found — only truncate if the trimmed portion is all one word.
	// Check if the rest after maxLen has a space too; if not, return original.
	if strings.Contains(text[maxLen:], " ") {
		return trimmed
	}
	return text
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
