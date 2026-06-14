package template

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// FuncMap returns all custom template functions needed to render the site.
func FuncMap(baseURL string) template.FuncMap {
	return template.FuncMap{
		// URL helpers
		"absURL":   func(s string) string { return baseURL + strings.TrimPrefix(s, "/") },
		"urlize":   ToURLize,
		"safeHTML": func(v interface{}) template.HTML { return template.HTML(toString(v)) },
		"safeJS":   func(v interface{}) template.JS { return template.JS(toString(v)) },
		"safeURL":  func(v interface{}) template.URL { return template.URL(toString(v)) },

		// Content helpers
		"plainify":    plainify,
		"markdownify": func(s string) (string, error) { return s, nil }, // placeholder, will be replaced
		"jsonify":     jsonifyFunc,
		"printf":      fmt.Sprintf,
		"substr":      substrFunc,
		"default":     defaultFunc,
		"cond":        condFunc,
		// rssLastBuildDate formats the most-recent Lastmod among ctx.RegularPages
		// using Hugo's RSS date layout. Returns "" when RegularPages is empty so
		// empty tag RSS produces <lastBuildDate></lastBuildDate> (compressed to
		// <lastBuildDate/>) matching Hugo's output for tags whose only pages
		// are hidden/never-listed.
		"rssLastBuildDate": RssLastBuildDate,

		// String operations
		"strings_RuneCount": utf8.RuneCountInString,
		"strings_Repeat":    strings.Repeat,
		"strings_Split":     strings.Split,
		"strings_Contains":  strings.Contains,
		"strings_HasPrefix": strings.HasPrefix,
		"strings_ToUpper":   strings.ToUpper,
		"strings_ToLower":   strings.ToLower,
		"strings_Replace":   strings.ReplaceAll,
		"strings_ReplaceRE": replaceREFunc,
		"strings_TrimSpace": strings.TrimSpace,
		"hasPrefix":         strings.HasPrefix,
		"lower":             strings.ToLower,
		"upper":             strings.ToUpper,
		"title":             strings.Title,
		"trimSpace":         strings.TrimSpace,

		// Crypto
		"crypto_MD5": func(s string) string {
			h := md5.Sum([]byte(s))
			return fmt.Sprintf("%x", h)
		},

		// Path
		"path_Base": filepath.Base,
		"path_Dir":  filepath.Dir,

		// Math. add/sub/mul/div coerce numeric args to float64 (matches Hugo
		// behavior: templates pass `{{ div $totalWords 10000.0 }}` where one
		// operand is a float literal). mod stays int — modulo is integer-only.
		"add": mathAdd,
		"sub": mathSub,
		"mul": mathMul,
		"div": mathDiv,
		"mod": func(a, b int) int { return a % b },

		// Comparison
		"ge": func(a, b interface{}) bool { return compare(a, b) >= 0 },
		"le": func(a, b interface{}) bool { return compare(a, b) <= 0 },
		"gt": func(a, b interface{}) bool { return compare(a, b) > 0 },
		"lt": func(a, b interface{}) bool { return compare(a, b) < 0 },
		"eq": func(a, b interface{}) bool { return compare(a, b) == 0 },
		"ne": func(a, b interface{}) bool { return compare(a, b) != 0 },

		// Collections
		"slice":   sliceFunc,
		"append":  appendSliceFunc,
		"first":   firstFunc,
		"last":    lastFunc,
		"where":   whereFunc,
		"sort":    sortFunc,
		"index":   indexFunc,
		"isset":   issetFunc,
		"in":      inFunc,
		"delimit": delimitFunc,
		"len":     lenFunc,
		"reverse": reverseFunc,
		"union":   unionFunc,

		// Scratch
		"newScratch": func() *Scratch { return NewScratch() },

		// Date
		"now":        func() string { return "" }, // placeholder
		"dateFormat": func(fmt, s string) string { return s },

		// Regex
		"replaceRE": replaceREFunc,
		"findRE":    findREFunc,

		// Misc
		"string":       func(v interface{}) string { return fmt.Sprintf("%v", v) },
		"int":          func(v interface{}) int { return toInt(v) },
		"echoParam":    echoParamFunc,
		"truncate":     truncateFunc,
		"dict":         dictFunc,
		"merge":        mergeFunc,
		"absLangURL":   func(s string) string { return baseURL + strings.TrimPrefix(s, "/") },
		"relLangURL":   func(s string) string { return s },
		"relURL":       func(s string) string { return s },
		"emojify":      func(s string) string { return s },
		"htmlEscape":   htmlEscapeFunc,
		"htmlUnescape": htmlUnescapeFunc,
		"highlight":    func(s, lang string) string { return s },
		"pluralize":    func(s string) string { return s + "s" },
		"singularize":  func(s string) string { return s },
		"humanize":     humanizeFunc,
		"print":        fmt.Sprint,
		"println":      fmt.Sprintln,
		"split":        strings.Split,
		"replace":      func(src, old, new string) string { return strings.ReplaceAll(src, old, new) },
		"trim":         strings.TrimSpace,
		"trimPrefix":   func(prefix, s string) string { return strings.TrimPrefix(s, prefix) },
		"trimSuffix":   func(suffix, s string) string { return strings.TrimSuffix(s, suffix) },
		"underscore":   func(s string) string { return strings.ReplaceAll(s, " ", "_") },

		// Hugo reflect / transform / lang (dotted, replaced at load time)
		"reflect_IsMap":           reflectIsMap,
		"reflect_IsSlice":         reflectIsSlice,
		"transform_XMLEscape":     func(v interface{}) string { return xmlEscapeFunc(toString(v)) },
		"lang_FormatNumberCustom": langFormatNumberCustom,
		"uniq":                    uniqFunc,
		"apply":                   func(fn interface{}, args ...interface{}) interface{} { return nil },
		"querify":                 querifyFunc,
		"getenv":                  func(s string) string { return os.Getenv(s) },
		"os_Getenv":               func(s string) string { return os.Getenv(s) },
		"time":                    timeParseFunc,
		"i18n":                    i18nFunc,
		"T":                       i18nFunc,
		"hreflang":                hreflangFunc,
		"langPrefix":              langPrefixFunc,
		"translationLinks":        translationLinksFunc,
		"safeCSS":                 func(v interface{}) template.CSS { return template.CSS(toString(v)) },
		"safeHTMLAttr":            func(v interface{}) template.HTMLAttr { return template.HTMLAttr(toString(v)) },
	}
}

