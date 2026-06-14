package qwen3

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// promptAssembler constructs the system + user prompts for a single
// Translate call. The system prompt is loaded once from cfg.SystemPromptFile;
// the user prompt is assembled per call with GLOSSARY block + SOURCE_TITLE +
// SOURCE_BODY.
type promptAssembler struct {
	systemPrompt string
}

// newPromptAssembler loads the system prompt from the configured file path
// (relative to projectRoot). Returns an error if the file cannot be read
// or is empty.
func newPromptAssembler(projectRoot, systemPromptFile string) (*promptAssembler, error) {
	full := systemPromptFile
	if !filepath.IsAbs(full) {
		full = filepath.Join(projectRoot, systemPromptFile)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("read system prompt %s: %w", full, err)
	}
	sp := strings.TrimSpace(string(data))
	if sp == "" {
		return nil, fmt.Errorf("system prompt %s is empty", full)
	}
	return &promptAssembler{systemPrompt: sp}, nil
}

// assembleUserPrompt builds the user message sent to the LLM.
//
// Format:
//
//	GLOSSARY:
//	srcTerm1 → tgtTerm1
//	srcTerm2 → tgtTerm2
//
//	SOURCE_TITLE: <title>
//
//	SOURCE_BODY:
//	<body>
//
//	Translate now. Output ONLY <title>...</title><body>...</body>.
func (a *promptAssembler) assembleUserPrompt(req translateRequest, glossary map[string]string) string {
	var b strings.Builder

	// GLOSSARY block (stable order for deterministic prompts)
	if len(glossary) > 0 {
		b.WriteString("GLOSSARY:\n")
		keys := make([]string, 0, len(glossary))
		for k := range glossary {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(k)
			b.WriteString(" → ")
			b.WriteString(glossary[k])
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Hints (user-supplied prompt additions)
	for _, hint := range req.Hints {
		b.WriteString("HINT: ")
		b.WriteString(hint)
		b.WriteString("\n")
	}
	if len(req.Hints) > 0 {
		b.WriteString("\n")
	}

	// Title
	b.WriteString("SOURCE_TITLE: ")
	b.WriteString(req.Title)
	b.WriteString("\n\n")

	// Body
	b.WriteString("SOURCE_BODY:\n")
	b.WriteString(req.Content)
	b.WriteString("\n\n")

	b.WriteString("Translate now. Output ONLY <title>...</title><body>...</body>.")

	return b.String()
}

// translateRequest is a local alias to avoid importing internal/translate
// (would create circular: translate → qwen3 → translate).
// The plugin.go layer converts translate.Request to this local type.
type translateRequest struct {
	Title    string
	Content  string
	Hints    []string
}
