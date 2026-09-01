package main

import (
	"fmt"
	"html/template"
	"math"
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
