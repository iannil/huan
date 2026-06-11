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
	partialFunc := func(name string, ctx interface{}) (string, error) {
		t := tmplRef.Lookup("partials/" + name)
		if t == nil {
			return "", fmt.Errorf("partial not found: %s", name)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, ctx); err != nil {
			return "", err
		}
		return buf.String(), nil
	}

	// Add the partial function to funcMap
	tmpl.Funcs(template.FuncMap{
		"partial":       partialFunc,
		"partialCached": func(name string, ctx interface{}) (string, error) { return partialFunc(name, ctx) },
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
    <lastBuildDate>{{ site.RegularPages.ByLastmod.Reverse.First.Lastmod.Format "Mon, 02 Jan 2006 15:04:05 -0700" | safeHTML }}</lastBuildDate>{{ end }}
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

func (s *Scratch) Set(key string, value interface{})            { s.data[key] = value }
func (s *Scratch) Get(key string) interface{}                   { return s.data[key] }
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
		default:
			s.data[key] = append(ex, v)
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

// ToURLize converts a string to a URL-safe slug.
func ToURLize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	return s
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
