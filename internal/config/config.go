package config

import (
	"html/template"
	"time"
)

// Config is the root configuration loaded from huan.yaml.
type Config struct {
	BaseURL      string            `yaml:"baseURL"`
	Title        string            `yaml:"title"`
	LanguageCode string            `yaml:"languageCode"`
	PublishDir   string            `yaml:"publishDir"`
	Paginate     int               `yaml:"paginate"`
	Minify       bool              `yaml:"minify"`
	HasCJKLang   bool              `yaml:"hasCJKLanguage"`
	SummaryLen   int               `yaml:"summaryLength"`
	EnableEmoji  bool              `yaml:"enableEmoji"`

	Author       AuthorConfig      `yaml:"author"`
	Params       ParamsConfig      `yaml:"params"`
	Menu         map[string][]MenuItem `yaml:"menu"`
	Social       []SocialItem      `yaml:"social"`
	Markup       MarkupConfig      `yaml:"markup"`
	Sitemap      SitemapConfig     `yaml:"sitemap"`
	RSS          RSSConfig         `yaml:"rss"`
	Outputs      OutputsConfig     `yaml:"outputs"`
	AI           AIConfig          `yaml:"ai"`
	Plugins      map[string]map[string]any `yaml:"plugins"`

	// Computed (not from YAML)
	BaseURLTemplate template.URL
	Services        ServicesConfig
}

// ServicesConfig mirrors Hugo's [services] section for template compatibility.
type ServicesConfig struct {
	RSS RSSConfig
	Disqus DisqusConfig
}

// DisqusConfig placeholder for Hugo compatibility.
type DisqusConfig struct {
	Shortname string
}

type AuthorConfig struct {
	Name string `yaml:"name"`
}

type ParamsConfig struct {
	SubTitle      string                       `yaml:"subTitle"`
	FooterSlogan  string                       `yaml:"footerSlogan"`
	Keywords      []string                     `yaml:"keywords"`
	Description   string                       `yaml:"description"`
	Copyrights    string                       `yaml:"copyrights"`
	EnableMathJax bool                         `yaml:"enableMathJax"`
	EnableSummary bool                         `yaml:"enableSummary"`
	MainSections  []string                     `yaml:"mainSections"`
	GoogleAnalytics string                     `yaml:"googleAnalytics"`
	CDNURL        string                       `yaml:"cdnURL"`

	Author        AuthorConfig                 `yaml:"author"`
}

type MenuItem struct {
	Name       string `yaml:"name"`
	Weight     int    `yaml:"weight"`
	Identifier string `yaml:"identifier"`
	URL        string `yaml:"url"`
}

type SocialItem struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

type MarkupConfig struct {
	Goldmark GoldmarkConfig `yaml:"goldmark"`
}

type GoldmarkConfig struct {
	Renderer   GoldmarkRendererConfig   `yaml:"renderer"`
	Extensions GoldmarkExtensionsConfig `yaml:"extensions"`
}

type GoldmarkRendererConfig struct {
	Unsafe bool `yaml:"unsafe"`
}

type GoldmarkExtensionsConfig struct {
	Typographer bool `yaml:"typographer"`
}

type SitemapConfig struct {
	ChangeFreq string  `yaml:"changefreq"`
	Filename   string  `yaml:"filename"`
	Priority   float64 `yaml:"priority"`
}

type RSSConfig struct {
	Limit int `yaml:"limit"`
}

type OutputsConfig struct {
	Home     []string `yaml:"home"`
	Page     []string `yaml:"page"`
	Section  []string `yaml:"section"`
	Taxonomy []string `yaml:"taxonomy"`
	Term     []string `yaml:"term"`
}

// AIConfig controls AI-friendly output features.
type AIConfig struct {
	LlmsTxt        bool `yaml:"llmsTxt"`
	ContentAPI     bool `yaml:"contentAPI"`
	MarkdownMirror bool `yaml:"markdownMirror"`
}

// BuildConfig mirrors Hugo's build frontmatter directive.
type BuildConfig struct {
	List             string `yaml:"list"`     // never, always
	Render           string `yaml:"render"`   // never, always
	PublishResources bool   `yaml:"publishResources"`
}

// CascadeConfig mirrors Hugo's cascade frontmatter directive.
type CascadeConfig struct {
	Build   BuildConfig       `yaml:"build"`
	Sitemap SitemapPageConfig `yaml:"sitemap"`
}

// SitemapPageConfig for per-page sitemap control.
type SitemapPageConfig struct {
	Disable    bool    `yaml:"disable"`
	ChangeFreq string  `yaml:"changefreq"`
	Priority   float64 `yaml:"priority"`
}

// Defaults returns a Config with sensible defaults.
func Defaults() *Config {
	return &Config{
		BaseURL:      "http://localhost:1313/",
		PublishDir:   "docs",
		Paginate:     10,
		Minify:       true,
		HasCJKLang:   true,
		SummaryLen:   120,
		LanguageCode: "zh-cn",
		Markup: MarkupConfig{
			Goldmark: GoldmarkConfig{
				Renderer: GoldmarkRendererConfig{Unsafe: true},
				Extensions: GoldmarkExtensionsConfig{Typographer: false},
			},
		},
		Sitemap: SitemapConfig{
			ChangeFreq: "weekly",
			Filename:   "sitemap.xml",
			Priority:   0.5,
		},
		RSS: RSSConfig{Limit: 20},
		Services: ServicesConfig{
			RSS: RSSConfig{Limit: 20},
		},
		Outputs: OutputsConfig{
			Home:     []string{"HTML", "RSS", "SearchIndex"},
			Page:     []string{"HTML"},
			Section:  []string{"HTML", "RSS"},
			Taxonomy: []string{"HTML", "RSS"},
			Term:     []string{"HTML", "RSS"},
		},
		AI: AIConfig{
			LlmsTxt:        true,
			ContentAPI:     true,
			MarkdownMirror: true,
		},
	}
}

// Now returns the current time, exposed to templates.
func (c *Config) Now() time.Time {
	return time.Now()
}
