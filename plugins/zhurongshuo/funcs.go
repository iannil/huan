package main

import (
	"fmt"
	"html/template"
	"math"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// readingTime estimates reading time for mixed CJK + English content.
// Chinese characters are counted at 300 chars/minute and English words
// at 200 words/minute. Returns a human-friendly string like "5 分钟".
func readingTime(content string) template.HTML {
	cjkCount := 0
	wordCount := 0
	inWord := false
	for _, r := range content {
		if r >= 0x4E00 && r <= 0x9FFF {
			cjkCount++
			inWord = false
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			if !inWord {
				wordCount++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	minutes := math.Ceil(float64(cjkCount)/300 + float64(wordCount)/200)
	if minutes < 1 {
		minutes = 1
	}
	return template.HTML(fmt.Sprintf("%d 分钟", int(minutes)))
}

// relatedPosts returns related posts by tag (placeholder).
// Actual relevance logic is handled by huan's built-in rendering pipeline.
func relatedPosts() string { return "" }

// toc generates a table of contents from page content (placeholder).
// Actual TOC generation is handled by huan's built-in rendering pipeline.
func toc() string { return "" }

// darkModeToggle returns the HTML for a dark mode toggle button.
func darkModeToggle() template.HTML {
	return template.HTML(`<button id="dark-mode-toggle" aria-label="切换深色模式">🌓</button>`)
}

// parseGuideYAML extracts the first ```guide fenced code block from a
// page's plain text and parses it as YAML. Guide handbook pages embed
// their structured content (thesis, main_chart, concepts, map,
// takeaways) this way; the layout template calls this function with
// .Plain. A missing block or malformed YAML is a hard error so the
// build log surfaces it (render errors count into Result.Errors).
func parseGuideYAML(plain string) (map[string]interface{}, error) {
	const fence = "```guide"
	start := strings.Index(plain, fence)
	if start < 0 {
		return nil, fmt.Errorf("guide page has no ```guide code block")
	}
	rest := plain[start+len(fence):]
	end := strings.Index(rest, "```")
	if end < 0 {
		return nil, fmt.Errorf("guide block not closed with ```")
	}
	block := rest[:end]
	// plainify may have HTML-escaped or joined lines; yaml handles \n.
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(block), &data); err != nil {
		return nil, fmt.Errorf("guide block YAML: %w", err)
	}
	return data, nil
}

// svgTextWidth estimates rendered text width. CJK chars and full-width
// punctuation measure 1.0×fontSize; other (ASCII/Latin) chars ≈0.55×.
// Estimation, not measurement — budgets in callers leave slack.
func svgTextWidth(s string, fontSize int) float64 {
	w := 0.0
	for _, r := range s {
		if isWideRune(r) {
			w += float64(fontSize)
		} else {
			w += float64(fontSize) * 0.55
		}
	}
	return w
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK punctuation
		return true
	case r >= 0xFF00 && r <= 0xFFEF: // fullwidth forms
		return true
	case r == 0x2014 || r == 0x2026: // em dash, ellipsis
		return true
	}
	return false
}

// svgTruncate returns s unchanged when it fits maxWidth; otherwise cuts
// it at the character boundary that fits and appends "…".
func svgTruncate(s string, maxWidth float64, fontSize int) string {
	if svgTextWidth(s, fontSize) <= maxWidth {
		return s
	}
	runes := []rune(s)
	ellipsisW := svgTextWidth("…", fontSize)
	acc := 0.0
	for i, r := range runes {
		rw := svgTextWidth(string(r), fontSize)
		if acc+rw+ellipsisW > maxWidth {
			return string(runes[:i]) + "…"
		}
		acc += rw
	}
	return s
}

// svgWrap breaks s into lines each fitting maxWidth (CJK char-by-char,
// ASCII on spaces). The last line may keep a long unbreakable word.
func svgWrap(s string, maxWidth float64, fontSize int) []string {
	var lines []string
	var cur []rune
	curW := 0.0
	flush := func() {
		if len(cur) > 0 {
			lines = append(lines, string(cur))
			cur = nil
			curW = 0.0
		}
	}
	for _, r := range s {
		rw := svgTextWidth(string(r), fontSize)
		if r == ' ' && curW == 0 {
			continue // skip leading spaces
		}
		if curW+rw > maxWidth {
			flush()
		}
		cur = append(cur, r)
		curW += rw
	}
	flush()
	return lines
}

// failRender lets guide templates convert content-authoring mistakes
// (unknown book slug, unsupported chart_type, missing fields) into render
// errors. huan counts render errors into Result.Errors and logs a WARN
// per page — surfaced in the build summary "Errors:" line (pipeline_render.go).
func failRender(msg string) (string, error) {
	return "", fmt.Errorf("guide: %s", msg)
}

// guideChartTypes lists chart partials supported by the guide layout.
// Templates validate main_chart.chart_type against this set before
// dispatching to charts/<type>.html.
var guideChartTypes = map[string]bool{
	"funnel": true, "ladder": true, "cycle": true, "layers": true,
	"flow": true, "network": true, "spectrum": true,
}

// guideChartTypesFn exposes the supported chart types to templates as a
// sorted slice usable with `in`: {{ if not (in (guideChartTypes) $ct) }}...
// (huan's indexFunc does not look up map[string]bool, so a plain slice
// composes better with the template builtin set.)
func guideChartTypesFn() []string {
	types := make([]string, 0, len(guideChartTypes))
	for k := range guideChartTypes {
		types = append(types, k)
	}
	sort.Strings(types)
	return types
}
