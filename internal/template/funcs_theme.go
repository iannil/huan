package template

import (
	"fmt"
	"html/template"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Site-theme template functions, promoted from the retired zhurongshuo
// theme plugin (ADR 0012 reversal: the theme lives in the site repo as
// layouts/ + static/, while its Go helpers ship with the huan binary).
// Behavior is byte-for-byte identical to the plugin implementations.

// readingTime estimates reading time for mixed CJK + English content.
// Chinese characters are counted at 300 chars/minute and English words
// at 200 words/minute. Returns a human-friendly string like "5 分钟".
func readingTime(content string) template.HTML {
	cjkCount := 0
	wordCount := 0
	inWord := false
	for _, r := range content {
		if r >= 0x4E00 && r <= 0x9FFF {
			cjkCount++
			inWord = false
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			if !inWord {
				wordCount++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	minutes := math.Ceil(float64(cjkCount)/300 + float64(wordCount)/200)
	if minutes < 1 {
		minutes = 1
	}
	return template.HTML(fmt.Sprintf("%d 分钟", int(minutes)))
}

// parseGuideYAML extracts the first ```guide fenced code block from a
// page's plain text and parses it as YAML. Guide handbook pages embed
// their structured content (thesis, main_chart, concepts, map,
// takeaways) this way; the layout template calls this function with
// .Plain. A missing block or malformed YAML is a hard error so the
// build log surfaces it (render errors count into Result.Errors).
func parseGuideYAML(plain string) (map[string]interface{}, error) {
	const fence = "```guide"
	start := strings.Index(plain, fence)
	if start < 0 {
		return nil, fmt.Errorf("guide page has no ```guide code block")
	}
	rest := plain[start+len(fence):]
	end := strings.Index(rest, "```")
	if end < 0 {
		return nil, fmt.Errorf("guide block not closed with ```")
	}
	block := rest[:end]
	// plainify may have HTML-escaped or joined lines; yaml handles \n.
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(block), &data); err != nil {
		return nil, fmt.Errorf("guide block YAML: %w", err)
	}
	return data, nil
}

// svgTextWidth estimates rendered text width. CJK chars and full-width
// punctuation measure 1.0×fontSize; other (ASCII/Latin) chars ≈0.55×.
// Estimation, not measurement — budgets in callers leave slack.
func svgTextWidth(s string, fontSize int) float64 {
	w := 0.0
	for _, r := range s {
		if isWideRune(r) {
			w += float64(fontSize)
		} else {
			w += float64(fontSize) * 0.55
		}
	}
	return w
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK punctuation
		return true
	case r >= 0xFF00 && r <= 0xFFEF: // fullwidth forms
		return true
	case r == 0x2014 || r == 0x2026: // em dash, ellipsis
		return true
	}
	return false
}

// svgTruncate returns s unchanged when it fits maxWidth; otherwise cuts
// it at the character boundary that fits and appends "…".
func svgTruncate(s string, maxWidth float64, fontSize int) string {
	if svgTextWidth(s, fontSize) <= maxWidth {
		return s
	}
	runes := []rune(s)
	ellipsisW := svgTextWidth("…", fontSize)
	acc := 0.0
	for i, r := range runes {
		rw := svgTextWidth(string(r), fontSize)
		if acc+rw+ellipsisW > maxWidth {
			return string(runes[:i]) + "…"
		}
		acc += rw
	}
	return s
}

// svgWrap prefers word boundaries for Latin text and character boundaries
// for CJK. A word wider than the entire line is split as a last resort.
func svgWrap(s string, maxWidth float64, fontSize int) []string {
	var lines []string
	cur := ""
	pendingSpace := false
	flush := func() {
		if cur != "" {
			lines = append(lines, cur)
			cur = ""
		}
	}
	runes := []rune(s)
	for i := 0; i < len(runes); {
		if unicode.IsSpace(runes[i]) {
			pendingSpace = true
			i++
			continue
		}
		j := i + 1
		if !isWideRune(runes[i]) {
			for j < len(runes) && !unicode.IsSpace(runes[j]) && !isWideRune(runes[j]) {
				j++
			}
		}
		token := string(runes[i:j])
		sep := ""
		if pendingSpace && cur != "" {
			sep = " "
		}
		if cur != "" && svgTextWidth(cur+sep+token, fontSize) > maxWidth {
			flush()
			sep = ""
		}
		cur += sep
		for _, r := range token {
			if cur != "" && svgTextWidth(cur+string(r), fontSize) > maxWidth {
				flush()
			}
			cur += string(r)
		}
		pendingSpace = false
		i = j
	}
	flush()
	return lines
}

// failRender lets guide templates convert content-authoring mistakes
// (unknown book slug, unsupported chart_type, missing fields) into render
// errors. huan counts render errors into Result.Errors and logs a WARN
// per page — surfaced in the build summary "Errors:" line (pipeline_render.go).
func failRender(msg string) (string, error) {
	return "", fmt.Errorf("guide: %s", msg)
}

// guideChartTypes lists chart partials supported by the guide layout.
// Templates validate main_chart.chart_type against this set before
// dispatching to charts/<type>.html.
var guideChartTypes = map[string]bool{
	"funnel": true, "ladder": true, "cycle": true, "layers": true,
	"flow": true, "network": true, "spectrum": true,
}

// guideChartTypesFn exposes the supported chart types to templates as a
// sorted slice usable with `in`: {{ if not (in (guideChartTypes) $ct) }}...
// (huan's indexFunc does not look up map[string]bool, so a plain slice
// composes better with the template builtin set.)
func guideChartTypesFn() []string {
	types := make([]string, 0, len(guideChartTypes))
	for k := range guideChartTypes {
		types = append(types, k)
	}
	sort.Strings(types)
	return types
}

// PublicationRevisionNoticeContent is the localized, user-visible notice copy.
type PublicationRevisionNoticeContent struct {
	AriaLabel string
	Title     string
	Body      string
}

type revisionNoticeConfig struct {
	enabled    bool
	urlRegexp  *regexp.Regexp
	content    map[string]PublicationRevisionNoticeContent
}

var (
	revisionNotice    revisionNoticeConfig
	revisionNoticeSet bool
	revisionNoticeMu  sync.RWMutex
)

// SetPublicationRevisionNotice installs the parsed
// params.publicationRevisionNotice configuration used by the
// publicationRevisionNotice template function. Call with nil to disable.
// Mirrors the retired zhurongshuo theme plugin's ParseConfig validation.
func SetPublicationRevisionNotice(raw map[string]any) error {
	cfg := revisionNoticeConfig{}
	if raw == nil {
		revisionNoticeMu.Lock()
		revisionNotice, revisionNoticeSet = cfg, true
		revisionNoticeMu.Unlock()
		return nil
	}
	notice, ok := raw["publicationRevisionNotice"].(map[string]any)
	if !ok && raw["publicationRevisionNotice"] != nil {
		return fmt.Errorf("publicationRevisionNotice: expected map, got %T", raw["publicationRevisionNotice"])
	}
	for key := range notice {
		switch key {
		case "enabled", "urlPattern", "content":
		default:
			return fmt.Errorf("publicationRevisionNotice: unknown field %q", key)
		}
	}
	if value, exists := notice["enabled"]; exists {
		enabled, ok := value.(bool)
		if !ok {
			return fmt.Errorf("publicationRevisionNotice.enabled: expected bool, got %T", value)
		}
		cfg.enabled = enabled
	}
	if value, exists := notice["urlPattern"]; exists {
		pattern, ok := value.(string)
		if !ok {
			return fmt.Errorf("publicationRevisionNotice.urlPattern: expected string, got %T", value)
		}
		if pattern != "" {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("publicationRevisionNotice.urlPattern: %w", err)
			}
			cfg.urlRegexp = compiled
		}
	}
	if value, exists := notice["content"]; exists {
		content, err := parseRevisionNoticeContent(value)
		if err != nil {
			return err
		}
		cfg.content = content
	}
	if cfg.enabled {
		if cfg.urlRegexp == nil {
			return fmt.Errorf("publicationRevisionNotice.urlPattern: required when enabled")
		}
		if len(cfg.content) == 0 {
			return fmt.Errorf("publicationRevisionNotice.content: at least one language is required when enabled")
		}
	}
	revisionNoticeMu.Lock()
	revisionNotice, revisionNoticeSet = cfg, true
	revisionNoticeMu.Unlock()
	return nil
}

func parseRevisionNoticeContent(value any) (map[string]PublicationRevisionNoticeContent, error) {
	rawContent, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("publicationRevisionNotice.content: expected map, got %T", value)
	}
	content := make(map[string]PublicationRevisionNoticeContent, len(rawContent))
	for language, rawLocalized := range rawContent {
		localized, ok := rawLocalized.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("publicationRevisionNotice.content.%s: expected map, got %T", language, rawLocalized)
		}
		for key := range localized {
			switch key {
			case "ariaLabel", "title", "body":
			default:
				return nil, fmt.Errorf("publicationRevisionNotice.content.%s: unknown field %q", language, key)
			}
		}
		ariaLabel, err := requiredString(localized, "ariaLabel")
		if err != nil {
			return nil, fmt.Errorf("publicationRevisionNotice.content.%s.%w", language, err)
		}
		title, err := requiredString(localized, "title")
		if err != nil {
			return nil, fmt.Errorf("publicationRevisionNotice.content.%s.%w", language, err)
		}
		body, err := requiredString(localized, "body")
		if err != nil {
			return nil, fmt.Errorf("publicationRevisionNotice.content.%s.%w", language, err)
		}
		content[language] = PublicationRevisionNoticeContent{AriaLabel: ariaLabel, Title: title, Body: body}
	}
	return content, nil
}

