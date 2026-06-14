package qwen3

import (
	"strings"
	"testing"
)

func TestParseXMLOutput_BothTags(t *testing.T) {
	raw := `<title>Hello World</title>
<body>
This is the body.
</body>`
	out, err := parseXMLOutput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Title != "Hello World" {
		t.Errorf("title = %q, want %q", out.Title, "Hello World")
	}
	if out.Body != "This is the body." {
		t.Errorf("body = %q, want %q", out.Body, "This is the body.")
	}
}

func TestParseXMLOutput_BodyBeforeTitle(t *testing.T) {
	// Tags in reversed order should still parse.
	raw := `<body>Body first.</body>
<title>Title second.</title>`
	out, err := parseXMLOutput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Title != "Title second." {
		t.Errorf("title = %q", out.Title)
	}
	if out.Body != "Body first." {
		t.Errorf("body = %q", out.Body)
	}
}

func TestParseXMLOutput_MultilineBody(t *testing.T) {
	raw := `<title>Title</title>
<body>
## Heading

Paragraph with **bold**.

- Item 1
- Item 2
</body>`
	out, err := parseXMLOutput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Body, "## Heading") {
		t.Errorf("body lost markdown: %q", out.Body)
	}
	if !strings.Contains(out.Body, "- Item 1") {
		t.Errorf("body lost list: %q", out.Body)
	}
}

func TestParseXMLOutput_MissingTitle(t *testing.T) {
	raw := `<body>only body</body>`
	_, err := parseXMLOutput(raw)
	if err == nil {
		t.Error("expected error for missing <title>")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("error should mention title: %v", err)
	}
}

func TestParseXMLOutput_MissingBody(t *testing.T) {
	raw := `<title>only title</title>`
	_, err := parseXMLOutput(raw)
	if err == nil {
		t.Error("expected error for missing <body>")
	}
}

func TestParseXMLOutput_EmptyTitle(t *testing.T) {
	raw := `<title>   </title>
<body>body</body>`
	_, err := parseXMLOutput(raw)
	if err == nil {
		t.Error("expected error for whitespace-only title")
	}
}

func TestParseXMLOutput_EmptyBody(t *testing.T) {
	raw := `<title>title</title>
<body></body>`
	_, err := parseXMLOutput(raw)
	if err == nil {
		t.Error("expected error for empty body")
	}
}

func TestParseXMLOutput_BodyWithHTMLEntities(t *testing.T) {
	// LLM output may contain HTML entities from markdown (e.g. &amp; in code).
	// The regex (?s) non-greedy match should handle this.
	raw := `<title>Title with &amp; entity</title>
<body>
Code: ` + "`<a href=\"foo&amp;bar\">`" + `
</body>`
	out, err := parseXMLOutput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Body, "foo&amp;bar") {
		t.Errorf("body should preserve HTML entities: %q", out.Body)
	}
}

// promptAssembler tests for the format-contract suffix. Belongs in
// parse_test.go because the prompt package's other tests live close to
// the parser — but the suffix is in prompt.go.

func TestAssembleUserPrompt_FormatRulesPresent(t *testing.T) {
	a := &promptAssembler{systemPrompt: "stub"}
	req := translateRequest{
		Title:   "Test Title",
		Content: "Test body content.",
	}
	prompt := a.assembleUserPrompt(req, nil)

	// Hard contractual clauses — must always be present, regardless of
	// user system_prompt_file content.
	mustContain := []string{
		"CRITICAL FORMAT RULES",
		"raw markdown",
		"Do NOT use any HTML tags",
		"<h1>",
		"<p>",
		"<ul>",
		"<body>",
		"<title>",
		"Preserve ALL source markdown structure 1:1",
	}
	for _, s := range mustContain {
		if !strings.Contains(prompt, s) {
			t.Errorf("user prompt missing required clause %q", s)
		}
	}
}

func TestAssembleUserPrompt_SourceBodyIncluded(t *testing.T) {
	a := &promptAssembler{systemPrompt: "stub"}
	req := translateRequest{
		Title:   "My Title",
		Content: "UNIQUE_MARKER_42 in body",
	}
	prompt := a.assembleUserPrompt(req, nil)
	if !strings.Contains(prompt, "My Title") {
		t.Error("user prompt should include source title")
	}
	if !strings.Contains(prompt, "UNIQUE_MARKER_42 in body") {
		t.Error("user prompt should include source body verbatim")
	}
}
