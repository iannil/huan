package qwen3

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan/internal/translate"
)

// helper: create a temp dir with a fake system prompt file
func setupPluginEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte("test system prompt"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	return dir
}

func TestParseConfig_Minimal(t *testing.T) {
	raw := map[string]any{
		"endpoint":            "http://localhost:11434",
		"model":               "qwen3-next:80b-a3b-instruct-q4_K_M",
		"system_prompt_file":  "prompt.md",
		"glossary_file":       "terms.yaml",
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Endpoint != "http://localhost:11434" {
		t.Errorf("endpoint = %q", cfg.Endpoint)
	}
	if cfg.Model != "qwen3-next:80b-a3b-instruct-q4_K_M" {
		t.Errorf("model = %q", cfg.Model)
	}
	// Defaults applied
	if cfg.TimeoutSeconds != 120 {
		t.Errorf("timeout default = %d, want 120", cfg.TimeoutSeconds)
	}
	if cfg.Concurrency != 1 {
		t.Errorf("concurrency default = %d, want 1", cfg.Concurrency)
	}
	if cfg.Quality.LengthRatioMin != 0.5 {
		t.Errorf("length_ratio_min default = %f, want 0.5", cfg.Quality.LengthRatioMin)
	}
}

func TestParseConfig_MissingRequired(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
	}{
		{"missing endpoint", map[string]any{
			"model": "x", "system_prompt_file": "p", "glossary_file": "g",
		}},
		{"missing model", map[string]any{
			"endpoint": "http://x", "system_prompt_file": "p", "glossary_file": "g",
		}},
		{"missing system_prompt_file", map[string]any{
			"endpoint": "http://x", "model": "m", "glossary_file": "g",
		}},
		{"missing glossary_file", map[string]any{
			"endpoint": "http://x", "model": "m", "system_prompt_file": "p",
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConfig(tc.raw)
			if err == nil {
				t.Error("expected error for missing required field")
			}
		})
	}
}

func TestNew_LoadsSystemPrompt(t *testing.T) {
	dir := setupPluginEnv(t)
	cfg := Config{
		Endpoint:         "http://localhost:11434",
		Model:            "qwen3:14b",
		SystemPromptFile: "prompt.md",
		GlossaryFile:     "terms.yaml",
	}
	p, err := New(cfg, dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "qwen3_translate" {
		t.Errorf("Name = %q, want %q", p.Name(), "qwen3_translate")
	}
	if p.prompt == nil {
		t.Error("prompt assembler not initialized")
	}
	if p.prompt.systemPrompt != "test system prompt" {
		t.Errorf("system prompt = %q", p.prompt.systemPrompt)
	}
}

func TestNew_MissingProjectRoot(t *testing.T) {
	cfg := Config{
		Endpoint:         "http://localhost:11434",
		Model:            "qwen3:14b",
		SystemPromptFile: "prompt.md",
		GlossaryFile:     "terms.yaml",
	}
	_, err := New(cfg, "")
	if err == nil {
		t.Error("expected error for empty projectRoot")
	}
}

func TestNew_MissingPromptFile(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Endpoint:         "http://localhost:11434",
		Model:            "qwen3:14b",
		SystemPromptFile: "nonexistent.md",
		GlossaryFile:     "terms.yaml",
	}
	_, err := New(cfg, dir)
	if err == nil {
		t.Error("expected error for missing prompt file")
	}
}

func TestNew_EmptyPromptFile(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(promptPath, []byte("   \n\n  "), 0644); err != nil {
		t.Fatalf("write empty prompt: %v", err)
	}
	cfg := Config{
		Endpoint:         "http://localhost:11434",
		Model:            "qwen3:14b",
		SystemPromptFile: "empty.md",
		GlossaryFile:     "terms.yaml",
	}
	_, err := New(cfg, dir)
	if err == nil {
		t.Error("expected error for whitespace-only prompt file")
	}
}

func TestTranslateResponseStatus_States(t *testing.T) {
	tests := []struct {
		name string
		resp *translate.Response
		want string
	}{
		{"nil", nil, "nil"},
		{"hard fail xml", &translate.Response{
			QualityChecks: translate.QualityResult{
				XMLParse:          false,
				LanguageDetection: true,
				MarkdownStructure: true,
			},
		}, "hard_fail"},
		{"soft warn length", &translate.Response{
			QualityChecks: translate.QualityResult{
				XMLParse:           true,
				LanguageDetection:  true,
				MarkdownStructure:  true,
				LengthRatio:        0.3,
				GlossaryCompliance: true,
			},
		}, "soft_warn_length"},
		{"soft warn glossary", &translate.Response{
			QualityChecks: translate.QualityResult{
				XMLParse:           true,
				LanguageDetection:  true,
				MarkdownStructure:  true,
				LengthRatio:        1.0,
				GlossaryCompliance: false,
			},
		}, "soft_warn_glossary"},
		{"success", &translate.Response{
			QualityChecks: translate.QualityResult{
				XMLParse:           true,
				LanguageDetection:  true,
				MarkdownStructure:  true,
				LengthRatio:        1.0,
				GlossaryCompliance: true,
			},
		}, "success"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := translateResponseStatus(tc.resp)
			if got != tc.want {
				t.Errorf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

// translateResponseStub is a test helper retained for future use.
// (Currently not used by TestTranslateResponseStatus_States, which builds
// translate.Response literals directly.)
type translateResponseStub struct {
	xmlParse     bool
	lengthRatio  float64
	glossaryOK   bool
}
