package equiv

import (
	"strings"

	"golang.org/x/net/html"
)

// AIFields is the set of HTML fields that affect LLM crawler friendliness.
type AIFields struct {
	MainText string          // innerText of <main>; pages without <main> yield empty (zhurongshuo templates use <main> so this is sufficient)
	Outline  []Heading       // h1-h6 in document order
	JSONLD   []string        // same content as SEOFields.JSONLD
	Semantic map[string]bool // which semantic elements are present
	NavLinks []string        // hrefs inside <nav>
}

// ExtractAI parses the HTML and returns AI-friendliness fields.
func ExtractAI(htmlSrc string) AIFields {
	out := AIFields{Semantic: map[string]bool{}}
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return out
	}
	walkAI(doc, &out)
	out.MainText = strings.TrimSpace(out.MainText)
	return out
}

func walkAI(n *html.Node, out *AIFields) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode {
		tag := strings.ToLower(n.Data)
		switch tag {
		case "header", "nav", "main", "article", "section", "footer", "aside":
			out.Semantic[tag] = true
		case "h1", "h2", "h3", "h4", "h5", "h6":
			lvl := map[string]int{"h1": 1, "h2": 2, "h3": 3, "h4": 4, "h5": 5, "h6": 6}[tag]
			out.Outline = append(out.Outline, Heading{Level: lvl, Text: textOf(n)})
		case "script":
			if getAttr(n, "type") == "application/ld+json" {
				out.JSONLD = append(out.JSONLD, strings.TrimSpace(textOf(n)))
			}
		}
		if tag == "nav" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				collectNavLinks(c, out)
			}
		}
	}
	if n.Type == html.ElementNode && strings.ToLower(n.Data) == "main" {
		out.MainText = collapseWS(textOf(n))
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkAI(c, out)
	}
}

func collectNavLinks(n *html.Node, out *AIFields) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode && strings.ToLower(n.Data) == "a" {
		if href := getAttr(n, "href"); href != "" {
			out.NavLinks = append(out.NavLinks, href)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectNavLinks(c, out)
	}
}

// collapseWS reduces any run of whitespace to a single space.
func collapseWS(s string) string {
	var buf strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				buf.WriteRune(' ')
			}
			prevSpace = true
		} else {
			buf.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(buf.String())
}

// Equal returns true if two AIFields are field-by-field equivalent.
func (a AIFields) Equal(o AIFields) bool {
	if a.MainText != o.MainText {
		return false
	}
	if len(a.Outline) != len(o.Outline) {
		return false
	}
	for i := range a.Outline {
		if a.Outline[i] != o.Outline[i] {
			return false
		}
	}
	if len(a.JSONLD) != len(o.JSONLD) {
		return false
	}
	for i := range a.JSONLD {
		if a.JSONLD[i] != o.JSONLD[i] {
			return false
		}
	}
	if len(a.Semantic) != len(o.Semantic) {
		return false
	}
	for k := range a.Semantic {
		if !o.Semantic[k] {
			return false
		}
	}
	if len(a.NavLinks) != len(o.NavLinks) {
		return false
	}
	for i := range a.NavLinks {
		if a.NavLinks[i] != o.NavLinks[i] {
			return false
		}
	}
	return true
}