// reflectField reads a named field from a struct using reflection.
func reflectField(item interface{}, name string) interface{} {
	v := reflect.ValueOf(item)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		return nil
	}
	return f.Interface()
}

// stripTags removes all HTML tags from a string.
func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(s, "")
}

// hugoNewLinePlaceholder mirrors Hugo's `tpl/template.go` constant. Used by
// plainify to preserve block-level element boundaries (`</p>`, `<br>`, `<br />`)
// as newlines through the tag-stripping step.
const hugoNewLinePlaceholder = "___hugonl_"

// stripHTMLReplacerPre is the pre-replacement applied before StripTags in Hugo's
// StripHTML: source `\n` becomes space (avoid being mistaken for block boundary),
// and `</p>` / `<br>` / `<br />` become placeholder (restored to `\n` after strip).
var stripHTMLReplacerPre = strings.NewReplacer(
	"\n", " ",
	"</p>", hugoNewLinePlaceholder,
	"<br>", hugoNewLinePlaceholder,
	"<br />", hugoNewLinePlaceholder,
)

// plainify strips HTML tags and produces output matching Hugo's StripHTML
// algorithm (tpl/template.go). The key behaviors:
//   - Source `\n` becomes space (so it isn't mistaken for a block boundary).
//   - `</p>` / `<br>` / `<br />` boundaries are preserved as `\n` via placeholder.
//   - Other tag boundaries (`<h2>`, `<div>`, etc.) get their surrounding source
//     `\n` converted to space (no placeholder).
//   - Consecutive runs of whitespace are deduped to a single char of the
//     leading type (`\n\n` → `\n`; `   ` → ` `; `\n ` → `\n`).
//   - Leading/trailing whitespace is preserved (no TrimSpace).
//
// Do NOT collapse-whitespace to a single space here — that breaks byte-level
// equivalence with Hugo for the `<meta name=description content="...">` use case.
//
// Returns template.HTML (not string) so the output bypasses Go template's
// auto-escape. This matches Hugo's plainify behavior — Hugo's plainify also
// returns safe HTML. Without this, `&quot;` (from goldmark body) becomes
// `&amp;quot;` after auto-escape, breaking description meta tag byte-parity
// (tdewolff minify can't normalize the double-escaped form back to ASCII `"`).
func plainify(v interface{}) template.HTML {
	s := toString(v)
	if !strings.ContainsAny(s, "<>") {
		return template.HTML(s)
	}

	pre := stripHTMLReplacerPre.Replace(s)
	preReplaced := pre != s

	s = stripTags(pre)

	if preReplaced {
		s = strings.ReplaceAll(s, hugoNewLinePlaceholder, "\n")
	}

	var wasSpace bool
	var buf strings.Builder
	for _, r := range s {
		isSpace := unicode.IsSpace(r)
		if !(isSpace && wasSpace) {
			buf.WriteRune(r)
		}
		wasSpace = isSpace
	}
	if buf.Len() > 0 {
		s = buf.String()
	}
	return template.HTML(s)
}

