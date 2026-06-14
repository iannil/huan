package qwen3

import (
	"context"
	"fmt"
	"strings"
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

// Translate implements translate.Translator via the chunked pipeline:
//
//  1. Split source body into Chunks by level-2 headings (## ).
//  2. For each chunk (in document order):
//     a. Build chunkPromptInput with PREVIOUSLY_TRANSLATED_SECTIONS
//        (sliding-window context from already-translated chunks).
//     b. Call Ollama.
//     c. Run per-chunk quality checks (format_purity, language_detection,
//        chunk_structure, length_ratio, glossary).
//     d. Retry up to RetryOnViolation times on failure.
//  3. Concatenate translated chunks into final body.
//  4. Return Response with aggregated QualityChecks.
//
// On any chunk's hard failure (XML parse / format_purity /
// language_detection / chunk_structure), the whole doc fails — no
// partial sidecar is written.
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

	// Ping Ollama on first call (fail-fast)
	if err := client.ping(ctx); err != nil {
		return nil, fmt.Errorf("qwen3: ollama unreachable: %w", err)
	}

	// Split source into chunks by level-2 headings.
	chunks := splitBySection(req.Content)
	logger.Log("chunk-split", observability.EventPoint, map[string]any{
		"chunk_total":      len(chunks),
		"has_preamble":     chunks[0].IsPreamble,
		"src_total_chars":  len(req.Content),
	})

	// Translate each chunk with sliding-window context.
	translatedChunks := make([]string, 0, len(chunks))
	var title string
	totalTokensUsed := 0
	var totalDurationMs int64
	maxRetryCount := 0
	// Doc-level char totals for aggregate length ratio.
	totalSrcChars := 0
	totalOutChars := 0
	// Aggregated hard-fail tracking: any single chunk's hard fail marks
	// the whole doc as hard-failed (no partial sidecar).
	aggXMLParse := true
	aggLanguageDetection := true
	aggChunkStructure := true
	aggFormatPurity := true
	aggGlossaryCompliance := true
	var lastFailReason string

	for chunkIdx, chunk := range chunks {
		chunkStart := time.Now()
		isFirst := chunkIdx == 0

		// Sliding-window context from previously translated chunks.
		prevContext := slidingWindowContext(translatedChunks, p.cfg.Quality.ChunkContextTokenBudget)

		// Per-chunk retry loop (attempts 0..RetryOnViolation).
		var (
			parsedChunk parsedOutput
			chunkTokens int
			chunkOK     bool
			chunkHard   []string
			retryUsed   int
		)

		for attempt := 0; attempt <= p.cfg.Quality.RetryOnViolation; attempt++ {
			retryUsed = attempt
			in := chunkPromptInput{
				Title:          req.Title,
				Hints:          req.Hints,
				ChunkHeading:   chunk.Heading,
				ChunkBody:      chunk.Body,
				PreviousContext: prevContext,
				IsFirst:        isFirst,
			}
			userPrompt := p.prompt.assembleChunkPrompt(in, req.Glossary)
			if attempt > 0 {
				userPrompt += "\n\nNOTE: Previous attempt for this chunk failed quality checks. " +
					"Please be more careful with format preservation, target language, and structure 1:1."
			}

			chatResp, err := client.chat(ctx, p.cfg.Model, p.prompt.systemPrompt, userPrompt)
			if err != nil {
				if p.cfg.FallbackModel != "" && p.cfg.FallbackModel != p.cfg.Model {
					logger.Log("chunk-fallback", observability.EventBranch, map[string]any{
						"chunk_index":   chunk.Index,
						"primary":       p.cfg.Model,
						"fallback":      p.cfg.FallbackModel,
						"primary_error": err.Error(),
					})
					chatResp, err = client.chat(ctx, p.cfg.FallbackModel, p.prompt.systemPrompt, userPrompt)
				}
				if err != nil {
					// HTTP/infra error → no point retrying within chunk loop.
					return nil, fmt.Errorf("qwen3: chunk %d/%d LLM call failed: %w",
						chunk.Index, len(chunks), err)
				}
			}

			// Parse XML output. First chunk requires <title> + <body>;
			// subsequent chunks require only <body>.
			var parsed parsedOutput
			if isFirst {
				parsed, err = parseXMLOutput(chatResp.Message.Content)
			} else {
				parsed, err = parseChunkBodyOutput(chatResp.Message.Content)
			}
			if err != nil {
				// XML parse failure = hard fail, no retry.
				aggXMLParse = false
				lastFailReason = fmt.Sprintf("chunk %d/%d XML parse: %v", chunk.Index, len(chunks), err)
				logger.Log("chunk-xml-fail", observability.EventPoint, map[string]any{
					"chunk_index": chunk.Index,
					"attempt":     attempt,
					"reason":      err.Error(),
				})
				chunkHard = []string{"xml_parse"}
				break
			}

			// Per-chunk quality checks (vs chunk source slice).
			chunkSrc := chunk.Source()
			chunkOut := parsed.Body
			langOK := p.quality.CheckLanguageDetection(chunkOut)
			structOK := p.quality.CheckChunkStructure(chunkSrc, chunkOut)
			formatOK := p.quality.CheckFormatPurity(chunkOut)
			lengthRatio, lengthOK := p.quality.CheckLengthRatio(chunkSrc, chunkOut)
			glossaryOK := p.quality.CheckGlossaryCompliance(chunkOut, req.Glossary)

			hardFails := []string{}
			if !langOK {
				hardFails = append(hardFails, "language_detection")
			}
			if !structOK {
				hardFails = append(hardFails, "markdown_structure")
			}
			if !formatOK {
				hardFails = append(hardFails, "format_purity")
			}

			if !structOK {
				diag := p.quality.checkChunkStructureDetailed(chunkSrc, chunkOut)
				logger.Log("chunk-structure-diag", observability.EventPoint, map[string]any{
					"chunk_index":       chunk.Index,
					"attempt":           attempt,
					"reason":            diag.FailedReason,
					"src_headings":      diag.SrcHeadings,
					"out_headings":      diag.OutHeadings,
					"src_paragraphs":    diag.SrcParagraphs,
					"out_paragraphs":    diag.OutParagraphs,
					"src_list_items":    diag.SrcListItems,
					"out_list_items":    diag.OutListItems,
					"src_content_blocks": diag.SrcContentBlocks,
					"out_content_blocks": diag.OutContentBlocks,
				})
			}

			logger.Log("chunk-check", observability.EventPoint, map[string]any{
				"chunk_index":    chunk.Index,
				"attempt":        attempt,
				"hard_fails":     hardFails,
				"length_ok":      lengthOK,
				"glossary_ok":    glossaryOK,
				"length_ratio":   lengthRatio,
			})

			// Hard fail → no retry (LLM structural issue).
			if len(hardFails) > 0 {
				chunkHard = hardFails
				lastFailReason = fmt.Sprintf("chunk %d/%d hard fail: %v", chunk.Index, len(chunks), hardFails)
				logger.Log("chunk-hard-fail", observability.EventPoint, map[string]any{
					"chunk_index": chunk.Index,
					"attempt":     attempt,
					"failed":      hardFails,
					"will_retry":  false,
				})
				break
			}

			// Soft fail → retry if budget remains.
			if (!lengthOK || !glossaryOK) && attempt < p.cfg.Quality.RetryOnViolation {
				logger.Log("chunk-soft-fail", observability.EventPoint, map[string]any{
					"chunk_index": chunk.Index,
					"attempt":     attempt,
					"length_ok":   lengthOK,
					"glossary_ok": glossaryOK,
					"will_retry":  true,
				})
				continue
			}

			// Success (or exhausted retries with soft fail only — acceptable).
			parsedChunk = parsed
			chunkTokens = chatResp.PromptEvalCount + chatResp.EvalCount
			chunkOK = true
			// Aggregate glossary for the doc-level response. Soft fails
			// don't block but are reflected in the final QualityResult.
			if !glossaryOK {
				aggGlossaryCompliance = false
			}
			break
		}

		chunkDurationMs := time.Since(chunkStart).Milliseconds()
		totalDurationMs += chunkDurationMs

		if !chunkOK {
			// Aggregate hard fails for doc-level Response.
			for _, hf := range chunkHard {
				switch hf {
				case "language_detection":
					aggLanguageDetection = false
				case "markdown_structure":
					aggChunkStructure = false
				case "format_purity":
					aggFormatPurity = false
				}
			}
			if retryUsed > maxRetryCount {
				maxRetryCount = retryUsed
			}
			logger.Log("chunk-aborted", observability.EventPoint, map[string]any{
				"chunk_index": chunk.Index,
				"reason":      lastFailReason,
				"hard_fails":  chunkHard,
			})
			// Atomic: stop on first chunk failure. Already-translated
			// chunks are discarded; no partial sidecar written.
			break
		}

		// Success path
		if isFirst {
			title = parsedChunk.Title
		}
		translatedChunks = append(translatedChunks, parsedChunk.Body)
		totalTokensUsed += chunkTokens
		totalSrcChars += len(chunk.Source())
		totalOutChars += len(parsedChunk.Body)
		if retryUsed > maxRetryCount {
			maxRetryCount = retryUsed
		}

		logger.Log("chunk-complete", observability.EventPoint, map[string]any{
			"chunk_index":   chunk.Index,
			"chunk_total":   len(chunks),
			"src_chars":     len(chunk.Source()),
			"out_chars":     len(parsedChunk.Body),
			"tokens":        chunkTokens,
			"duration_ms":   chunkDurationMs,
			"context_tokens": estimateTokens(prevContext),
			"attempts":      retryUsed + 1,
		})
	}

	// Assemble final doc-level response.
	// If any chunk hard-failed, Body is empty (atomic — no partial).
	body := ""
	status := "success"
	if len(translatedChunks) == len(chunks) {
		body = strings.Join(translatedChunks, "\n\n")
	} else {
		status = "hard_fail"
	}

	// Aggregate length ratio from chunk char totals.
	var docLengthRatio float64
	if totalSrcChars > 0 {
		docLengthRatio = float64(totalOutChars) / float64(totalSrcChars)
	}

	resp := &translate.Response{
		Title:      title,
		Body:       body,
		Model:      p.cfg.Model,
		TokensUsed: totalTokensUsed,
		DurationMs: time.Since(overallStart).Milliseconds(),
		QualityChecks: translate.QualityResult{
			XMLParse:           aggXMLParse,
			LanguageDetection:  aggLanguageDetection,
			MarkdownStructure:  aggChunkStructure,
			FormatPurity:       aggFormatPurity,
			LengthRatio:        docLengthRatio,
			GlossaryCompliance: aggGlossaryCompliance,
			RetryCount:         maxRetryCount,
		},
	}

	logger.LogFunctionEnd(spanID, time.Since(overallStart), map[string]any{
		"status":         status,
		"hard_failures":  resp.QualityChecks.HardCheckFailures(),
		"chunks_total":   len(chunks),
		"chunks_ok":      len(translatedChunks),
		"tokens":         totalTokensUsed,
		"glossary_ok":    aggGlossaryCompliance,
		"retries_max":    maxRetryCount,
		"last_fail":      lastFailReason,
	})

	return resp, nil
}

// translateResponseStatus returns a human-readable status string for logging.
func translateResponseStatus(r *translate.Response) string {
	if r == nil {
		return "nil"
	}
	if len(r.QualityChecks.HardCheckFailures()) > 0 {
		return "hard_fail"
	}
	if r.QualityChecks.LengthRatio < 0.5 || r.QualityChecks.LengthRatio > 3.5 {
		return "soft_warn_length"
	}
	if !r.QualityChecks.GlossaryCompliance {
		return "soft_warn_glossary"
	}
	return "success"
}
