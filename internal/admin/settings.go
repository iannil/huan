package admin

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// SiteSettings holds the Phase 1 config fields manageable from the admin UI.
// JSON tags use snake_case for the frontend API.
type SiteSettings struct {
	Title        string         `json:"title" yaml:"title"`
	EnableEmoji  bool           `json:"enableEmoji" yaml:"enableEmoji"`
	Minify       bool           `json:"minify" yaml:"minify"`
	Paginate     int            `json:"paginate" yaml:"paginate"`
	SummaryLen   int            `json:"summaryLength" yaml:"summaryLength"`
	Params       ParamsSettings `json:"params" yaml:"params"`
}

type ParamsSettings struct {
	SubTitle        string `json:"subTitle" yaml:"subTitle"`
	FooterSlogan    string `json:"footerSlogan" yaml:"footerSlogan"`
	Description     string `json:"description" yaml:"description"`
	Copyrights      string `json:"copyrights" yaml:"copyrights"`
	GoogleAnalytics string `json:"googleAnalytics" yaml:"googleAnalytics"`
	CDNURL          string `json:"cdnURL" yaml:"cdnURL"`
	EnableMathJax   bool   `json:"enableMathJax" yaml:"enableMathJax"`
	EnableSummary   bool   `json:"enableSummary" yaml:"enableSummary"`
}

// readSettings reads huan.yaml and returns the manageable subset as SiteSettings.
func readSettings(sourceDir string) (*SiteSettings, error) {
	path := filepath.Join(sourceDir, "huan.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read huan.yaml: %w", err)
	}
	var s SiteSettings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse site settings: %w", err)
	}
	return &s, nil
}

// updateSettings writes the given SiteSettings fields into huan.yaml using
// yaml.Node to preserve comments, formatting, and field order.
// Unknown fields in the YAML are left untouched.
func updateSettings(sourceDir string, s *SiteSettings) error {
	path := filepath.Join(sourceDir, "huan.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read huan.yaml: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse yaml node tree: %w", err)
	}

	root := doc.Content[0] // root mapping node

	// Top-level scalar fields
	setScalarField(root, []string{"title"}, s.Title)
	setScalarField(root, []string{"enableEmoji"}, s.EnableEmoji)
	setScalarField(root, []string{"minify"}, s.Minify)
	setScalarField(root, []string{"paginate"}, s.Paginate)
	setScalarField(root, []string{"summaryLength"}, s.SummaryLen)

	// Nested under params:
	setScalarField(root, []string{"params", "subTitle"}, s.Params.SubTitle)
	setScalarField(root, []string{"params", "footerSlogan"}, s.Params.FooterSlogan)
	setScalarField(root, []string{"params", "description"}, s.Params.Description)
	setScalarField(root, []string{"params", "copyrights"}, s.Params.Copyrights)
	setScalarField(root, []string{"params", "googleAnalytics"}, s.Params.GoogleAnalytics)
	setScalarField(root, []string{"params", "cdnURL"}, s.Params.CDNURL)
	setScalarField(root, []string{"params", "enableMathJax"}, s.Params.EnableMathJax)
	setScalarField(root, []string{"params", "enableSummary"}, s.Params.EnableSummary)

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&doc); err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	encoder.Close()

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write huan.yaml: %w", err)
	}
	return nil
}

// readSettingsYaml returns the raw text of huan.yaml for the YAML editor.
func readSettingsYaml(sourceDir string) (string, error) {
	path := filepath.Join(sourceDir, "huan.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read huan.yaml: %w", err)
	}
	return string(data), nil
}

// updateSettingsYaml validates and writes raw YAML text to huan.yaml.
func updateSettingsYaml(sourceDir string, content string) error {
	// Validate that the content is parseable YAML
	var check map[string]any
	if err := yaml.Unmarshal([]byte(content), &check); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	path := filepath.Join(sourceDir, "huan.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write huan.yaml: %w", err)
	}
	return nil
}

// --- yaml.Node helpers ---

// setScalarField walks the yaml.Node tree along path and sets the scalar value.
func setScalarField(root *yaml.Node, path []string, val interface{}) {
	node := root
	for _, key := range path[:len(path)-1] {
		node = findMappingEntry(node, key)
		if node == nil {
			return
		}
	}
	last := path[len(path)-1]
	target := findMappingEntry(node, last)
	if target == nil {
		return
	}
	setScalar(target, val)
}

// findMappingEntry finds the value node for a key in a mapping node.
// mapping.Content is a flat [key1, val1, key2, val2, ...] slice.
func findMappingEntry(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// setScalar replaces a scalar node's tag and value while preserving comments/line info.
func setScalar(n *yaml.Node, val interface{}) {
	n.Kind = yaml.ScalarNode
	switch v := val.(type) {
	case string:
		n.Tag = "!!str"
		n.Value = v
	case bool:
		n.Tag = "!!bool"
		if v {
			n.Value = "true"
		} else {
			n.Value = "false"
		}
	case int:
		n.Tag = "!!int"
		n.Value = strconv.Itoa(v)
	}
}
