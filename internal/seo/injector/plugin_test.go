package injector

import (
	"strings"
	"testing"
)

func TestParseConfig_Default(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig(nil): %v", err)
	}
	if cfg.DescriptionMaxLength != 160 {
		t.Errorf("DescriptionMaxLength = %d, want 160", cfg.DescriptionMaxLength)
	}
	if !cfg.InjectOG {
		t.Error("InjectOG = false, want true")
	}
	if !cfg.InjectTwitter {
		t.Error("InjectTwitter = false, want true")
	}
}

func TestParseConfig_Overrides(t *testing.T) {
	raw := map[string]any{
		"descriptionMaxLength": 200,
		"defaultOGImage":       "/images/default.png",
		"injectOG":             false,
		"injectTwitter":        false,
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.DescriptionMaxLength != 200 {
		t.Errorf("DescriptionMaxLength = %d, want 200", cfg.DescriptionMaxLength)
	}
	if cfg.DefaultOGImage != "/images/default.png" {
		t.Errorf("DefaultOGImage = %q", cfg.DefaultOGImage)
	}
	if cfg.InjectOG {
		t.Error("InjectOG = true, want false")
	}
	if cfg.InjectTwitter {
		t.Error("InjectTwitter = true, want false")
	}
}

func TestInjectHTML_NoHead(t *testing.T) {
	src := "<html><body>no head</body></html>"
	result, err := InjectHTML(src, &InjectOptions{PageTitle: "test"})
	if err != nil {
		t.Fatalf("InjectHTML: %v", err)
	}
	if result != src {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestInjectHTML_AddsMissingTags(t *testing.T) {
	src := `<html><head><title>My Page</title></head><body><p>Hello world. This is a test description for SEO purposes.</p></body></html>`
	opts := &InjectOptions{
		PageTitle: "My Page",
		PageURL:   "https://example.com/page/",
		PageKind:  "page",
	}
	result, err := InjectHTML(src, opts)
	if err != nil {
		t.Fatalf("InjectHTML: %v", err)
	}

	// Verify key tags were injected
	checks := []string{
		`<meta name="description" content="`,
		`<meta property="og:title" content="My Page">`,
		`<meta property="og:url" content="https://example.com/page/">`,
		`<meta property="og:type" content="article">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<!-- huan seo-injector -->`,
	}
	for _, check := range checks {
		if !contains(result, check) {
			t.Errorf("missing injected tag: %s", check)
		}
	}
}

func TestInjectHTML_SkipsExistingTags(t *testing.T) {
	src := `<html><head><meta name="description" content="existing desc"></head><body><p>Content</p></body></html>`
	opts := &InjectOptions{
		PageTitle: "My Page",
	}
	result, err := InjectHTML(src, opts)
	if err != nil {
		t.Fatalf("InjectHTML: %v", err)
	}
	// Should keep only one description
	count := strings.Count(result, `name="description"`)
	if count != 1 {
		t.Errorf("expected 1 description, got %d: %s", count, result)
	}
}

func TestInjectHTML_OgTypeWebsite(t *testing.T) {
	src := `<html><head><title>Section</title></head><body><p>Content</p></body></html>`
	opts := &InjectOptions{
		PageTitle: "Section",
		PageKind:  "section",
	}
	result, err := InjectHTML(src, opts)
	if err != nil {
		t.Fatalf("InjectHTML: %v", err)
	}
	if !strings.Contains(result, `content="website"`) {
		t.Errorf("expected og:type website for section, got: %s", result)
	}
}

func TestExtractPlainText(t *testing.T) {
	html := `<html><head><title>Test</title></head><body><p>Hello world.</p><p>Second paragraph.</p></body></html>`
	text := ExtractPlainText(html)
	expected := "Hello world. Second paragraph."
	if text != expected {
		t.Errorf("got %q, want %q", text, expected)
	}
}

func TestExtractPlainText_SkipsNavStyles(t *testing.T) {
	html := `<html><body><nav>Nav links</nav><style>.foo{}</style><p>Real content</p><footer>Footer</footer></body></html>`
	text := ExtractPlainText(html)
	expected := "Real content"
	if text != expected {
		t.Errorf("got %q, want %q", text, expected)
	}
}

func TestTruncateToWordBoundary(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"Short text", 100, "Short text"},
		{"Hello world this is a test", 15, "Hello world"},
		{"Oneword", 3, "Oneword"},
		{"", 10, ""},
	}
	for _, tt := range tests {
		got := TruncateToWordBoundary(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("TruncateToWordBoundary(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestExtractExistingTags(t *testing.T) {
	html := `<html><head>
		<meta name="description" content="x">
		<meta property="og:title" content="y">
		<meta name="twitter:card" content="summary">
	</head></html>`
	tags := ExtractExistingTags(html)
	checks := []string{"description", "og:title", "twitter:card"}
	for _, c := range checks {
		if !tags[c] {
			t.Errorf("missing tag %q in extracted set", c)
		}
	}
}

// helpers
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