// toString converts any value to a string for template functions that expect strings.
func toString(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case template.HTML:
		return string(val)
	case template.JS:
		return string(val)
	case template.URL:
		return string(val)
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func reflectIsMap(v interface{}) bool {
	switch v.(type) {
	case map[string]interface{}, map[interface{}]interface{}:
		return true
	default:
		return false
	}
}

func reflectIsSlice(v interface{}) bool {
	switch v.(type) {
	case []interface{}, []string, []map[string]interface{}:
		return true
	default:
		return false
	}
}

func xmlEscapeFunc(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	).Replace(s)
}

// langFormatNumberCustom formats a number with thousands separator and given decimals.
// Hugo: lang.FormatNumberCustom <decimals> <number>
func langFormatNumberCustom(decimals int, num interface{}) string {
	var f float64
	switch v := num.(type) {
	case int:
		f = float64(v)
	case float64:
		f = v
	default:
		return ""
	}
	format := fmt.Sprintf("%%.%df", decimals)
	return fmt.Sprintf(format, f)
}

// RssLastBuildDate returns the RSS-formatted most-recent Lastmod among the
// context's RegularPages, or "" when RegularPages is empty. The empty string
// matches Hugo's output for "empty" tags (those whose only pages are hidden),
// where Hugo emits <lastBuildDate/> rather than a zero-time formatted date.
func RssLastBuildDate(ctx interface{}) string {
	c, ok := ctx.(*Context)
	if !ok || c == nil {
		return ""
	}
	if len(c.RegularPages) == 0 {
		return ""
	}
	var latest time.Time
	for _, item := range c.RegularPages {
		pc := AsCtx(item)
		if pc == nil {
			continue
		}
		if pc.Lastmod.After(latest) {
			latest = pc.Lastmod
		}
	}
	if latest.IsZero() {
		return ""
	}
	return latest.Format("Mon, 02 Jan 2006 15:04:05 -0700")
}

func uniqFunc(slice interface{}) []interface{} {
	items := toSlice(slice)
	seen := map[string]bool{}
	var result []interface{}
	for _, item := range items {
		key := fmt.Sprintf("%v", item)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	if result == nil {
		return []interface{}{}
	}
	return result
}

// querifyFunc builds a URL query string from key/value pairs.
func querifyFunc(args ...interface{}) (string, error) {
	if len(args)%2 != 0 {
		return "", fmt.Errorf("querify requires even number of arguments")
	}
	var parts []string
	for i := 0; i < len(args); i += 2 {
		parts = append(parts, fmt.Sprintf("%v=%v", args[i], args[i+1]))
	}
	return strings.Join(parts, "&"), nil
}

// TimeResult is what the time function returns.
type TimeResult struct {
	value time.Time
}

func (t *TimeResult) Format(layout string) string {
	return t.value.Format(layout)
}

func (t *TimeResult) Unix() int64 { return t.value.Unix() }

// timeParseFunc mirrors Hugo's time function - parses a string into a time object.
func timeParseFunc(args ...interface{}) (*TimeResult, error) {
	if len(args) == 0 {
		return &TimeResult{value: time.Now()}, nil
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("time expects string, got %T", args[0])
	}
	formats := []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return &TimeResult{value: t}, nil
		}
	}
	return nil, fmt.Errorf("cannot parse time: %s", s)
}

