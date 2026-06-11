// Package encrypt implements full-page content access control: public, protected, private.
// Mirrors layouts/partials/content-redact.html logic.
package encrypt

import (
	"crypto/md5"
	"fmt"
	"html/template"
	"strings"
	"unicode/utf8"

	"github.com/novel_ttl/huan/internal/content"
	"github.com/novel_ttl/huan/internal/shortcode"
)

// EncryptedEntry is a single entry from data/encrypted/content.json.
type EncryptedEntry struct {
	Encrypted string `json:"encrypted"`
	Seed      string `json:"seed"`
}

// Engine handles per-page access-control decisions and rendering.
type Engine struct {
	encryptedData map[string]EncryptedEntry // keyed by fileId = MD5(file path)
	encryptGroups map[string]EncryptGroupConfig
}

// EncryptGroupConfig matches the config.EncryptGroupConfig shape.
type EncryptGroupConfig struct {
	Hint  string
	Mode  string // "full" or "random"
	Ratio int
}

// NewEngine creates a new encrypt engine.
// encryptedData: from Site.Data["encrypted"]["content"] (map[string]map)
// encryptGroups: from params.encryptGroups config
func NewEngine(encryptedData interface{}, encryptGroups map[string]EncryptGroupConfig) *Engine {
	e := &Engine{
		encryptGroups: encryptGroups,
		encryptedData: map[string]EncryptedEntry{},
	}

	// Normalize encryptedData into map[string]EncryptedEntry
	if m, ok := encryptedData.(map[string]interface{}); ok {
		for k, v := range m {
			if vm, ok := v.(map[string]interface{}); ok {
				entry := EncryptedEntry{}
				if enc, ok := vm["encrypted"].(string); ok {
					entry.Encrypted = enc
				}
				if seed, ok := vm["seed"].(string); ok {
					entry.Seed = seed
				}
				e.encryptedData[k] = entry
			}
		}
	}

	return e
}

// Render produces the final HTML for a page's content, applying access control.
// For public content: returns content as-is.
// For protected content: returns encrypted-content div with redaction.
func (e *Engine) Render(page *content.Page, scRegistry *shortcode.Registry, site *content.Site) (template.HTML, error) {
	access := page.Access
	if access == "" {
		access = "public"
	}

	if access != "protected" {
		// Public content: just return as-is
		return page.Content, nil
	}

	// Protected content: determine encrypt group
	groupName := page.EncryptGroup
	if groupName == "" {
		groupName = "default"
	}

	var mode string
	var ratio int
	if gc, ok := e.encryptGroups[groupName]; ok {
		mode = gc.Mode
		ratio = gc.Ratio
		if mode == "" {
			mode = "full"
		}
	} else if gc, ok := e.encryptGroups["default"]; ok {
		mode = gc.Mode
		ratio = gc.Ratio
		if mode == "" {
			mode = "full"
		}
	} else {
		mode = "full"
	}

	// Frontmatter can override
	if page.EncryptMode != "" {
		mode = page.EncryptMode
	}
	if page.EncryptRatio > 0 {
		ratio = page.EncryptRatio
	}

	// Generate fileId from file path
	fileId := ""
	if page.RelPath != "" {
		h := md5.Sum([]byte(page.RelPath))
		fileId = fmt.Sprintf("%x", h)
	}

	encData, hasEncrypted := e.encryptedData[fileId]

	// Build output HTML
	var sb strings.Builder

	sb.WriteString(`<div class="encrypted-content"`)
	sb.WriteString(fmt.Sprintf(` data-encrypted="%s"`, encData.Encrypted))
	sb.WriteString(fmt.Sprintf(` data-group="%s"`, groupName))
	sb.WriteString(fmt.Sprintf(` data-mode="%s"`, mode))
	sb.WriteString(fmt.Sprintf(` data-ratio="%d"`, ratio))
	sb.WriteString(fmt.Sprintf(` data-seed="%s"`, encData.Seed))
	sb.WriteString(` data-title-selector=".post-title">`)
	sb.WriteString(`<div class="encrypted-content-body">`)

	if mode == "random" {
		// Random redact: keep content visible, JS handles partial redaction
		sb.WriteString(fmt.Sprintf(`<div class="random-redact-content" data-seed="%s" data-ratio="%d">`, encData.Seed, ratio))
		sb.WriteString(string(page.Content))
		sb.WriteString(`</div>`)
	} else {
		// Full redact: render all-blocks placeholder
		plain := stripHTML(string(page.Content))
		length := utf8.RuneCountInString(plain)
		blocks := strings.Repeat("█", length)
		sb.WriteString(fmt.Sprintf(`<div class="redacted-content"><span class="redacted">%s</span></div>`, blocks))
	}

	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	// If no encrypted data was found, fall back to a non-decryptable redaction
	if !hasEncrypted {
		return e.renderFallback(page, mode, ratio), nil
	}

	return template.HTML(sb.String()), nil
}

// renderFallback produces the redaction when encrypted data isn't found
// (matches Hugo's fallback branch in content-redact.html).
func (e *Engine) renderFallback(page *content.Page, mode string, ratio int) template.HTML {
	var sb strings.Builder

	if mode == "random" {
		seedSource := page.RelPath
		if seedSource == "" {
			seedSource = page.URL
		}
		h := md5.Sum([]byte(seedSource))
		hash := fmt.Sprintf("%x", h)
		sb.WriteString(fmt.Sprintf(`<div class="random-redact-content" data-seed="%s" data-ratio="%d">`, hash, ratio))
		sb.WriteString(string(page.Content))
		sb.WriteString(`</div>`)
	} else {
		plain := stripHTML(string(page.Content))
		length := utf8.RuneCountInString(plain)
		blocks := strings.Repeat("█", length)
		sb.WriteString(fmt.Sprintf(`<div class="redacted-content"><span class="redacted">%s</span></div>`, blocks))
	}

	return template.HTML(sb.String())
}

// stripHTML removes HTML tags - mirrors plainify in Hugo.
func stripHTML(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
