package qwen3

import (
	"context"
	"fmt"
	"time"

	"github.com/iannil/huan/internal/observability"
	"github.com/iannil/huan/internal/translate"
)

// Plugin is the Qwen3 translate plugin. It implements both plugin.Plugin
// (via Name) and translate.Translator (via Translate), so it is discoverable
// both as a base plugin and as a translator via plugin.Find[translate.Translator].
type Plugin struct {
	cfg         Config
	client      *ollamaClient
	prompt      *promptAssembler
	quality     *qualityChecker
	projectRoot string
}

// New constructs a Plugin from parsed Config. projectRoot is the directory
// containing huan.yaml (used to resolve system_prompt_file / glossary_file
// relative paths).
//
// Returns an error if the system prompt file cannot be loaded. Does NOT
// ping Ollama at construction time — that's deferred to the first
// Translate call so users can construct plugins without a running Ollama
// (useful for `huan plugin list`).
//
// Logger is created per Translate() call (mirrors cloudflare plugin's
// per-Deploy() logger pattern). For batch correlation across files, the
// CLI orchestrator maintains its own outer logger.
func New(cfg Config, projectRoot string) (*Plugin, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("qwen3: projectRoot is required")
	}
	assembler, err := newPromptAssembler(projectRoot, cfg.SystemPromptFile)
	if err != nil {
		return nil, fmt.Errorf("qwen3: init prompt assembler: %w", err)
	}
	return &Plugin{
		cfg:         cfg,
		client:      nil, // lazy: created per Translate() call so logger/trace_id is per-invocation
		prompt:      assembler,
		quality:     newQualityChecker(cfg.Quality),
		projectRoot: projectRoot,
	}, nil
}

// Name is the unique plugin identifier. Pairs with the yaml key under plugins:
// (i.e. plugins.qwen3_translate.*).
func (p *Plugin) Name() string { return "qwen3_translate" }

// Config returns the parsed configuration. Used by the CLI to display
// effective config in `huan plugin info`.
func (p *Plugin) Config() Config { return p.cfg }