// i18nFunc returns the translated string for the key, or the key itself.
func i18nFunc(key string, args ...interface{}) string {
	if b := currentI18nBundle(); b != nil {
		return b.Translate(key, args...)
	}
	return key
}

// hreflangFunc emits <link rel="alternate" hreflang="..." href="..."> tags
// for all language variants of the current page. The current language's link
// is included; an additional hreflang="x-default" points to the default-lang URL.
//
// Usage in templates:
//
//	{{ hreflang . }}
//
// Output (for an English page on a zh-cn+en site):
//
//	<link rel="alternate" hreflang="zh-cn" href="https://example.com/posts/foo/">
//	<link rel="alternate" hreflang="en" href="https://example.com/en/posts/foo/">
//	<link rel="alternate" hreflang="x-default" href="https://example.com/posts/foo/">
//
// Returns empty string for single-language builds (no languages: block).
func hreflangFunc(ctx interface{}) template.HTML {
	c, ok := ctx.(*Context)
	if !ok || c == nil {
		return ""
	}
	if len(c.TranslationLinks) == 0 {
		return ""
	}
	// Resolve default language code via cfg (SiteContext.Config).
	var defaultLangCode string
	if c.Site != nil && c.Site.Config != nil {
		defaultLangCode = c.Site.Config.DefaultLanguageCode()
	}

	var b strings.Builder
	var defaultURL string
	for _, link := range c.TranslationLinks {
		// Emit one alternate link per language
		b.WriteString(`<link rel="alternate" hreflang="`)
		b.WriteString(link.Lang)
		b.WriteString(`" href="`)
		b.WriteString(link.URL)
		b.WriteString(`">`)
		// Track default-language URL for x-default
		if link.Lang == defaultLangCode {
			defaultURL = link.URL
		}
	}
	if defaultURL != "" {
		b.WriteString(`<link rel="alternate" hreflang="x-default" href="`)
		b.WriteString(defaultURL)
		b.WriteString(`">`)
	}
	return template.HTML(b.String())
}

// langPrefixFunc returns the current page's language URL prefix
// (e.g. "" for default language, "/en" for English subpath).
//
// Usage: <a href="{{ langPrefix . }}/posts/foo/">Foo</a>
func langPrefixFunc(ctx interface{}) string {
	c, ok := ctx.(*Context)
	if !ok || c == nil {
		return ""
	}
	return c.LanguagePrefix
}

// translationLinksFunc returns the list of TranslationLink for the current
// page, suitable for iterating in templates to render language switcher UI.
//
// Usage:
//
//	{{ range translationLinks . }}
//	  <a href="{{ .URL }}" {{ if .IsCurrent }}class="active"{{ end }}>{{ .LanguageName }}</a>
//	{{ end }}
func translationLinksFunc(ctx interface{}) []TranslationLink {
	c, ok := ctx.(*Context)
	if !ok || c == nil {
		return nil
	}
	return c.TranslationLinks
}

func jsonifyFunc(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func substrFunc(s string, start, end int) string {
	runes := []rune(s)
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start >= end {
		return ""
	}
	return string(runes[start:end])
}

func defaultFunc(def interface{}, val interface{}) interface{} {
	if val == nil || val == "" || val == false {
		return def
	}
	if s, ok := val.(string); ok && s == "" {
		return def
	}
	return val
}

func condFunc(cond bool, a, b interface{}) interface{} {
	if cond {
		return a
	}
	return b
}

func sliceFunc(args ...interface{}) PageSlice {
	// Return PageSlice so Go-template variables initialized via `slice` remain
	// chainable with .ByDate/.ByLastmod/.Reverse after later reassignment.
	return PageSlice(args)
}

func appendSliceFunc(args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("append requires at least 2 arguments")
	}

	// Find which arg is the slice. Accept []interface{} or PageSlice (named type).
	var slice []interface{}
	var isPageSlice bool
	var sliceIdx int
	for i, a := range args {
		if s, ok := a.([]interface{}); ok {
			slice = s
			sliceIdx = i
			break
		}
		if ps, ok := a.(PageSlice); ok {
			slice = []interface{}(ps)
			isPageSlice = true
			sliceIdx = i
			break
		}
	}
	if slice == nil {
		return nil, fmt.Errorf("no slice argument found")
	}

	result := append([]interface{}{}, slice...)
	for i, a := range args {
		if i == sliceIdx {
			continue
		}
		result = append(result, a)
	}

	if isPageSlice {
		return PageSlice(result), nil
	}
	return result, nil
}