func requiredString(values map[string]any, key string) (string, error) {
	value, ok := values[key]
	if !ok {
		return "", fmt.Errorf("%s: required", key)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s: expected string, got %T", key, value)
	}
	if text == "" {
		return "", fmt.Errorf("%s: must not be empty", key)
	}
	return text, nil
}

// publicationRevisionNotice returns the localized notice for a page when
// the feature is enabled, the URL matches the configured pattern, and
// copy exists for the page language. Nil otherwise (template `with` skips).
func publicationRevisionNotice(url, language string) *PublicationRevisionNoticeContent {
	revisionNoticeMu.RLock()
	defer revisionNoticeMu.RUnlock()
	if !revisionNoticeSet || !revisionNotice.enabled || revisionNotice.urlRegexp == nil || !revisionNotice.urlRegexp.MatchString(url) {
		return nil
	}
	content, ok := revisionNotice.content[language]
	if !ok {
		return nil
	}
	return &content
}

// ThemeFuncMap returns the site-theme template functions promoted from
// the zhurongshuo theme plugin. They are merged into FuncMap output so
// layouts/ can call them without any plugin loaded.
func ThemeFuncMap() template.FuncMap {
	return template.FuncMap{
		"readingTime":               readingTime,
		"parseGuideYAML":            parseGuideYAML,
		"svgTextWidth":              svgTextWidth,
		"svgTruncate":               svgTruncate,
		"svgWrap":                   svgWrap,
		"failRender":                failRender,
		"guideChartTypes":           guideChartTypesFn,
		"publicationRevisionNotice": publicationRevisionNotice,
	}
}