// Translate implements translate.Translator. It:
//  1. Assembles system + user prompts (glossary + source title + source body)
//  2. Calls Ollama HTTP /api/chat with the configured model
//  3. Parses <title>...</title><body>...</body> from the response
//  4. Runs quality checks (XML parse, language detection, markdown structure)
//  5. Returns Response with QualityChecks populated
//
// On hard quality check failure (XML parse / language detection / markdown
// structure), the Response is still returned (caller decides whether to
// consume) but QualityChecks.HardCheckFailures() will list the failures.
//
// Retries: when a soft check fails (length ratio, glossary compliance),
// Translate retries up to cfg.Quality.RetryOnViolation times with a hint
// appended to the user prompt.
func (p *Plugin) Translate(ctx context.Context, req translate.Request) (*translate.Response, error) {
	overallStart := time.Now()
	traceID := observability.NewTraceID()
	logger := observability.NewLogger(traceID)
	client := newOllamaClient(p.cfg.Endpoint, p.cfg.Timeout(), logger)

	spanID := "translate-" + req.SourceLang + "-" + req.TargetLang
	logger.LogFunctionStart(spanID, map[string]any{
		"source_lang": req.SourceLang,
		"target_lang": req.TargetLang,
		"model":       p.cfg.Model,
		"title_len":   len(req.Title),
		"content_len": len(req.Content),
	})

	// Validate request
	if req.Content == "" {
		return nil, fmt.Errorf("qwen3: request content is empty")
	}
	if req.SourceLang == "" || req.TargetLang == "" {
		return nil, fmt.Errorf("qwen3: source/target language is required")
	}

	localReq := translateRequest{
		Title:   req.Title,
		Content: req.Content,
		Hints:   req.Hints,
	}

	// Ping Ollama on first call (fail-fast)
	if err := client.ping(ctx); err != nil {
		return nil, fmt.Errorf("qwen3: ollama unreachable: %w", err)
	}

	var lastResponse *translate.Response
	maxAttempts := 1 + p.cfg.Quality.RetryOnViolation

	for attempt := 0; attempt < maxAttempts; attempt++ {
		userPrompt := p.prompt.assembleUserPrompt(localReq, req.Glossary)
		if attempt > 0 {
			// Append retry hint
			userPrompt += "\n\nNOTE: Previous attempt failed quality checks. Please be more careful with formatting and terminology."
		}

		callStart := time.Now()
		chatResp, err := client.chat(ctx, p.cfg.Model, p.prompt.systemPrompt, userPrompt)
		callDuration := time.Since(callStart)

		if err != nil {
			// Try fallback model if configured
			if p.cfg.FallbackModel != "" && p.cfg.FallbackModel != p.cfg.Model {
				logger.Log("translate-fallback", observability.EventBranch, map[string]any{
					"reason":        "primary model failed",
					"primary":       p.cfg.Model,
					"fallback":      p.cfg.FallbackModel,
					"primary_error": err.Error(),
				})
				chatResp, err = client.chat(ctx, p.cfg.FallbackModel, p.prompt.systemPrompt, userPrompt)
				callDuration = time.Since(callStart)
			}
			if err != nil {
				return nil, fmt.Errorf("qwen3: LLM call failed: %w", err)
			}
		}

		// Parse XML output
		parsed, err := parseXMLOutput(chatResp.Message.Content)
		if err != nil {
			// XML parse failure is a hard quality check failure — no retry
			// (LLM clearly didn't follow output contract).
			lastResponse = &translate.Response{
				Title:      "",
				Body:       chatResp.Message.Content, // raw output for debugging
				Model:      chatResp.Model,
				TokensUsed: chatResp.PromptEvalCount + chatResp.EvalCount,
				DurationMs: callDuration.Milliseconds(),
				QualityChecks: translate.QualityResult{
					XMLParse:          false,
					LanguageDetection: false,
					MarkdownStructure: false,
					RetryCount:        attempt,
				},
			}
			break
		}

		// Run quality checks
		qr := translate.QualityResult{
			XMLParse:   true,
			RetryCount: attempt,
		}
		qr.LanguageDetection = p.quality.CheckLanguageDetection(parsed.Body)
		qr.MarkdownStructure = p.quality.CheckMarkdownStructure(req.Content, parsed.Body)
		lengthRatio, lengthOK := p.quality.CheckLengthRatio(req.Content, parsed.Body)
		qr.LengthRatio = lengthRatio
		qr.GlossaryCompliance = p.quality.CheckGlossaryCompliance(parsed.Body, req.Glossary)

		lastResponse = &translate.Response{
			Title:         parsed.Title,
			Body:          parsed.Body,
			Model:         chatResp.Model,
			TokensUsed:    chatResp.PromptEvalCount + chatResp.EvalCount,
			DurationMs:    callDuration.Milliseconds(),
			QualityChecks: qr,
		}

		// If hard checks pass AND soft checks pass, done.
		hardFails := qr.HardCheckFailures()
		softOK := lengthOK && qr.GlossaryCompliance
		if len(hardFails) == 0 && softOK {
			break
		}
		// Hard failures never retry (LLM structural issue); only soft failures retry
		if len(hardFails) > 0 {
			logger.Log("translate-hard-fail", observability.EventPoint, map[string]any{
				"failed_checks": hardFails,
				"attempt":       attempt,
			})
			break
		}
		logger.Log("translate-soft-fail", observability.EventPoint, map[string]any{
			"length_ok":    lengthOK,
			"glossary_ok":  qr.GlossaryCompliance,
			"length_ratio": lengthRatio,
			"attempt":      attempt,
			"will_retry":   attempt+1 < maxAttempts,
		})
	}

	logger.LogFunctionEnd(spanID, time.Since(overallStart), map[string]any{
		"status":        translateResponseStatus(lastResponse),
		"hard_failures": lastResponse.QualityChecks.HardCheckFailures(),
		"length_ratio":  lastResponse.QualityChecks.LengthRatio,
		"glossary_ok":   lastResponse.QualityChecks.GlossaryCompliance,
		"tokens":        lastResponse.TokensUsed,
		"retries":       lastResponse.QualityChecks.RetryCount,
	})

	return lastResponse, nil
}

// translateResponseStatus returns a human-readable status string for logging.
func translateResponseStatus(r *translate.Response) string {
	if r == nil {
		return "nil"
	}
	if len(r.QualityChecks.HardCheckFailures()) > 0 {
		return "hard_fail"
	}
	if r.QualityChecks.LengthRatio < 0.5 || r.QualityChecks.LengthRatio > 2.5 {
		return "soft_warn_length"
	}
	if !r.QualityChecks.GlossaryCompliance {
		return "soft_warn_glossary"
	}
	return "success"
}
