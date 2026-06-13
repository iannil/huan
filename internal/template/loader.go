package template

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Loader discovers and loads template files from a theme and local layouts.
type Loader struct {
	themeDir   string
	layoutsDir string
	funcMap    template.FuncMap
}

// tmplRef is the currently active template (set to the clone during Render).
// It's a package-level variable because the partial closure must reference it.
var tmplRef *template.Template

// SetActiveTemplate updates the active template reference used by the partial closure.
func SetActiveTemplate(t *template.Template) {
	tmplRef = t
}

// NewLoader creates a template loader.
func NewLoader(sourceDir, themeName string, funcMap template.FuncMap) *Loader {
	return &Loader{
		themeDir:   filepath.Join(sourceDir, "themes", themeName, "layouts"),
		layoutsDir: filepath.Join(sourceDir, "layouts"),
		funcMap:    funcMap,
	}
}

// LoadAll loads all templates and returns a ready-to-execute *template.Template.
func (l *Loader) LoadAll() (*template.Template, error) {
	// Collect template file contents: theme first, then local overrides
	templates := map[string]string{}

	if _, err := os.Stat(l.themeDir); err == nil {
		if err := l.walkDir(l.themeDir, templates); err != nil {
			return nil, fmt.Errorf("load theme: %w", err)
		}
	}

	if _, err := os.Stat(l.layoutsDir); err == nil {
		if err := l.walkDir(l.layoutsDir, templates); err != nil {
			return nil, fmt.Errorf("load layouts: %w", err)
		}
	}

	// Create the root template with all functions.
	// This tmpl is the factory: never Execute it directly. Always Clone() first.
	tmpl := template.New("").Funcs(l.funcMap)

	// partialFunc uses the package-level tmplRef, which Renderer sets to the
	// current clone before each Execute.
	partialFunc := func(name string, ctx interface{}) (template.HTML, error) {
		t := tmplRef.Lookup("partials/" + name)
		if t == nil {
			return "", fmt.Errorf("partial not found: %s", name)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, ctx); err != nil {
			return "", err
		}
		return template.HTML(buf.String()), nil
	}

	// Add the partial function to funcMap
	tmpl.Funcs(template.FuncMap{
		"partial":       partialFunc,
		"partialCached": func(name string, ctx interface{}) (template.HTML, error) { return partialFunc(name, ctx) },
		"site":          func() *SiteContext { return nil },
	})

	// Register Hugo internal templates as empty stubs (not used by this site).
	internalTemplates := map[string]string{
		"_internal/disqus.html":          `<!-- disqus disabled -->`,
		"_internal/google_analytics.html": ``,
		"_internal/google_analytics_async.html": ``,
		"_internal/opengraph.html":       ``,
		"_internal/schema.html":          ``,
		"_internal/twitter_cards.html":   ``,
	}

	// Override content-redact.html: in huan, content is pre-processed by the
	// encrypt engine before rendering, so the partial just outputs .Content.
	templates["partials/content-redact.html"] = `{{ .Content }}`

	// Override RSS template: Hugo's rss.xml does `$pctx := . ; if .IsHome { $pctx = .Site }`,
	// which requires .Site and . to be the same type. huan models Site and Page as
	// distinct types, so we rewrite the template to use site.RegularPages directly.
	// The output matches Hugo's: latest items in the channel.
	templates["_default/rss.xml"] = `{{- printf "<?xml version=\"1.0\" encoding=\"utf-8\" standalone=\"yes\"?>" | safeHTML }}
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>{{ if eq .Title .Site.Title }}{{ .Site.Title }}{{ else }}{{ with .Title }}{{ . }} on {{ end }}{{ .Site.Title }}{{ end }}</title>
    <link>{{ .Permalink }}</link>
    <description>Recent content {{ if ne .Title .Site.Title }}{{ with .Title }}in {{ . }} {{ end }}{{ end }}on {{ .Site.Title }}</description>
    <generator>Hugo</generator>
    <language>{{ site.Language.LanguageCode }}</language>{{ with .Site.Copyright }}
    <copyright>{{ . }}</copyright>{{ end }}{{ if not .Date.IsZero }}
    <lastBuildDate>{{ rssLastBuildDate . | safeHTML }}</lastBuildDate>{{ end }}
    {{- with .OutputFormats.Get "RSS" }}
    {{ printf "<atom:link href=%q rel=\"self\" type=%q />" .Permalink .MediaType.Type | safeHTML }}
    {{- end }}
    {{- $limit := .Site.Config.Services.RSS.Limit -}}
    {{- $pages := .RegularPages -}}
    {{- if ge $limit 1 -}}
    {{- $pages = $pages | first $limit -}}
    {{- end -}}
    {{- range $pages }}
    <item>
      <title>{{ .Title }}</title>
      <link>{{ .Permalink }}</link>
      <pubDate>{{ .PublishDate.Format "Mon, 02 Jan 2006 15:04:05 -0700" | safeHTML }}</pubDate>
      <guid>{{ .Permalink }}</guid>
      <description>{{ .Summary | transform_XMLEscape | safeHTML }}</description>
    </item>
    {{- end }}
  </channel>
</rss>`

	// Override sitemap.xml: Hugo's template iterates .Pages with sitemap filtering.
	// huan's SiteContext.Pages is a PageSlice that the template can range over.
	templates["_default/sitemap.xml"] = `{{ printf "<?xml version=\"1.0\" encoding=\"utf-8\" standalone=\"yes\"?>" | safeHTML }}
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"
  xmlns:xhtml="http://www.w3.org/1999/xhtml">
  {{ range where .Pages "Sitemap.Disable" "ne" true }}
    {{- if .Permalink -}}
  <url>
    <loc>{{ .Permalink }}</loc>{{ if not .Lastmod.IsZero }}
    <lastmod>{{ safeHTML ( .Lastmod.Format "2006-01-02T15:04:05-07:00" ) }}</lastmod>{{ end }}{{ with .Sitemap.ChangeFreq }}
    <changefreq>{{ . }}</changefreq>{{ end }}{{ if ge .Sitemap.Priority 0.0 }}
    <priority>{{ .Sitemap.Priority }}</priority>{{ end }}
  </url>
    {{- end -}}
  {{ end }}
</urlset>`

	// Parse all templates (pre-process dotted function names)
	for name, content := range templates {
		// Replace Hugo-style dotted function calls with underscored versions
		content = replaceDottedFuncs(content)
		if _, err := tmpl.New(name).Parse(content); err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
	}
	for name, content := range internalTemplates {
		if _, err := tmpl.New(name).Parse(content); err != nil {
			return nil, fmt.Errorf("parse internal template %s: %w", name, err)
		}
	}

	tmplRef = tmpl
	return tmpl, nil
}

