package qwen3

import (
	"unicode"
	"unicode/utf8"
)

// estimateTokens approximates the LLM token count of a string. The exact
// tokenizer is BPE (Qwen3 uses tiktoken-style), which we don't link in.
// This heuristic is intentionally rough — its job is to size the sliding
// window, not to bill tokens.
//
// Empirically for mixed CJK/ASCII markdown:
//   - Pure CJK: ~2.5 chars/token (denser encoding)
//   - Pure ASCII (English): ~4 chars/token
//   - Mixed: ~3 chars/token
//
// We pick 3 as the universal divisor. This tends to OVER-estimate CJK
// (more conservative — fewer previous chunks fit in budget) and
// UNDER-estimate ASCII (less conservative — more previous chunks fit).
// The asymmetry is acceptable because the budget is a soft target.
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	charCount := utf8.RuneCountInString(s)
	tokens := charCount / 3
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// slidingWindowContext returns the largest suffix of `previous` whose
// total estimated tokens fit within `budget`. Used to inject narrative
// context into chunked translation without exceeding the model's context
// window.
//
// Behavior:
//   - If budget <= 0, returns "" (no context).
//   - If the LAST previous chunk already exceeds budget, returns just
//     that chunk (better to have one recent neighbor than nothing).
//   - Otherwise, accumulates chunks from most-recent backwards until
//     adding the next-older chunk would exceed budget.
//
// Returned string is the joined chunks in DOCUMENT ORDER (oldest first),
// separated by "\n\n---\n\n" so the LLM sees clear chunk boundaries.
func slidingWindowContext(previous []string, budget int) string {
	if budget <= 0 || len(previous) == 0 {
		return ""
	}

	// Walk backwards from most-recent, accumulate while under budget.
	var selected []string
	total := 0
	for i := len(previous) - 1; i >= 0; i-- {
		chunkTokens := estimateTokens(previous[i])
		// Always include the most-recent chunk, even if it alone exceeds
		// budget. Better to have one neighbor than zero context.
		if len(selected) > 0 && total+chunkTokens > budget {
			break
		}
		selected = append([]string{previous[i]}, selected...)
		total += chunkTokens
	}

	if len(selected) == 0 {
		return ""
	}

	// Join with a visible separator so the LLM sees chunk boundaries.
	sep := "\n\n---\n\n"
	out := selected[0]
	for _, s := range selected[1:] {
		out += sep + s
	}
	return out
}

// isCJK returns true if the rune is in the CJK Unified Ideographs block.
// Used only for diagnostics; estimateTokens uses a simpler char/3 ratio.
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r)
}
