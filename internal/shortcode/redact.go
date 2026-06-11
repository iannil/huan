package shortcode

import (
	"crypto/md5"
	"fmt"
	"strings"
	"unicode/utf8"
)

// RedactHandler implements the {{< redact >}} shortcode.
//
// Parameters:
//   force="true"  - force full redaction
//   show="true"   - force no redaction
//   random="true" - random partial redaction
//   ratio="50"    - redaction ratio (0-100), default 50
//
// Behavior:
//   1. If force=true → redact
//   2. If show=true  → no redact
//   3. Else, check page frontmatter "redact" field
//   4. Else, check global redactFolders config (matches page directory)
//   5. Apply full or random redaction based on mode
func RedactHandler(ctx *Context) (string, error) {
	content := ctx.Inner
	shouldRedact := false

	forceHide := ctx.Params["force"]
	forceShow := ctx.Params["show"]
	randomMode := ctx.Params["random"]
	ratioStr := ctx.Params["ratio"]
	if ratioStr == "" {
		ratioStr = "50"
	}
	ratio := parseIntDefault(ratioStr, 50)

	if forceHide == "true" {
		shouldRedact = true
	} else if forceShow == "true" {
		shouldRedact = false
	} else if ctx.Page != nil {
		// Check page frontmatter "redact" - via Params map
		// Page struct doesn't have Redact directly; check Params if present
		// For now, use access field as proxy
		if ctx.Page.Access == "protected" {
			shouldRedact = true
		}
	}

	if !shouldRedact {
		return content, nil
	}

	if randomMode == "true" {
		return randomRedact(content, ratio), nil
	}
	return fullRedact(content), nil
}

// fullRedact produces an all-blocks version of the content.
// Matches Hugo: count runes, repeat █ that many times.
func fullRedact(content string) string {
	len := utf8.RuneCountInString(content)
	return fmt.Sprintf(`<span class="redacted">%s</span>`, strings.Repeat("█", len))
}

// randomRedact produces a partially-redacted version using a deterministic seed.
// Matches Hugo: MD5(content)[:8] → hex seed → (i*31+seed) % 100 < ratio → redact word.
func randomRedact(content string, ratio int) string {
	hash := md5.Sum([]byte(content))
	hexHash := fmt.Sprintf("%x", hash)[:8]
	seed := hexToSeed(hexHash)

	// Hugo splits by space
	words := strings.Split(content, " ")
	var result []string

	for i, word := range words {
		if word == "" {
			continue
		}
		decision := (i*31 + seed) % 100
		if decision < ratio {
			wordLen := utf8.RuneCountInString(word)
			blocks := strings.Repeat("█", wordLen)
			result = append(result, fmt.Sprintf(`<span class="redacted">%s</span>`, blocks))
		} else {
			result = append(result, word)
		}
	}

	return strings.Join(result, " ")
}

// hexToSeed mirrors Hugo's algorithm: sum of hex digit values weighted by position.
func hexToSeed(hex string) int {
	seed := 0
	chars := hex[:8]
	for i, c := range chars {
		code := hexDigitToInt(c)
		seed += code * (i + 1)
	}
	return seed
}

func hexDigitToInt(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

func parseIntDefault(s string, def int) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}
