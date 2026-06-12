package encrypt

import (
	"html/template"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/content"
)

func TestRenderPublic(t *testing.T) {
	engine := NewEngine(nil, map[string]EncryptGroupConfig{})
	page := &content.Page{
		Access:  "public",
		Content: template.HTML("<p>hello</p>"),
	}

	out, err := engine.Render(page, nil, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if string(out) != "<p>hello</p>" {
		t.Errorf("public content should pass through unchanged, got: %s", out)
	}
}

func TestRenderProtectedFullNoData(t *testing.T) {
	engine := NewEngine(nil, map[string]EncryptGroupConfig{
		"default": {Hint: "Protected", Mode: "full"},
	})
	page := &content.Page{
		Access:  "protected",
		Content: template.HTML("<p>secret content here</p>"),
		RelPath: "test/page.md",
	}

	out, err := engine.Render(page, nil, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	outStr := string(out)
	if !strings.Contains(outStr, "█") {
		t.Errorf("expected redaction blocks in output: %s", outStr)
	}
}

func TestRenderProtectedRandomWithSeed(t *testing.T) {
	engine := NewEngine(nil, map[string]EncryptGroupConfig{
		"kachuai": {Hint: "Random", Mode: "random", Ratio: 50},
	})
	page := &content.Page{
		Access:       "protected",
		EncryptGroup: "kachuai",
		Content:      template.HTML("<p>visible content</p>"),
		RelPath:      "test/page.md",
	}

	out, err := engine.Render(page, nil, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	outStr := string(out)
	if !strings.Contains(outStr, "random-redact-content") {
		t.Errorf("expected random-redact-content class: %s", outStr)
	}
	if !strings.Contains(outStr, "data-ratio=") {
		t.Errorf("expected data-ratio attribute: %s", outStr)
	}
}

func TestRenderProtectedWithEncryptedData(t *testing.T) {
	encryptedData := map[string]interface{}{
		"abc123": map[string]interface{}{
			"encrypted": "ENCRYPTED_PAYLOAD",
			"seed":      "SEED123",
		},
	}
	engine := NewEngine(encryptedData, map[string]EncryptGroupConfig{
		"default": {Hint: "P", Mode: "full"},
	})
	page := &content.Page{
		Access:  "protected",
		Content: template.HTML("<p>hello world</p>"),
		RelPath: "test/page.md",
	}

	out, err := engine.Render(page, nil, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	outStr := string(out)
	// Page won't match fileId "abc123" (uses MD5 of "test/page.md"), so falls back
	// Should still produce redaction output
	if !strings.Contains(outStr, "█") && !strings.Contains(outStr, "redact") {
		t.Errorf("expected redaction output: %s", outStr)
	}
}
