package qwen3

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		in   string
		// expect roughly len(runes) / 3
		wantMin int
		wantMax int
	}{
		{"empty", "", 0, 0},
		{"single ascii", "a", 1, 1},
		{"ascii word", "hello", 1, 2},
		{"ascii sentence", "the quick brown fox jumps over", 8, 12},
		{"single cjk", "法", 1, 1},
		{"cjk sentence", "法不净空觉无性也", 2, 3},
		{"mixed", "Hello 法不净空 World", 5, 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := estimateTokens(tc.in)
			if got < tc.wantMin || got > tc.wantMax {
				t.Errorf("estimateTokens(%q) = %d, want in [%d, %d]", tc.in, got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestSlidingWindowContext_Empty(t *testing.T) {
	if got := slidingWindowContext(nil, 8000); got != "" {
		t.Errorf("nil previous with budget = %q, want empty", got)
	}
	if got := slidingWindowContext([]string{}, 8000); got != "" {
		t.Errorf("empty previous with budget = %q, want empty", got)
	}
}

func TestSlidingWindowContext_ZeroBudget(t *testing.T) {
	prev := []string{"chunk 1", "chunk 2"}
	if got := slidingWindowContext(prev, 0); got != "" {
		t.Errorf("zero budget = %q, want empty", got)
	}
}

func TestSlidingWindowContext_AllFits(t *testing.T) {
	prev := []string{"aaa", "bbb", "ccc"}
	// Each ~1 token, budget 8000 → all fit
	got := slidingWindowContext(prev, 8000)
	for _, p := range prev {
		if !strings.Contains(got, p) {
			t.Errorf("expected %q in result, got %q", p, got)
		}
	}
}

func TestSlidingWindowContext_BudgetOverflow(t *testing.T) {
	// Each chunk ~3000 chars → ~1000 tokens. Budget 1500.
	// Most-recent always included (~1000). Second-most-recent would push
	// to ~2000, exceeds 1500. Result: only the last chunk.
	prev := []string{
		strings.Repeat("a", 3000),
		strings.Repeat("b", 3000),
		strings.Repeat("c", 3000),
	}
	got := slidingWindowContext(prev, 1500)
	if !strings.Contains(got, strings.Repeat("c", 3000)) {
		t.Error("most-recent chunk must always be included")
	}
	if strings.Contains(got, strings.Repeat("a", 3000)) {
		t.Error("oldest chunk should be excluded when over budget")
	}
	if strings.Contains(got, strings.Repeat("b", 3000)) {
		t.Error("middle chunk should be excluded when over budget")
	}
}

func TestSlidingWindowContext_PartialFit(t *testing.T) {
	// 3 chunks of ~500 tokens each, budget 1200.
	// Most-recent: 500. Add middle: 1000. Add oldest: 1500 > 1200, stop.
	// Result: middle + most-recent.
	chunkA := strings.Repeat("a", 1500) // ~500 tokens
	chunkB := strings.Repeat("b", 1500)
	chunkC := strings.Repeat("c", 1500)
	prev := []string{chunkA, chunkB, chunkC}
	got := slidingWindowContext(prev, 1200)
	if !strings.Contains(got, chunkB) {
		t.Error("middle chunk should be included")
	}
	if !strings.Contains(got, chunkC) {
		t.Error("most-recent chunk should be included")
	}
	if strings.Contains(got, chunkA) {
		t.Error("oldest chunk should be excluded (would exceed budget)")
	}
}

func TestSlidingWindowContext_OrderIsDocumentOrder(t *testing.T) {
	prev := []string{"first", "second", "third"}
	got := slidingWindowContext(prev, 1000)
	// All fit; verify they appear in document order.
	idxFirst := strings.Index(got, "first")
	idxSecond := strings.Index(got, "second")
	idxThird := strings.Index(got, "third")
	if !(idxFirst < idxSecond && idxSecond < idxThird) {
		t.Errorf("chunks not in document order: idxFirst=%d idxSecond=%d idxThird=%d in %q",
			idxFirst, idxSecond, idxThird, got)
	}
}

func TestSlidingWindowContext_SingleChunkOverBudget(t *testing.T) {
	// One chunk that exceeds budget. Should still include it (better
	// one neighbor than zero).
	big := strings.Repeat("x", 30000) // ~10000 tokens
	prev := []string{big}
	got := slidingWindowContext(prev, 1000)
	if !strings.Contains(got, big) {
		t.Error("single chunk over budget should still be included")
	}
}

func TestSlidingWindowContext_ChunkSeparator(t *testing.T) {
	prev := []string{"aaa", "bbb"}
	got := slidingWindowContext(prev, 8000)
	if !strings.Contains(got, "---") {
		t.Errorf("chunks should be separated by '---', got %q", got)
	}
}