func firstFunc(n int, s interface{}) (interface{}, error) {
	slice := toSlice(s)
	if n > len(slice) {
		n = len(slice)
	}
	if _, ok := s.(PageSlice); ok {
		return PageSlice(slice[:n]), nil
	}
	return slice[:n], nil
}

func lastFunc(n int, s interface{}) ([]interface{}, error) {
	slice := toSlice(s)
	if n > len(slice) {
		n = len(slice)
	}
	return slice[len(slice)-n:], nil
}

// whereFunc supports Hugo's where function with multiple forms:
//   where collection key value             - exact match
//   where collection key operator value    - operator: eq, ne, gt, lt, ge, le, in
//
// Returns the same type as the input when possible (PageSlice stays PageSlice),
// so templates can chain methods like .ByDate on the result.
func whereFunc(args ...interface{}) (interface{}, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("where needs at least 3 args")
	}

	originalCollection := args[0]
	collection := toSlice(originalCollection)
	key := fmt.Sprintf("%v", args[1])

	var matched []interface{}
	var err error
	if len(args) == 4 {
		op := fmt.Sprintf("%v", args[2])
		val := args[3]
		matched, err = filterWithOperator(collection, key, op, val)
	} else {
		val := args[2]
		for _, item := range collection {
			v := extractField(item, key)
			if v == nil {
				continue
			}
			if val == nil {
				matched = append(matched, item)
				continue
			}
			if fmt.Sprintf("%v", v) == fmt.Sprintf("%v", val) {
				matched = append(matched, item)
			}
		}
		if matched == nil {
			matched = []interface{}{}
		}
	}
	if err != nil {
		return nil, err
	}

	// Preserve PageSlice type so chained methods work
	if _, isPS := originalCollection.(PageSlice); isPS {
		result := make(PageSlice, 0, len(matched))
		for _, m := range matched {
			if ctx, ok := m.(*Context); ok {
				result = append(result, ctx)
			}
		}
		return result, nil
	}

	return matched, nil
}

// filterWithOperator applies the operator form of where.
func filterWithOperator(collection []interface{}, key, op string, val interface{}) ([]interface{}, error) {
	var result []interface{}
	for _, item := range collection {
		v := extractField(item, key)
		if v == nil {
			continue
		}
		if matchOperator(v, op, val) {
			result = append(result, item)
		}
	}
	if result == nil {
		return []interface{}{}, nil
	}
	return result, nil
}

// matchOperator returns true if value matches val under the given operator.
func matchOperator(value interface{}, op string, target interface{}) bool {
	switch op {
	case "eq", "=":
		return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", target)
	case "ne", "!=":
		return fmt.Sprintf("%v", value) != fmt.Sprintf("%v", target)
	case "gt", ">":
		return compare(value, target) > 0
	case "lt", "<":
		return compare(value, target) < 0
	case "ge", ">=":
		return compare(value, target) >= 0
	case "le", "<=":
		return compare(value, target) <= 0
	case "in":
		items := toSlice(target)
		for _, it := range items {
			if fmt.Sprintf("%v", it) == fmt.Sprintf("%v", value) {
				return true
			}
		}
		return false
	}
	return false
}

// extractField retrieves a (possibly dotted) field from a struct or map.
// e.g., extractField(ctx, "Sitemap.Disable") walks Sitemap → Disable.
func extractField(item interface{}, key string) interface{} {
	parts := strings.Split(key, ".")
	current := item
	for _, p := range parts {
		current = readField(current, p)
		if current == nil {
			return nil
		}
	}
	return current
}

// readField reads a single field from a struct or map.
func readField(item interface{}, name string) interface{} {
	if item == nil {
		return nil
	}
	switch v := item.(type) {
	case map[string]interface{}:
		if val, ok := v[name]; ok {
			return val
		}
		return nil
	case map[interface{}]interface{}:
		if val, ok := v[name]; ok {
			return val
		}
		return nil
	default:
		// Use reflection on struct
		return reflectField(item, name)
	}
}

