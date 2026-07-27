package main

import (
	"fmt"
	"html/template"
	"math"
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
