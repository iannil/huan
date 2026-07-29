package main

import (
	"fmt"
	"html"
	"strings"
)

// wrapSVG wraps rendered SVG in a semantic <figure>.
func wrapSVG(svg, lang, figureClass string) string {
	return fmt.Sprintf(`<figure class="%s %s-%s" role="img">%s</figure>`,
		figureClass, figureClass, lang, svg)
}

const mermaidInitMarker = "mermaid.initialize"

// fallbackReplacement computes the HTML that replaces a diagram block when Kroki
// rendering failed. Only mermaid can render client-side; other languages keep
// their original highlighted code block regardless of mode.
func fallbackReplacement(b diagramBlock, cfg *Config) (string, bool) {
	if cfg.FallbackMode == "client" && b.Lang == "mermaid" {
		return `<pre class="mermaid">` + html.EscapeString(b.Source) + `</pre>`, true
	}
	// codeblock, fail, or non-mermaid client: keep the original chroma block.
	return b.Full, false
}

// injectMermaidJS inserts the mermaid.js <script> and an initialize call once
// before </body>. It is a no-op if the init marker is already present or there
// is no </body>.
func injectMermaidJS(htmlSrc, mermaidJS string) string {
	if strings.Contains(htmlSrc, mermaidInitMarker) {
		return htmlSrc
	}
	idx := strings.LastIndex(htmlSrc, "</body>")
	if idx < 0 {
		return htmlSrc
	}
	snippet := fmt.Sprintf(
		"\n<script src=%q></script>\n<script>%s({startOnLoad:true});</script>\n",
		mermaidJS, "mermaid.initialize")
	return htmlSrc[:idx] + snippet + htmlSrc[idx:]
}