func sortFunc(slice interface{}, args ...string) ([]interface{}, error) {
	items := toSlice(slice)
	if len(items) <= 1 {
		return items, nil
	}
	// Make a copy to avoid mutating input
	result := make([]interface{}, len(items))
	copy(result, items)

	// Sort key: if a field name is given as the first arg, sort by that field;
	// otherwise (Hugo's `sort $seq` without a field), sort by the element
	// itself. Without this branch, huan returned the slice unchanged whenever
	// no field was provided, which broke book/practice list page part ordering
	// (templates use `sort ($scratch.Get "partSlugs")` with no field arg).
	keyOf := func(v interface{}) interface{} { return v }
	if len(args) > 0 && args[0] != "" {
		field := args[0]
		keyOf = func(v interface{}) interface{} { return extractField(v, field) }
	}
	// Insertion sort (stable). compare() handles mixed string/number cases
	// via fmt.Sprintf, matching Hugo's loose-typing sort behavior.
	for i := 1; i < len(result); i++ {
		for j := i; j > 0; j-- {
			a := keyOf(result[j])
			b := keyOf(result[j-1])
			if compare(a, b) < 0 {
				result[j], result[j-1] = result[j-1], result[j]
			} else {
				break
			}
		}
	}
	return result, nil
}

func indexFunc(m interface{}, keys ...interface{}) (interface{}, error) {
	current := m
	for _, key := range keys {
		switch v := current.(type) {
		case map[string]interface{}:
			k := fmt.Sprintf("%v", key)
			var ok bool
			current, ok = v[k]
			if !ok {
				return nil, nil
			}
		case map[interface{}]interface{}:
			var ok bool
			// Try direct key match first (handles non-string keys in yaml maps).
			current, ok = v[key]
			if !ok {
				// Fall back to stringified key.
				current, ok = v[fmt.Sprintf("%v", key)]
			}
			if !ok {
				return nil, nil
			}
		case []interface{}:
			idx, ok := toIntOK(key)
			if !ok || idx < 0 || idx >= len(v) {
				return nil, nil
			}
			current = v[idx]
		case []string:
			idx, ok := toIntOK(key)
			if !ok || idx < 0 || idx >= len(v) {
				return nil, nil
			}
			current = v[idx]
		case PageSlice:
			idx, ok := toIntOK(key)
			if !ok || idx < 0 || idx >= len(v) {
				return nil, nil
			}
			current = v[idx]
		default:
			return nil, nil
		}
	}
	return current, nil
}

// toIntOK tries to convert v to int. Returns (int, true) on success.
func toIntOK(v interface{}) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case string:
		var i int
		if n, err := fmt.Sscanf(x, "%d", &i); n == 1 && err == nil {
			return i, true
		}
	}
	return 0, false
}

func issetFunc(m interface{}, key string) bool {
	switch v := m.(type) {
	case map[string]interface{}:
		_, ok := v[key]
		return ok
	default:
		return false
	}
}

func inFunc(slice interface{}, val interface{}) bool {
	items := toSlice(slice)
	for _, item := range items {
		if fmt.Sprintf("%v", item) == fmt.Sprintf("%v", val) {
			return true
		}
	}
	return false
}

func delimitFunc(slice interface{}, sep interface{}, last ...interface{}) string {
	items := toSlice(slice)
	sepStr := fmt.Sprintf("%v", sep)
	strs := make([]string, len(items))
	for i, item := range items {
		strs[i] = fmt.Sprintf("%v", item)
	}
	if len(last) > 0 && len(strs) > 1 {
		return strings.Join(strs[:len(strs)-1], sepStr) + fmt.Sprintf("%v", last[0]) + strs[len(strs)-1]
	}
	return strings.Join(strs, sepStr)
}

func lenFunc(v interface{}) int {
	switch val := v.(type) {
	case string:
		return len(val)
	case []interface{}:
		return len(val)
	case []string:
		return len(val)
	case map[string]interface{}:
		return len(val)
	case TaxonomyContext:
		return len(val)
	case int:
		return val
	default:
		// Reflection fallback for typed slices/maps
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
			return rv.Len()
		}
		return 0
	}
}

func reverseFunc(slice interface{}) []interface{} {
	items := toSlice(slice)
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items
}

func unionFunc(slices ...interface{}) []interface{} {
	var result []interface{}
	for _, s := range slices {
		result = append(result, toSlice(s)...)
	}
	return result
}

