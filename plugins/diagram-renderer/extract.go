package main

import (
	"html"
	"regexp"
	"strings"
)

// diagramBlock is one chroma-highlighted fenced code block whose language is in
// the configured allowlist. Full is the entire "<div class=\"highlight\">…</div>"
// span (the replacement target); Source is the losslessly recovered raw source.
type diagramBlock struct {
	Full   string
	Lang   string
	Source string
}

// highlightBlockRe matches a chroma highlight wrapper and captures its data-lang
// and inner <code> content. The renderer emits:
//   <div class="highlight"><pre ...><code class="language-X" data-lang="X">…</code></pre></div>
var highlightBlockRe = regexp.MustCompile(
	`(?s)<div class="highlight">.*?<code[^>]*\sdata-lang="([^"]+)"[^>]*>(.*?)</code>.*?</div>`)

// spanTagRe matches any opening or closing <span> tag chroma inserts.
var spanTagRe = regexp.MustCompile(`</?span[^>]*>`)

// findDiagramBlocks returns every highlight block whose data-lang is in languages.
func findDiagramBlocks(htmlSrc string, languages []string) []diagramBlock {
	allow := make(map[string]bool, len(languages))
	for _, l := range languages {
		allow[strings.ToLower(l)] = true
	}
	var out []diagramBlock
	for _, m := range highlightBlockRe.FindAllStringSubmatch(htmlSrc, -1) {
		lang := strings.ToLower(m[1])
		if !allow[lang] {
			continue
		}
		out = append(out, diagramBlock{
			Full:   m[0],
			Lang:   lang,
			Source: extractSource(m[2]),
		})
	}
	return out
}

// extractSource recovers raw diagram source from a chroma <code> body by
// removing the <span> wrappers and unescaping HTML entities. Chroma tokenizes
// without adding or removing characters, so this round-trips losslessly.
func extractSource(codeInner string) string {
	s := spanTagRe.ReplaceAllString(codeInner, "")
	return html.UnescapeString(s)
}
