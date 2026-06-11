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
		"plainify":    func(v interface{}) string { return stripTags(toString(v)) },
		"markdownify": func(s string) (string, error) { return s, nil }, // placeholder, will be replaced
		"jsonify":     jsonifyFunc,
		"printf":      fmt.Sprintf,
		"substr":      substrFunc,
		"default":     defaultFunc,
		"cond":        condFunc,

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

		// Math
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"div": func(a, b int) int { return a / b },
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
		"replace":      func(old, new, src string) string { return strings.ReplaceAll(src, old, new) },
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

	// If a field name is provided as the first arg, sort by that field.
	if len(args) > 0 && args[0] != "" {
		field := args[0]
		// Sort ascending by the field, using stable order.
		// Use Sprintf-based comparison so numbers and strings work.
		for i := 1; i < len(result); i++ {
			for j := i; j > 0; j-- {
				a := extractField(result[j], field)
				b := extractField(result[j-1], field)
				if compare(a, b) < 0 {
					result[j], result[j-1] = result[j-1], result[j]
				} else {
					break
				}
			}
		}
	}
	return result, nil
}

func indexFunc(m interface{}, keys ...string) (interface{}, error) {
	current := m
	for _, key := range keys {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[key]
			if !ok {
				return nil, nil
			}
		case map[interface{}]interface{}:
			var ok bool
			current, ok = v[key]
			if !ok {
				return nil, nil
			}
		default:
			return nil, nil
		}
	}
	return current, nil
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

func replaceREFunc(pattern, repl, src string) string {
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(src, repl)
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
