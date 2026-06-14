// Package translate defines the Translator capability interface — the second
// concrete capability in huan's unified plugin system (after deploy.Deployer,
// see docs/adr/0003-unified-plugin-system.md and docs/adr/0008-translator-capability-qwen3-plugin.md).
//
// Plugins that translate content between languages (e.g. zh-cn → en via a
// local LLM, or future API providers) implement Translator. The capability
// owns only the contract + shared request/response types. Each provider
// implementation lives in its own subpackage (e.g. internal/translate/qwen3).
package translate

import (
	"context"

	"github.com/iannil/huan/internal/plugin"
)

// Translator is the capability interface for plugins that translate content
// between languages. It embeds plugin.Plugin (so translators are discoverable
// as base plugins) and adds Translate.
//
// A plugin implementing Translator registers under a unique Name (e.g.
// "qwen3_translate") and is queried via:
//
//	translators := plugin.Find[translate.Translator](registry)
type Translator interface {
	plugin.Plugin

	// Translate converts source content to the target language.
	//
	// Contract:
	//   - Honor ctx for cancellation.
	//   - Return a Response even on partial quality-check failures; only
	//     return a non-nil error when the call cannot proceed at all (e.g.
	//     LLM unreachable, invalid config, malformed request).
	//   - Per-file quality issues (XML parse, language detection, markdown
	//     structure) are encoded in Response.QualityChecks, not the error.
	//   - The caller decides whether to consume a Response with hard quality
	//     failures (typically: discard and log).
	Translate(ctx context.Context, req Request) (*Response, error)
}

// Request carries the inputs to a single Translate call. Value type on
// purpose: callers fill it and pass to plugin; immutable semantics.
type Request struct {
	// SourceLang is the source language code (e.g. "zh-cn").
	SourceLang string

	// TargetLang is the target language code (e.g. "en").
	TargetLang string

	// Title is the source title. Translated separately in the same LLM call
	// (output as <title>...</title>). May be empty for body-only translation.
	Title string

	// Content is the source markdown body to translate.
	Content string

	// ContentType identifies the content format. Currently only "markdown"
	// is supported; future values: "plain", "html".
	ContentType string

	// Hints are user-supplied prompt hints appended to the system prompt.
	// Example: ["preserve philosophical tone", "use contractions"].
	Hints []string

	// Glossary maps source-language terms to their fixed target-language
	// translations. The plugin injects these into the prompt as a GLOSSARY
	// block AND post-validates that the output honors them.
	Glossary map[string]string
}

// Response carries the result of a Translate call.
type Response struct {
	// Title is the translated title. Empty when Request.Title was empty.
	Title string

	// Body is the translated body content.
	Body string

	// Model is the model identifier used (e.g. "qwen3-next:80b-a3b-instruct-q4_K_M").
	Model string

	// TokensUsed is the total tokens consumed (input + output).
	TokensUsed int

	// DurationMs is the LLM call duration in milliseconds.
	DurationMs int64

	// QualityChecks reports the per-check pass/fail status.
	QualityChecks QualityResult
}

// QualityResult reports the outcome of post-translation quality checks.
// Hard checks (XMLParse, LanguageDetection, MarkdownStructure) failing
// means the output is unusable and should be discarded. Soft checks
// (LengthRatio, GlossaryCompliance, repetition) failing means the output
// is usable but quality is degraded.
type QualityResult struct {
	// XMLParse is true when the LLM output parsed as
	// <title>...</title><body>...</body>. HARD check.
	XMLParse bool `json:"xml_parse"`

	// LanguageDetection is true when the output is ≥ 80% (configurable)
	// the target language. HARD check.
	LanguageDetection bool `json:"language_detection"`

	// MarkdownStructure is true when heading/list/link/image counts in
	// the output match the source ± tolerance (configurable). HARD check.
	MarkdownStructure bool `json:"markdown_structure"`

	// LengthRatio is body_words / source_words. Outside [0.5, 2.5]
	// (configurable) is a soft warning.
	LengthRatio float64 `json:"length_ratio"`

	// GlossaryCompliance is true when all glossary terms were correctly
	// applied in the output. Soft check.
	GlossaryCompliance bool `json:"glossary_compliance"`

	// RetryCount is the number of retries triggered by quality failures.
	RetryCount int `json:"retry_count"`
}

// HardCheckFailures returns the list of hard check names that failed.
// Empty when all hard checks pass. Used by callers to decide whether to
// discard the response.
func (q QualityResult) HardCheckFailures() []string {
	var failed []string
	if !q.XMLParse {
		failed = append(failed, "xml_parse")
	}
	if !q.LanguageDetection {
		failed = append(failed, "language_detection")
	}
	if !q.MarkdownStructure {
		failed = append(failed, "markdown_structure")
	}
	return failed
}
