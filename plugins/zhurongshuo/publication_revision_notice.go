package main

import (
	"fmt"
	"regexp"

	"github.com/iannil/huan/pkg/plugin"
)

// Config is the zhurongshuo theme configuration from huan.yaml.
type Config struct {
	PublicationRevisionNotice PublicationRevisionNoticeConfig
}

// PublicationRevisionNoticeConfig controls the optional revision notice.
type PublicationRevisionNoticeConfig struct {
	Enabled    bool
	URLPattern string
	Content    map[string]PublicationRevisionNoticeContent
	urlRegexp  *regexp.Regexp
}

// PublicationRevisionNoticeContent is the localized, user-visible notice copy.
type PublicationRevisionNoticeContent struct {
	AriaLabel string
	Title     string
	Body      string
}

// ParseConfig parses and validates the theme plugin configuration.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := &Config{}
	rawNotice, ok := raw["publicationRevisionNotice"]
	if !ok {
		return cfg, nil
	}
	notice, ok := rawNotice.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("publicationRevisionNotice: expected map, got %T", rawNotice)
	}

	for key := range notice {
		switch key {
		case "enabled", "urlPattern", "content":
		default:
			return nil, fmt.Errorf("publicationRevisionNotice: unknown field %q", key)
		}
	}

	if value, exists := notice["enabled"]; exists {
		enabled, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("publicationRevisionNotice.enabled: expected bool, got %T", value)
		}
		cfg.PublicationRevisionNotice.Enabled = enabled
	}
	if value, exists := notice["urlPattern"]; exists {
		pattern, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("publicationRevisionNotice.urlPattern: expected string, got %T", value)
		}
		cfg.PublicationRevisionNotice.URLPattern = pattern
		if pattern != "" {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("publicationRevisionNotice.urlPattern: %w", err)
			}
			cfg.PublicationRevisionNotice.urlRegexp = compiled
		}
	}
	if value, exists := notice["content"]; exists {
		content, err := parsePublicationRevisionNoticeContent(value)
		if err != nil {
			return nil, err
		}
		cfg.PublicationRevisionNotice.Content = content
	}

	if cfg.PublicationRevisionNotice.Enabled {
		if cfg.PublicationRevisionNotice.urlRegexp == nil {
			return nil, fmt.Errorf("publicationRevisionNotice.urlPattern: required when enabled")
		}
		if len(cfg.PublicationRevisionNotice.Content) == 0 {
			return nil, fmt.Errorf("publicationRevisionNotice.content: at least one language is required when enabled")
		}
	}
	return cfg, nil
}

func parsePublicationRevisionNoticeContent(value any) (map[string]PublicationRevisionNoticeContent, error) {
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

func (t *ZhurongshuoTheme) publicationRevisionNotice(url, language string) *PublicationRevisionNoticeContent {
	notice := &t.cfg.PublicationRevisionNotice
	if !notice.Enabled || notice.urlRegexp == nil || !notice.urlRegexp.MatchString(url) {
		return nil
	}
	content, ok := notice.Content[language]
	if !ok {
		return nil
	}
	return &content
}

// ConfigSchema declares the top-level plugin configuration shape.
func (t *ZhurongshuoTheme) ConfigSchema() plugin.Schema {
	return plugin.Schema{Fields: []plugin.FieldSchema{
		{Key: "publicationRevisionNotice", Type: "map", Required: false, Description: "URL-scoped localized publication revision notice"},
	}}
}
