package content

import (
	"html/template"
	"time"

	"github.com/iannil/huan/internal/config"
)

// Page represents a single content page.
type Page struct {
	// Frontmatter fields
	Title         string   `yaml:"title"`
	Date          string   `yaml:"date"`
	Lastmod       string   `yaml:"lastmod"`
	Draft         bool     `yaml:"draft"`
	Hidden        bool     `yaml:"hidden"`
	Type          string   `yaml:"type"`
	Slug          string   `yaml:"slug"`
	Tags          []string `yaml:"tags"`
	Keywords      []string `yaml:"keywords"`
	Description   string   `yaml:"description"`
	Author        string   `yaml:"author"`
	Image         string   `yaml:"image"`
	FeaturedImage string   `yaml:"featured_image"`

	// Access control
	Access       string `yaml:"access"`
	EncryptGroup string `yaml:"encryptGroup"`
	EncryptMode  string `yaml:"encryptMode"`
	EncryptRatio int    `yaml:"encryptRatio"`

	// Hugo build directives
	Build   config.BuildConfig    `yaml:"build"`
	Cascade config.CascadeConfig  `yaml:"cascade"`
	Sitemap config.SitemapPageConfig `yaml:"sitemap"`

	// Computed fields (populated during loading)
	FilePath     string
	RelPath      string // relative to content/ e.g. "posts/2020/08/2601.md"
	URL          string // e.g. "/posts/2020/08/2601/"
	Section      string // e.g. "posts", "books", "gallery"
	Kind         string // "page", "section", "home", "taxonomy", "term"
	Content      template.HTML
	Summary      template.HTML
	Plain        string
	WordCount    int
	ReadingTime  int
	DateParsed   time.Time
	LastmodParsed time.Time
	Weight       int

	// Raw markdown body (before rendering)
	RawContent   string

	// Tree structure
	Parent               *Page
	Pages                []*Page
	RegularPages         []*Page
	RegularPagesRecursive []*Page // all descendant regular pages, recursively
	Sections             []*Page  // immediate child section pages
}

// IsHome returns true for the root index page.
func (p *Page) IsHome() bool {
	return p.Kind == "home"
}

// IsPage returns true for regular content pages.
func (p *Page) IsPage() bool {
	return p.Kind == "page"
}

// IsSection returns true for section index pages.
func (p *Page) IsSection() bool {
	return p.Kind == "section"
}

// Site holds all site-wide data.
type Site struct {
	Title        string
	BaseURL      string
	Language     string
	Params       map[string]interface{}
	Menus        map[string][]config.MenuItem
	Pages        []*Page
	RegularPages []*Page
	Data         map[string]interface{}
	Taxonomies   map[string]Taxonomy
	Config       *config.Config

	// Section index pages
	Sections map[string]*Page

	// TaxonomyOriginalCase maps plural (e.g. "tags") → {urlized-key → originalcased-name}.
	// Used to recover display casing for term-page titles (e.g. FANFAN) while
	// keeping the urlized key (e.g. fanfan) for filesystem paths and URL paths.
	TaxonomyOriginalCase map[string]map[string]string
}

// Taxonomy maps a tag name to its pages.
type Taxonomy map[string][]*Page

