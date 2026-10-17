package template

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// Tests for the site-theme functions promoted from the zhurongshuo theme
// plugin (funcs_theme.go). Ported from the plugin's svg_layout_test.go,
// funcs_guide_test.go, and publication_revision_notice_test.go.

func TestSvgTextWidth(t *testing.T) {
	cn := svgTextWidth("实在建构", 16)
	if math.Abs(cn-64) > 0.01 { // 4 CJK chars × 16
		t.Errorf("CN width = %v, want 64", cn)
	}
	en := svgTextWidth("abcd", 16)
	if en <= 0 || en >= 64 { // ASCII narrower than CJK
		t.Errorf("EN width = %v, want in (0,64)", en)
	}
	mixed := svgTextWidth("实在ab", 16)
	want := 2*16.0 + svgTextWidth("ab", 16)
	if math.Abs(mixed-want) > 0.01 {
		t.Errorf("mixed = %v, want %v", mixed, want)
	}
}

func TestSvgTruncate(t *testing.T) {
	short := svgTruncate("短", 100, 16)
	if short != "短" {
		t.Errorf("short = %q, want unchanged", short)
	}
	long := svgTruncate("这是一个非常长的中文标签文本", 50, 16) // 13 chars × 16 = 208 > 50
	if !strings.HasSuffix(long, "…") {
		t.Errorf("long = %q, want ellipsis suffix", long)
	}
	if svgTextWidth(long, 16) > 50+16 { // ellipsis may slightly exceed
		t.Errorf("truncated width exceeds budget: %v", svgTextWidth(long, 16))
	}
}

func TestSvgWrap(t *testing.T) {
	lines := svgWrap("可能性基底与观察收敛", 64, 16) // 10 chars → 2+ 行
	if len(lines) < 2 {
		t.Fatalf("wrap lines = %d, want >= 2", len(lines))
	}
	for _, ln := range lines {
		if svgTextWidth(ln, 16) > 64+16 {
			t.Errorf("line %q too wide: %v", ln, svgTextWidth(ln, 16))
		}
	}
	// ASCII wraps on word boundaries
	enLines := svgWrap("Convergent Rationality Science", 80, 16)
	for _, ln := range enLines {
		if svgTextWidth(ln, 16) > 80+16 {
			t.Errorf("en line %q too wide", ln)
		}
	}
}

func TestSvgWrapWordBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width float64
		want  []string
	}{
		{"Bound the good result", 120, []string{"Bound the", "good result"}},
		{"Return feedback locally", 120, []string{"Return", "feedback", "locally"}},
		{"Keep reopening conditions", 120, []string{"Keep", "reopening", "conditions"}},
		{"  One   two  ", 120, []string{"One two"}},
		{"中文 abc def", 64, []string{"中文", "abc def"}},
	} {
		got := svgWrap(tc.text, tc.width, 16)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %q, want %q", tc.text, got, tc.want)
		}
	}
	for _, text := range []string{"UnbreakableLongIdentifier", "中文连续字符测试"} {
		lines := svgWrap(text, 64, 16)
		if strings.Join(lines, "") != text {
			t.Errorf("lost text: %q => %q", text, lines)
		}
		for _, line := range lines {
			if svgTextWidth(line, 16) > 64 {
				t.Errorf("overflow: %q", line)
			}
		}
	}
}

func TestReadingTime(t *testing.T) {
	// Pure CJK: 600 chars / 300 cpm = 2 分钟.
	cn := strings.Repeat("法", 600)
	if got := string(readingTime(cn)); got != "2 分钟" {
		t.Errorf("readingTime(CJK) = %q, want %q", got, "2 分钟")
	}
	// Pure English: 200 words / 200 wpm = 1 分钟 (min clamp).
	en := strings.Repeat("word ", 200)
	if got := string(readingTime(en)); got != "1 分钟" {
		t.Errorf("readingTime(EN) = %q, want %q", got, "1 分钟")
	}
	// Empty clamps to 1 分钟.
	if got := string(readingTime("")); got != "1 分钟" {
		t.Errorf("readingTime(empty) = %q, want %q", got, "1 分钟")
	}
}

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

func TestFailRenderReturnsError(t *testing.T) {
	if _, err := failRender("book not found"); err == nil {
		t.Fatal("failRender must return an error")
	}
}

func TestGuideChartTypesFn(t *testing.T) {
	m := map[string]bool{}
	for _, ct := range guideChartTypesFn() {
		m[ct] = true
	}
	for _, ct := range []string{"funnel", "ladder", "cycle", "layers", "flow", "network", "spectrum"} {
		if !m[ct] {
			t.Errorf("guideChartTypes missing %q", ct)
		}
	}
	if m["pie"] {
		t.Error("guideChartTypes should not contain unsupported type pie")
	}
}

