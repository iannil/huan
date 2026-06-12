package equiv

import (
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
)

// SEOFields is the set of HTML fields that affect SEO.
type SEOFields struct {
	Title       string
	Description string
	OG          map[string]string // property -> content
	Twitter     map[string]string // name (twitter:*) -> content
	Canonical   string
	Robots      string
	Headings    []Heading
	JSONLD      []string // normalized (re-marshaled) JSON strings
	Links       []Link   // href + nofollow pairs
}

// Heading is a single h1-h6 entry.
type Heading struct {
	Level int
	Text  string
}

// Link captures href + nofollow status for SEO link graph comparison.
type Link struct {
	Href     string
	Nofollow bool
}

// ExtractSEO parses the HTML and returns normalized SEO fields.
func ExtractSEO(htmlSrc string) SEOFields {
	out := SEOFields{
		OG:      map[string]string{},
		Twitter: map[string]string{},
	}
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return out
	}
	walkSEO(doc, &out)
	// Normalize JSONLD by re-marshaling.
	normalized := make([]string, 0, len(out.JSONLD))
	for _, raw := range out.JSONLD {
		var anyVal interface{}
		if err := json.Unmarshal([]byte(raw), &anyVal); err == nil {
			b, _ := json.Marshal(anyVal)
			normalized = append(normalized, string(b))
		} else {
			normalized = append(normalized, raw)
		}
	}
	out.JSONLD = normalized
	return out
}

func walkSEO(n *html.Node, out *SEOFields) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode {
		switch strings.ToLower(n.Data) {
		case "title":
			out.Title = collapseWS(textOf(n))
		case "meta":
			extractMeta(n, out)
		case "link":
			if getAttr(n, "rel") == "canonical" {
				out.Canonical = getAttr(n, "href")
			}
		case "h1", "h2", "h3", "h4", "h5", "h6":
			lvl := map[string]int{"h1": 1, "h2": 2, "h3": 3, "h4": 4, "h5": 5, "h6": 6}[strings.ToLower(n.Data)]
			out.Headings = append(out.Headings, Heading{Level: lvl, Text: collapseWS(textOf(n))})
		case "script":
			if getAttr(n, "type") == "application/ld+json" {
				out.JSONLD = append(out.JSONLD, strings.TrimSpace(textOf(n)))
			}
		case "a":
			href := getAttr(n, "href")
			if href != "" {
				out.Links = append(out.Links, Link{Href: href, Nofollow: getAttr(n, "rel") == "nofollow"})
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkSEO(c, out)
	}
}

func extractMeta(n *html.Node, out *SEOFields) {
	name := getAttr(n, "name")
	prop := getAttr(n, "property")
	content := getAttr(n, "content")
	switch {
	case name == "description":
		out.Description = content
	case name == "robots":
		out.Robots = content
	case strings.HasPrefix(prop, "og:"):
		out.OG[prop] = content
	case strings.HasPrefix(name, "twitter:"):
		out.Twitter[name] = content
	}
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var buf strings.Builder
	var w func(*html.Node)
	w = func(node *html.Node) {
		if node.Type == html.TextNode {
			buf.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			w(c)
		}
	}
	w(n)
	return strings.TrimSpace(buf.String())
}

// Equal returns true if two SEOFields are field-by-field equivalent.
func (s SEOFields) Equal(o SEOFields) bool {
	if s.Title != o.Title || s.Description != o.Description ||
		s.Canonical != o.Canonical || s.Robots != o.Robots {
		return false
	}
	if !mapEqual(s.OG, o.OG) || !mapEqual(s.Twitter, o.Twitter) {
		return false
	}
	if len(s.Headings) != len(o.Headings) {
		return false
	}
	for i := range s.Headings {
		if s.Headings[i] != o.Headings[i] {
			return false
		}
	}
	if len(s.JSONLD) != len(o.JSONLD) {
		return false
	}
	for i := range s.JSONLD {
		if s.JSONLD[i] != o.JSONLD[i] {
			return false
		}
	}
	if len(s.Links) != len(o.Links) {
		return false
	}
	for i := range s.Links {
		if s.Links[i] != o.Links[i] {
			return false
		}
	}
	return true
}

func mapEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
