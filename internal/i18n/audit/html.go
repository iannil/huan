// Package audit provides pure logic for the i18n page audit: parsing rendered
// HTML, checking per-page language correctness, and computing zh/en parity.
// All functions are I/O-free; the CLI (cmd/huan/translate_audit.go) handles
// HTTP fetching, filesystem access, and report writing.
package audit

import (
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Page holds the text extracted from a rendered page that the audit cares
// about: the document language, the article title, and the main-content prose
// (with code, scripts, and styles removed).
type Page struct {
	Lang  string // <html lang="..."> value, lowercased (e.g. "zh-cn", "en")
	Title string // text of the .post_title element
	Prose string // text of .post_content, excluding <pre>/<code>/<script>/<style>
}

// Parse reads rendered HTML and extracts the audit-relevant text. It locates
// the article body via the theme's `post_content` class and the title via
// `post_title` (see layouts/_default/single.html). Prose excludes code spans
// and blocks so CJK inside code examples is not mistaken for untranslated
// prose — mirroring langdetect.CJKRunesOutsideCode for markdown source.
func Parse(r io.Reader) (Page, error) {
	root, err := html.Parse(r)
	if err != nil {
		return Page{}, err
	}
	var p Page
	p.Lang = strings.ToLower(strings.TrimSpace(findHTMLLang(root)))
	if node := findByClass(root, "post_title"); node != nil {
		p.Title = strings.TrimSpace(collectProse(node))
	}
	if node := findByClass(root, "post_content"); node != nil {
		p.Prose = strings.TrimSpace(collectProse(node))
	}
	return p, nil
}

// findHTMLLang returns the lang attribute of the <html> element, or "".
func findHTMLLang(n *html.Node) string {
	if n.Type == html.ElementNode && n.DataAtom == atom.Html {
		for _, a := range n.Attr {
			if a.Key == "lang" {
				return a.Val
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if v := findHTMLLang(c); v != "" {
			return v
		}
	}
	return ""
}

// findByClass returns the first element whose class attribute contains the
// given whitespace-delimited token (e.g. "post_content" matches
// class="post_content markdown").
func findByClass(n *html.Node, token string) *html.Node {
	if n.Type == html.ElementNode {
		for _, a := range n.Attr {
			if a.Key == "class" && hasClass(a.Val, token) {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByClass(c, token); found != nil {
			return found
		}
	}
	return nil
}

func hasClass(classAttr, token string) bool {
	for _, f := range strings.Fields(classAttr) {
		if f == token {
			return true
		}
	}
	return false
}

// collectProse returns the concatenated text content of n, skipping
// <script>, <style>, <pre>, and <code> subtrees (code is not prose).
func collectProse(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.DataAtom {
			case atom.Script, atom.Style, atom.Pre, atom.Code:
				return
			}
		}
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