func revisionNoticeTestConfig(enabled bool, pattern string) map[string]any {
	return map[string]any{
		"publicationRevisionNotice": map[string]any{
			"enabled":    enabled,
			"urlPattern": pattern,
			"content": map[string]any{
				"zh-cn": map[string]any{
					"ariaLabel": "状态",
					"title":     "RC预览版，非最终版本。",
					"body":      "中文说明。",
				},
				"en": map[string]any{
					"ariaLabel": "Status",
					"title":     "RC preview, not the final version.",
					"body":      "English notice.",
				},
			},
		},
	}
}

func TestSetPublicationRevisionNotice(t *testing.T) {
	if err := SetPublicationRevisionNotice(revisionNoticeTestConfig(true, `^/(?:en/)?books/`)); err != nil {
		t.Fatalf("SetPublicationRevisionNotice: %v", err)
	}
	defer func() { _ = SetPublicationRevisionNotice(nil) }()

	tests := []struct {
		name     string
		url      string
		language string
		wantNil  bool
	}{
		{name: "Chinese match", url: "/books/volume-1/book/", language: "zh-cn", wantNil: false},
		{name: "English match", url: "/en/books/volume-1/book/", language: "en", wantNil: false},
		{name: "URL mismatch", url: "/posts/example/", language: "zh-cn", wantNil: true},
		{name: "language missing", url: "/books/volume-1/book/", language: "fr", wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := publicationRevisionNotice(tt.url, tt.language)
			if (got == nil) != tt.wantNil {
				t.Fatalf("publicationRevisionNotice(%q, %q) = %v, wantNil %v", tt.url, tt.language, got, tt.wantNil)
			}
		})
	}
}

func TestSetPublicationRevisionNoticeDefaultsDisabled(t *testing.T) {
	if err := SetPublicationRevisionNotice(nil); err != nil {
		t.Fatalf("SetPublicationRevisionNotice(nil): %v", err)
	}
	if got := publicationRevisionNotice("/books/example/", "en"); got != nil {
		t.Fatalf("notice must default to disabled, got %v", got)
	}
	if err := SetPublicationRevisionNotice(map[string]any{}); err != nil {
		t.Fatalf("SetPublicationRevisionNotice(empty): %v", err)
	}
	if got := publicationRevisionNotice("/books/example/", "en"); got != nil {
		t.Fatalf("empty config must leave notice disabled, got %v", got)
	}
}

func TestSetPublicationRevisionNoticeRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
	}{
		{
			name: "unknown field",
			raw: map[string]any{
				"publicationRevisionNotice": map[string]any{"unknownField": true},
			},
		},
		{
			name: "enabled without pattern",
			raw: map[string]any{
				"publicationRevisionNotice": map[string]any{"enabled": true},
			},
		},
		{
			name: "bad regexp",
			raw: map[string]any{
				"publicationRevisionNotice": map[string]any{
					"enabled":    true,
					"urlPattern": "[invalid(",
					"content": map[string]any{
						"en": map[string]any{"ariaLabel": "a", "title": "t", "body": "b"},
					},
				},
			},
		},
		{
			name: "wrong type",
			raw: map[string]any{
				"publicationRevisionNotice": map[string]any{"enabled": "yes"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SetPublicationRevisionNotice(tt.raw); err == nil {
				t.Fatal("expected config validation error")
			}
		})
	}
	// Restore disabled state after error-path tests.
	_ = SetPublicationRevisionNotice(nil)
}

func TestThemeFuncMapContainsPromotedFuncs(t *testing.T) {
	fm := ThemeFuncMap()
	for _, name := range []string{
		"readingTime", "parseGuideYAML", "svgTextWidth", "svgTruncate",
		"svgWrap", "failRender", "guideChartTypes", "publicationRevisionNotice",
	} {
		if _, ok := fm[name]; !ok {
			t.Errorf("ThemeFuncMap missing %q", name)
		}
	}
}

func TestFuncMapMergesThemeFuncs(t *testing.T) {
	fm := FuncMap("https://example.com/")
	for _, name := range []string{
		"readingTime", "parseGuideYAML", "svgTextWidth", "svgTruncate",
		"svgWrap", "failRender", "guideChartTypes", "publicationRevisionNotice",
	} {
		if _, ok := fm[name]; !ok {
			t.Errorf("FuncMap missing promoted theme func %q", name)
		}
	}
}
