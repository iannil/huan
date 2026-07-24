package injector

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

// InjectHTML scans HTML <head>, checks existing tags, and injects missing ones.
// Returns modified HTML. If src has no <head>, returns src unchanged.
func InjectHTML(src string, opts *InjectOptions) (string, error) {
	if opts == nil {
		return src, nil
	}
	opts.setDefaults()

	// Parse existing tags
	existing := ExtractExistingTags(src)

	// Build missing tags
	var tags []string

	// description
	if _, has := existing["description"]; !has {
		desc := extractDescriptionFromHTML(src, opts.DescriptionMaxLength)
		if desc != "" {
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
			desc := extractDescriptionFromHTML(src, opts.DescriptionMaxLength)
			if desc != "" {
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
			desc := extractDescriptionFromHTML(src, opts.DescriptionMaxLength)
			if desc != "" {
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
	result := make(map[string]bool)
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return result
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "meta" {
			name := getAttr(n, "name")
			prop := getAttr(n, "property")
			if name != "" {
				result[name] = true
			}
			if prop != "" {
				result[prop] = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return result
}

// extractDescriptionFromHTML extracts plain text from <body> and truncates it.
func extractDescriptionFromHTML(htmlSrc string, maxLen int) string {
	bodyText := ExtractPlainText(htmlSrc)
	if bodyText == "" {
		return ""
	}
	return TruncateToWordBoundary(bodyText, maxLen)
}

// ExtractPlainText extracts all text content from <body> of an HTML document.
func ExtractPlainText(htmlSrc string) string {
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return ""
	}
	var buf strings.Builder
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n == nil {
			return
		}
		// Skip <style>, <script>, <nav>, <header>, <footer> content
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "style" || tag == "script" || tag == "nav" || tag == "header" || tag == "footer" {
				return
			}
		}
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
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

	// Find <body>
	var findBody func(*html.Node) *html.Node
	findBody = func(n *html.Node) *html.Node {
		if n == nil {
			return nil
		}
		if n.Type == html.ElementNode && n.Data == "body" {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := findBody(c); found != nil {
				return found
			}
		}
		return nil
	}

	body := findBody(doc)
	if body == nil {
		return ""
	}
	extract(body)
	return strings.TrimSpace(buf.String())
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