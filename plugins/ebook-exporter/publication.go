package main

import (
	"bytes"
	"fmt"
	"image/png"
	"net/url"
	"os"
	"path/filepath"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
	"gopkg.in/yaml.v3"
)

func parsePublication(raw any) (content.Publication, error) {
	var p content.Publication
	if _, ok := raw.(map[string]any); !ok {
		return p, fmt.Errorf("publication: expected map, got %T", raw)
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return p, err
	}
	// YAML coercion accepts numbers as strings; inspect scalar types before decode.
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return p, err
	}
	var check func(*yaml.Node) error
	check = func(n *yaml.Node) error {
		if n.Kind == yaml.ScalarNode && n.Tag != "!!str" {
			return fmt.Errorf("publication: expected string, got %s", n.Tag)
		}
		for _, child := range n.Content {
			if err := check(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := check(&node); err != nil {
		return p, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&p); err != nil {
		return p, fmt.Errorf("publication: %w", err)
	}
	if p.BaseURL != "" {
		u, err := url.Parse(p.BaseURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			return p, fmt.Errorf("publication.base_url: expected absolute HTTP(S) URL without query or fragment")
		}
	}
	return p, nil
}

func loadPublication(sourceDir string, p content.Publication) (content.Publication, error) {
	if p.CoverBackground == "" {
		return p, nil
	}
	path := filepath.Clean(p.CoverBackground)
	if filepath.IsAbs(path) || path == ".." || !filepath.IsLocal(path) {
		return p, fmt.Errorf("publication.cover_background: expected site-relative PNG path")
	}
	data, err := os.ReadFile(filepath.Join(sourceDir, path))
	if err != nil {
		return p, fmt.Errorf("publication.cover_background: %w", err)
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		return p, fmt.Errorf("publication.cover_background: %w", err)
	}
	p.BackgroundPNG = data
	return p, nil
}
