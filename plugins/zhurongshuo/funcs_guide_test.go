package main

import (
	"strings"
	"testing"
)

func TestParseGuideYAMLExtractsGuideBlock(t *testing.T) {
	plain := "引入文字段落。\n\n```guide\nthesis:\n  claim: \"世界是被建构的\"\n  puzzle: |\n    为什么？\ntakeaways:\n  - \"第一点\"\n```\n\n后续段落。"
	g, err := parseGuideYAML(plain)
	if err != nil {
		t.Fatalf("parseGuideYAML: %v", err)
	}
	thesis, ok := g["thesis"].(map[string]interface{})
	if !ok {
		t.Fatalf("thesis not a map: %T", g["thesis"])
	}
	if thesis["claim"] != "世界是被建构的" {
		t.Errorf("claim = %v, want 世界是被建构的", thesis["claim"])
	}
	takeaways, ok := g["takeaways"].([]interface{})
	if !ok || len(takeaways) != 1 {
		t.Fatalf("takeaways = %v", g["takeaways"])
	}
}

func TestParseGuideYAMLMissingBlock(t *testing.T) {
	_, err := parseGuideYAML("没有代码块的正文")
	if err == nil {
		t.Fatal("expected error for missing ```guide block")
	}
	if !strings.Contains(err.Error(), "guide") {
		t.Errorf("error should mention guide block: %v", err)
	}
}

func TestParseGuideYAMLMalformedYAML(t *testing.T) {
	bad := "```guide\nthesis: [unclosed\n```"
	_, err := parseGuideYAML(bad)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}