func replaceREFunc(pattern, repl interface{}, src interface{}) string {
	// Accept interface{} because Hugo's plainify returns template.HTML, and
	// templates like `{{ $x | plainify | replaceRE ... }}` pipe HTML-typed
	// values here. With strict string params, Go template errors:
	//   "wrong type for value; expected string; got template.HTML"
	p := toString(pattern)
	r := toString(repl)
	s := toString(src)
	re := regexp.MustCompile(p)
	return re.ReplaceAllString(s, r)
}

func findREFunc(pattern, src string) []string {
	re := regexp.MustCompile(pattern)
	return re.FindAllString(src, -1)
}

func echoParamFunc(m interface{}, key string) interface{} {
	switch v := m.(type) {
	case map[string]interface{}:
		if val, ok := v[key]; ok {
			return val
		}
	}
	return ""
}

func truncateFunc(length int, s string) string {
	runes := []rune(s)
	if len(runes) <= length {
		return s
	}
	return string(runes[:length]) + "…"
}

func dictFunc(args ...interface{}) (map[string]interface{}, error) {
	if len(args)%2 != 0 {
		return nil, fmt.Errorf("dict requires even number of arguments")
	}
	m := make(map[string]interface{})
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings")
		}
		m[key] = args[i+1]
	}
	return m, nil
}

// mergeFunc merges multiple maps. Later maps override earlier ones.
func mergeFunc(args ...interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for _, arg := range args {
		switch v := arg.(type) {
		case map[string]interface{}:
			for k, val := range v {
				result[k] = val
			}
		case map[interface{}]interface{}:
			for k, val := range v {
				result[fmt.Sprintf("%v", k)] = val
			}
		default:
			return nil, fmt.Errorf("merge expects maps, got %T", arg)
		}
	}
	return result, nil
}

func htmlEscapeFunc(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	).Replace(s)
}

func htmlUnescapeFunc(s string) string {
	return strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&#34;", `"`,
		"&#39;", "'",
	).Replace(s)
}

func humanizeFunc(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	if runes[0] >= 'a' && runes[0] <= 'z' {
		runes[0] = runes[0] - 32
	}
	return string(runes)
}

// Helper functions

func toSlice(v interface{}) []interface{} {
	switch val := v.(type) {
	case []interface{}:
		return val
	case []string:
		result := make([]interface{}, len(val))
		for i, s := range val {
			result[i] = s
		}
		return result
	case []*interface{}:
		return *new([]interface{})
	case PageSlice:
		// []*Context → []interface{}
		result := make([]interface{}, len(val))
		for i, c := range val {
			result[i] = c
		}
		return result
	case []*Context:
		result := make([]interface{}, len(val))
		for i, c := range val {
			result[i] = c
		}
		return result
	default:
		// Reflection-based fallback for typed slices
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			result := make([]interface{}, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				result[i] = rv.Index(i).Interface()
			}
			return result
		}
		return nil
	}
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case string:
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	default:
		return 0
	}
}

// toFloat64 coerces numeric types to float64. Matches Hugo template math
// behavior, where operands of add/sub/mul/div may be int, int64, float32 or
// float64. Returns (0, false) for non-numeric values so callers can decide
// how to surface the error (templates typically render "<no value>").
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// mathAdd/sub/mul/div implement Hugo-style template arithmetic with float64
// coercion. Both operands are coerced; result is always float64 (e.g.
// `div 10 4` yields 2.5, not 2 — matching Hugo semantics).
func mathAdd(a, b interface{}) interface{} {
	af, _ := toFloat64(a)
	bf, _ := toFloat64(b)
	return af + bf
}

func mathSub(a, b interface{}) interface{} {
	af, _ := toFloat64(a)
	bf, _ := toFloat64(b)
	return af - bf
}

func mathMul(a, b interface{}) interface{} {
	af, _ := toFloat64(a)
	bf, _ := toFloat64(b)
	return af * bf
}

func mathDiv(a, b interface{}) interface{} {
	af, _ := toFloat64(a)
	bf, _ := toFloat64(b)
	if bf == 0 {
		return 0.0
	}
	return af / bf
}

func compare(a, b interface{}) int {
	// Simple string-based comparison
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	switch {
	case as < bs:
		return -1
	case as > bs:
		return 1
	default:
		return 0
	}
}
