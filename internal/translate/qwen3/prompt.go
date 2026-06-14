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

	// Format contract — huan-enforced. Layered on top of the user's
	// system_prompt_file (which owns translation style/tone). This suffix
	// is the plugin's output contract: the .en.md sidecar MUST be raw
	// markdown.
	//
	// Why hardcoded here (not in system_prompt_file): Qwen3-Next-80B has
	// a strong prior to convert markdown to HTML on long zh→en inputs.
	// Empirically, "preserve markdown structure" alone is insufficient;
	// explicit prohibition + enumerated markdown equivalents is required.
	// This is huan's contract, not a user preference — applies regardless
	// of system_prompt_file content.
	b.WriteString("Translate now. Output ONLY raw markdown wrapped in <title>...</title><body>...</body>.\n\n")
	b.WriteString("CRITICAL FORMAT RULES (output will be REJECTED if violated):\n")
	b.WriteString("- The <body> MUST be raw markdown. Do NOT use any HTML tags.\n")
	b.WriteString("- Forbidden HTML tags: <p>, <h1>..<h6>, <ul>, <ol>, <li>, <pre>, <blockquote>, <table>, <tr>, <td>, <th>, <div>, <section>.\n")
	b.WriteString("- Headings: use # / ## / ### (same levels as source).\n")
	b.WriteString("- Paragraphs: separate with blank lines (NO <p> tags).\n")
	b.WriteString("- Lists: use - or * or + (same marker style as source).\n")
	b.WriteString("- Links: [text](url). Images: ![alt](url).\n")
	b.WriteString("- Code blocks: triple-backtick fences with the same language tag as source.\n")
	b.WriteString("- Preserve ALL source markdown structure 1:1 (no merging, splitting, or omitting sections).\n")

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

// chunkPromptInput carries everything assembleChunkPrompt needs to build
// a per-chunk user message.
type chunkPromptInput struct {
	// Title is the source title. Sent on every chunk to keep the model
	// grounded in the document's overall subject.
	Title string

	// Hints are user-supplied style hints. Sent on every chunk.
	Hints []string

	// ChunkHeading is the level-2 heading line that opens this chunk
	// (e.g., "## 4.1 主客同源"). Empty for the preamble chunk.
	ChunkHeading string

	// ChunkBody is the markdown content of this chunk EXCLUDING the
	// heading line. Always non-empty (caller skips empty chunks).
	ChunkBody string

	// PreviousContext is the sliding-window-injected previously
	// translated content. Empty for the first chunk. Used to give the
	// model narrative flow without letting it restructure across
	// sections.
	PreviousContext string

	// IsFirst is true for the first chunk (preamble or first section).
	// First chunk's prompt asks for <title>...</title><body>...</body>;
	// subsequent chunks ask for only <body>...</body>.
	IsFirst bool
}

// assembleChunkPrompt builds the user message for a single chunk's
// translation. Structure:
//
//	GLOSSARY: ...
//	HINT: ...
//	SOURCE_TITLE: <title>
//
//	PREVIOUSLY_TRANSLATED_SECTIONS (context only, do NOT re-translate):
//	<sliding window context, or "(none)" for first chunk)>
//
//	---
//
//	TRANSLATE_NOW (translate this section only):
//	<chunk heading line if any>
//	<chunk body>
//
//	{format rules}
//	Output ONLY <title>...</title><body>...</body>.   ← only on first chunk
//	Output ONLY <body>...</body>.                     ← subsequent chunks
func (a *promptAssembler) assembleChunkPrompt(in chunkPromptInput, glossary map[string]string) string {
	var b strings.Builder

	// GLOSSARY block (stable order, applied per chunk for term consistency)
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

	// Hints
	for _, hint := range in.Hints {
		b.WriteString("HINT: ")
		b.WriteString(hint)
		b.WriteString("\n")
	}
	if len(in.Hints) > 0 {
		b.WriteString("\n")
	}

	// Title (every chunk — keeps model grounded)
	b.WriteString("SOURCE_TITLE: ")
	b.WriteString(in.Title)
	b.WriteString("\n\n")

	// Previously translated context (sliding window)
	b.WriteString("PREVIOUSLY_TRANSLATED_SECTIONS (context only — do NOT re-translate, do NOT include in output):\n")
	if in.PreviousContext == "" {
		b.WriteString("(none — this is the first chunk)\n")
	} else {
		b.WriteString(in.PreviousContext)
		b.WriteString("\n")
	}
	b.WriteString("\n---\n\n")

	// Current chunk to translate
	b.WriteString("TRANSLATE_NOW (translate this chunk only — preserve source structure 1:1):\n")
	if in.ChunkHeading != "" {
		b.WriteString(in.ChunkHeading)
		b.WriteString("\n")
	}
	b.WriteString(in.ChunkBody)
	b.WriteString("\n\n")

	// Format contract (same as non-chunked, but output target differs)
	b.WriteString("CRITICAL FORMAT RULES (output will be REJECTED if violated):\n")
	b.WriteString("- Output MUST be raw markdown. Do NOT use any HTML tags.\n")
	b.WriteString("- Forbidden HTML tags: <p>, <h1>..<h6>, <ul>, <ol>, <li>, <pre>, <blockquote>, <table>, <tr>, <td>, <th>, <div>, <section>.\n")
	b.WriteString("- Headings: use # / ## / ### matching source levels exactly.\n")
	b.WriteString("- Paragraphs: separate with blank lines (NO <p> tags).\n")
	b.WriteString("- Lists: use - or * or + (same marker style as source).\n")
	b.WriteString("- Links: [text](url). Images: ![alt](url).\n")
	b.WriteString("- Code blocks: triple-backtick fences with the same language tag as source.\n")
	b.WriteString("- Translate EVERY paragraph in TRANSLATE_NOW. Do NOT skip or summarize content.\n")
	b.WriteString("- Do NOT add headings, sections, or intermediate groupings that are not in TRANSLATE_NOW.\n")
	b.WriteString("- Output language: target language only (no source-language residue).\n\n")

	if in.IsFirst {
		b.WriteString("Output ONLY <title>...</title><body>...</body>. The <body> must contain the translation of TRANSLATE_NOW.")
	} else {
		b.WriteString("Output ONLY <body>...</body>. The <body> must contain ONLY the translation of TRANSLATE_NOW (no <title>).")
	}

	return b.String()
}
