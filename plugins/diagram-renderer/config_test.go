package main

import "testing"

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.Enabled {
		t.Errorf("Enabled default = true, want false")
	}
	if c.KrokiURL != "http://localhost:8000" {
		t.Errorf("KrokiURL = %q", c.KrokiURL)
	}
	if c.CacheDir != ".huan/cache/diagrams" {
		t.Errorf("CacheDir = %q", c.CacheDir)
	}
	if c.TimeoutMs != 5000 {
		t.Errorf("TimeoutMs = %d", c.TimeoutMs)
	}
	if c.FallbackMode != "client" {
		t.Errorf("FallbackMode = %q", c.FallbackMode)
	}
	if c.FigureClass != "diagram" {
		t.Errorf("FigureClass = %q", c.FigureClass)
	}
	want := []string{"mermaid", "plantuml", "graphviz", "d2"}
	if len(c.Languages) != len(want) {
		t.Fatalf("Languages = %v", c.Languages)
	}
	for i := range want {
		if c.Languages[i] != want[i] {
			t.Errorf("Languages[%d] = %q, want %q", i, c.Languages[i], want[i])
		}
	}
}

func TestParseConfigOverrides(t *testing.T) {
	raw := map[string]any{
		"enabled":      true,
		"krokiUrl":     "http://kroki:8000",
		"languages":    []any{"mermaid", "d2"},
		"timeoutMs":    float64(3000),
		"fallback":     map[string]any{"mode": "codeblock", "mermaidJs": "/js/mermaid.js"},
		"figureClass":  "chart",
		"excludeKinds": []any{"home"},
	}
	c, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if !c.Enabled || c.KrokiURL != "http://kroki:8000" || c.TimeoutMs != 3000 {
		t.Errorf("scalars not parsed: %+v", c)
	}
	if len(c.Languages) != 2 || c.Languages[1] != "d2" {
		t.Errorf("Languages = %v", c.Languages)
	}
	if c.FallbackMode != "codeblock" || c.MermaidJS != "/js/mermaid.js" {
		t.Errorf("fallback = %q %q", c.FallbackMode, c.MermaidJS)
	}
	if len(c.ExcludeKinds) != 1 || c.ExcludeKinds[0] != "home" {
		t.Errorf("ExcludeKinds = %v", c.ExcludeKinds)
	}
}

func TestParseConfigTypeError(t *testing.T) {
	if _, err := ParseConfig(map[string]any{"krokiUrl": 123}); err == nil {
		t.Errorf("expected type error for krokiUrl")
	}
}
