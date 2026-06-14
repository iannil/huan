package qwen3

import (
	"fmt"
	"regexp"
	"strings"
)

// parsedOutput holds the title and body extracted from LLM output that
// follows the <title>...</title><body>...</body> contract.
type parsedOutput struct {
	Title string
	Body  string
}

// parseXMLOutput extracts <title> and <body> contents from the LLM response.
// Used for the FIRST chunk in a chunked translation (or for legacy non-chunked
// calls), where the prompt asks for both tags. <title> is REQUIRED.
//
// Tags may appear in either order; whitespace around tags is tolerated.
// Returns an error if either tag is missing or empty.
//
// Implementation note: regex is used (not encoding/xml) because LLM output
// is markdown and may contain entities/characters that confuse strict XML
// parsers. The regex is non-greedy and case-sensitive on tag name.
func parseXMLOutput(raw string) (parsedOutput, error) {
	titleRe := regexp.MustCompile(`(?s)<title>(.*?)</title>`)
	bodyRe := regexp.MustCompile(`(?s)<body>(.*?)</body>`)

	titleMatch := titleRe.FindStringSubmatch(raw)
	bodyMatch := bodyRe.FindStringSubmatch(raw)

	if len(titleMatch) < 2 {
		return parsedOutput{}, fmt.Errorf("parse: <title> tag not found in LLM output")
	}
	if len(bodyMatch) < 2 {
		return parsedOutput{}, fmt.Errorf("parse: <body> tag not found in LLM output")
	}

	title := strings.TrimSpace(titleMatch[1])
	body := strings.TrimSpace(bodyMatch[1])

	if title == "" {
		return parsedOutput{}, fmt.Errorf("parse: <title> tag is empty")
	}
	if body == "" {
		return parsedOutput{}, fmt.Errorf("parse: <body> tag is empty")
	}

	return parsedOutput{Title: title, Body: body}, nil
}

// parseChunkBodyOutput extracts only <body> from the LLM response. Used for
// NON-FIRST chunks in chunked translation, where the prompt asks only for
// <body>...</body> (no <title>). Title is irrelevant for these chunks.
//
// Returns an error if <body> is missing or empty.
func parseChunkBodyOutput(raw string) (parsedOutput, error) {
	bodyRe := regexp.MustCompile(`(?s)<body>(.*?)</body>`)
	bodyMatch := bodyRe.FindStringSubmatch(raw)
	if len(bodyMatch) < 2 {
		return parsedOutput{}, fmt.Errorf("parse: <body> tag not found in chunk output")
	}
	body := strings.TrimSpace(bodyMatch[1])
	if body == "" {
		return parsedOutput{}, fmt.Errorf("parse: <body> tag is empty")
	}
	return parsedOutput{Title: "", Body: body}, nil
}

