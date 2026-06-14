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
//
// The contract is:
//
//	<title>English Title</title>
//	<body>
//	English markdown body
//	</body>
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
