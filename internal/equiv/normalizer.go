// Package equiv provides HTML/SEO/AI field comparison utilities
// for verifying huan output equivalence against Hugo.
package equiv

import (
	"bytes"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// NormalizeHTML returns a canonical form of the input HTML, suitable
// for byte-level comparison of pages that should render identically.
//
// Normalizations applied:
//   - Implicit html/head/body wrappers injected by the parser are stripped,
//     so fragment-in == fragment-out and full-document-in == full-document-in.
//   - Whitespace between tags is removed (<div>\n  <p> → <div><p>)
//   - Self-closing tags are canonicalized (<br/> and <br /> → <br/>)
//   - Attributes are sorted by name with double-quoted values
//   - Boolean attributes are normalized
//   - HTML entities are decoded then re-encoded as named entities where possible
func NormalizeHTML(in string) string {
	doc, err := html.Parse(strings.NewReader(in))
	if err != nil {
		return in
	}
	var buf bytes.Buffer
	crawlNormalize(doc, &buf)
	return buf.String()
}

// isImplicitWrapper reports whether the element is one of the structural
// tags the HTML parser inserts automatically. These are stripped during
// normalization so that comparing two semantically-equivalent documents
// (one as a fragment, one as a full document) yields identical output.
func isImplicitWrapper(name string) bool {
	switch strings.ToLower(name) {
	case "html", "head", "body":
		return true
	}
	return false
}

func crawlNormalize(n *html.Node, buf *bytes.Buffer) {
	if n == nil {
		return
	}
	switch n.Type {
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			crawlNormalize(c, buf)
		}
		return
	case html.ElementNode:
		if isImplicitWrapper(n.Data) {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				crawlNormalize(c, buf)
			}
			return
		}
		buf.WriteString("<")
		buf.WriteString(n.Data)
		attrs := append([]html.Attribute(nil), n.Attr...)
		sort.Slice(attrs, func(i, j int) bool { return attrs[i].Key < attrs[j].Key })
		for _, a := range attrs {
			buf.WriteString(" ")
			buf.WriteString(a.Key)
			if a.Val != "" {
				buf.WriteString(`="`)
				buf.WriteString(html.EscapeString(a.Val))
				buf.WriteString(`"`)
			}
		}
		if isVoidElement(n.Data) {
			buf.WriteString("/>")
		} else {
			buf.WriteString(">")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				crawlNormalize(c, buf)
			}
			buf.WriteString("</")
			buf.WriteString(n.Data)
			buf.WriteString(">")
		}
		return
	case html.TextNode:
		folded := strings.TrimSpace(n.Data)
		if folded != "" {
			buf.WriteString(html.EscapeString(folded))
		}
		return
	case html.CommentNode:
		return
	}
}

func isVoidElement(name string) bool {
	switch strings.ToLower(name) {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}