func (l *Loader) walkDir(dir string, templates map[string]string) error {
	return filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".html" && ext != ".xml" && ext != ".json" {
			return nil
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relPath)

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		templates[name] = string(data)
		return nil
	})
}

// Scratch provides template-scoped mutable storage.
type Scratch struct {
	data map[string]interface{}
}

func NewScratch() *Scratch {
	return &Scratch{data: make(map[string]interface{})}
}

func (s *Scratch) Set(key string, value interface{}) interface{} { s.data[key] = value; return "" }
func (s *Scratch) Get(key string) interface{}                    { return s.data[key] }
func (s *Scratch) Add(key string, value interface{}) interface{} {
	existing, ok := s.data[key]
	if !ok {
		// First add: detect slice-like values to initialize as []interface{}
		switch sl := value.(type) {
		case []interface{}:
			s.data[key] = append([]interface{}{}, sl...)
		case PageSlice:
			s.data[key] = append([]interface{}{}, sl...)
		default:
			s.data[key] = value
		}
		return ""
	}

	// Existing value: append if both are slices
	switch ex := existing.(type) {
	case []interface{}:
		switch v := value.(type) {
		case []interface{}:
			s.data[key] = append(ex, v...)
		case PageSlice:
			s.data[key] = append(ex, v...)
		default:
			s.data[key] = append(ex, v)
		}
		return ""
	case PageSlice:
		switch v := value.(type) {
		case []interface{}:
			s.data[key] = append([]interface{}(ex), v...)
		case PageSlice:
			s.data[key] = append([]interface{}(ex), v...)
		default:
			s.data[key] = append([]interface{}(ex), v)
		}
		return ""
	}

	// Numeric/string addition
	switch v := value.(type) {
	case int:
		if e, ok := existing.(int); ok {
			s.data[key] = e + v
		}
	case float64:
		if e, ok := existing.(float64); ok {
			s.data[key] = e + v
		}
	case string:
		if e, ok := existing.(string); ok {
			s.data[key] = e + v
		}
	}
	return ""
}

// Map is a generic string-keyed map.
type Map map[string]interface{}

// ToURLize mirrors Hugo's urlize behavior:
//   - lowercase ASCII letters
//   - spaces become "-"
//   - CJK and other Unicode characters are URL-encoded with UPPERCASE hex
//     (matching Hugo's urlize output, e.g. 书稿 → %E4%B9%A6%E7%A8%BF)
//   - other special chars (parens, etc.) are URL-encoded with UPPERCASE hex
// Note: this differs from html/template's auto-URL-escaping (lowercase hex)
// used when templates emit raw CJK in href/src attributes.
func ToURLize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '/':
			b.WriteRune(r)
		default:
			// URL-encode all other characters (CJK, punctuation) with uppercase hex
			for _, c := range []byte(string(r)) {
				b.WriteString(percentEncode(c))
			}
		}
	}
	return b.String()
}

// percentEncode returns %XX for a byte, using uppercase hex.
func percentEncode(b byte) string {
	const hex = "0123456789ABCDEF"
	return string([]byte{'%', hex[b>>4], hex[b&0xF]})
}

// replaceDottedFuncs converts Hugo's dotted function calls to Go template compatible names.
// e.g., "crypto.MD5" → "crypto_MD5", "strings.RuneCount" → "strings_RuneCount"
var dottedFuncs = []struct{ from, to string }{
	{"crypto.MD5", "crypto_MD5"},
	{"strings.RuneCount", "strings_RuneCount"},
	{"strings.Repeat", "strings_Repeat"},
	{"strings.Split", "strings_Split"},
	{"strings.Contains", "strings_Contains"},
	{"strings.HasPrefix", "strings_HasPrefix"},
	{"strings.ToUpper", "strings_ToUpper"},
	{"strings.ToLower", "strings_ToLower"},
	{"strings.ReplaceRE", "strings_ReplaceRE"},
	{"strings.Replace", "strings_Replace"},
	{"strings.TrimSpace", "strings_TrimSpace"},
	{"path.Base", "path_Base"},
	{"path.Dir", "path_Dir"},
	{"reflect.IsMap", "reflect_IsMap"},
	{"reflect.IsSlice", "reflect_IsSlice"},
	{"transform.XMLEscape", "transform_XMLEscape"},
	{"lang.FormatNumberCustom", "lang_FormatNumberCustom"},
	{"os.Getenv", "os_Getenv"},
}

func replaceDottedFuncs(content string) string {
	for _, r := range dottedFuncs {
		content = strings.ReplaceAll(content, r.from, r.to)
	}
	return content
}
