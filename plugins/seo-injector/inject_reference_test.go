// Frozen eager-analysis implementation for differential tests and benchmarks.
package main

import (
	"fmt"
	"golang.org/x/net/html"
	"os"
	"path/filepath"
	"strings"
)

// referenceHtmlAnalysis holds the result of a single html.Parse pass over a document:
// existing meta tag identifiers, the <title> text, and the plain text of
// <body>. Collecting all three from one parse replaces the previous three
// full parses per file (title + existing tags + plain text).
type referenceHtmlAnalysis struct {
	existing map[string]bool
	title    string
	bodyText string
}

// referenceAnalyzeHTML parses src once and collects meta tag identifiers (whole
// document), the first non-empty <title> text, and the plain text of the
// first <body> (skipping style/script/nav/header/footer subtrees) — matching
// what ExtractExistingTags, extractTitle and ExtractPlainText produced from
// separate parses.
func referenceAnalyzeHTML(src string) *referenceHtmlAnalysis {
	a := &referenceHtmlAnalysis{existing: make(map[string]bool)}
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return a
	}

	// Single walk: collect meta identifiers and the title.
	var walk func(*html.Node)
	var body *html.Node
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
				if body == nil {
					body = n
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if body != nil {
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
		extract(body)
		a.bodyText = strings.TrimSpace(buf.String())
	}
	return a
}

// referenceInjectHTML scans HTML <head>, checks existing tags, and injects missing ones.
// Returns modified HTML. If src has no <head>, returns src unchanged.
func referenceInjectHTML(src string, opts *InjectOptions) (string, error) {
	return referenceInjectWithAnalysis(src, opts, nil)
}

// referenceInjectHTML is referenceInjectHTML with an optional precomputed analysis (nil = parse).
func referenceInjectWithAnalysis(src string, opts *InjectOptions, a *referenceHtmlAnalysis) (string, error) {
	if opts == nil {
		return src, nil
	}
	opts.setDefaults()

	if a == nil {
		a = referenceAnalyzeHTML(src)
	}
	existing := a.existing

	// Plain text of <body>, extracted at most once per document.
	descText := ""
	descDone := false
	description := func() string {
		if !descDone {
			descDone = true
			if a.bodyText != "" {
				descText = TruncateToWordBoundary(a.bodyText, opts.DescriptionMaxLength)
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

func (p *SEOInjector) referenceProcessFile(filePath, outputDir string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	// Compute relative URL
	rel, err := filepath.Rel(outputDir, filePath)
	if err != nil {
		return fmt.Errorf("rel path: %w", err)
	}
	// Convert filesystem path to URL path. On Windows, use ToSlash.
	urlPath := "/" + strings.ReplaceAll(rel, string(filepath.Separator), "/")

	// One parse per file: meta tags, title and body text come from the same
	// analysis pass (previously three separate html.Parse calls).
	analysis := referenceAnalyzeHTML(string(data))

	opts := &InjectOptions{
		DescriptionMaxLength: p.cfg.DescriptionMaxLength,
		DefaultOGImage:       p.cfg.DefaultOGImage,
		InjectOG:             p.cfg.InjectOG,
		InjectTwitter:        p.cfg.InjectTwitter,
		PageURL:              urlPath, // relative to site root; caller should prepend baseURL if needed
		PageKind:             p.guessKind(rel),
		PageTitle:            analysis.title,
	}

	result, err := referenceInjectWithAnalysis(string(data), opts, analysis)
	if err != nil {
		return fmt.Errorf("inject: %w", err)
	}

	if result == string(data) {
		return nil // no changes
	}

	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
