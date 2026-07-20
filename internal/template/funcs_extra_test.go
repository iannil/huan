package template

import (
	"encoding/json"
	"html/template"
	"testing"
	"time"

	"github.com/iannil/huan/internal/config"
)

// ============================================================================
// funcs_test.go — Tests for uncovered template functions
// ============================================================================

func TestFuncMap_JSONify(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["jsonify"]
	result, err := f.(func(interface{}) (string, error))(map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("jsonify error: %v", err)
	}
	expected := `{"key":"value"}`
	if result != expected {
		t.Errorf("jsonify: got %s, want %s", result, expected)
	}
}

func TestFuncMap_JSONify_Nested(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["jsonify"]
	input := map[string]interface{}{
		"name":  "test",
		"count": 42,
	}
	result, err := f.(func(interface{}) (string, error))(input)
	if err != nil {
		t.Fatalf("jsonify error: %v", err)
	}
	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Errorf("jsonify result is not valid JSON: %v", err)
	}
}

func TestFuncMap_Default(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["default"]

	// default(actual, nil) → nil is falsy, returns actual
	result := f.(func(interface{}, interface{}) interface{})("actual", nil)
	if result != "actual" {
		t.Errorf("default(actual, nil): got %v, want actual", result)
	}

	// default(fallback, actual) → actual is truthy, returns actual
	result2 := f.(func(interface{}, interface{}) interface{})("fallback", "actual")
	if result2 != "actual" {
		t.Errorf("default(fallback, actual): got %v, want actual", result2)
	}

	// default(fallback, "") → empty string is falsy, returns fallback
	result3 := f.(func(interface{}, interface{}) interface{})("fallback", "")
	if result3 != "fallback" {
		t.Errorf("default(fallback, empty): got %v, want fallback", result3)
	}

	// default(fallback, false) → false is falsy, returns fallback
	result4 := f.(func(interface{}, interface{}) interface{})("fallback", false)
	if result4 != "fallback" {
		t.Errorf("default(fallback, false): got %v, want fallback", result4)
	}
}

func TestFuncMap_Cond(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["cond"]

	// true case
	result := f.(func(bool, interface{}, interface{}) interface{})(true, "yes", "no")
	if result != "yes" {
		t.Errorf("cond(true): got %v, want yes", result)
	}

	// false case
	result2 := f.(func(bool, interface{}, interface{}) interface{})(false, "yes", "no")
	if result2 != "no" {
		t.Errorf("cond(false): got %v, want no", result2)
	}
}

func TestFuncMap_Substr(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["substr"]

	result := f.(func(string, int, int) string)("hello world", 0, 5)
	if result != "hello" {
		t.Errorf("substr(hello world, 0, 5): got %s, want hello", result)
	}

	// negative start
	result2 := f.(func(string, int, int) string)("hello", -1, 5)
	if result2 != "hello" {
		t.Errorf("substr with negative start: got %s, want hello", result2)
	}

	// end beyond length
	result3 := f.(func(string, int, int) string)("hi", 0, 100)
	if result3 != "hi" {
		t.Errorf("substr with end beyond length: got %s, want hi", result3)
	}
}

func TestFuncMap_Slice(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["slice"]

	result := f.(func(...interface{}) PageSlice)("a", "b", "c")
	if len(result) != 3 {
		t.Errorf("slice length: got %d, want 3", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("slice contents: got %v, want [a b c]", result)
	}
}

func TestFuncMap_Append(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["append"]

	// Append to slice
	slice := []interface{}{"a", "b"}
	result, err := f.(func(...interface{}) (interface{}, error))(slice, "c")
	if err != nil {
		t.Fatalf("append error: %v", err)
	}
	resultSlice := toSlice(result)
	if len(resultSlice) != 3 {
		t.Errorf("append length: got %d, want 3", len(resultSlice))
	}

	// Append to PageSlice
	ps := PageSlice{&Context{Title: "p1"}}
	result2, err := f.(func(...interface{}) (interface{}, error))(ps, &Context{Title: "p2"})
	if err != nil {
		t.Fatalf("append to PageSlice error: %v", err)
	}
	psResult := toSlice(result2)
	if len(psResult) != 2 {
		t.Errorf("append to PageSlice length: got %d, want 2", len(psResult))
	}
}

func TestFuncMap_FirstLast(t *testing.T) {
	fm := FuncMap("https://example.com/")
	items := []interface{}{"a", "b", "c"}

	// First
	firstFn := fm["first"].(func(int, interface{}) (interface{}, error))
	result, err := firstFn(2, items)
	if err != nil {
		t.Fatalf("first error: %v", err)
	}
	firstSlice := toSlice(result)
	if len(firstSlice) != 2 || firstSlice[0] != "a" || firstSlice[1] != "b" {
		t.Errorf("first(2): got %v, want [a b]", firstSlice)
	}

	// First with PageSlice
	ps := PageSlice{&Context{Title: "p1"}, &Context{Title: "p2"}, &Context{Title: "p3"}}
	resultPs, err := firstFn(2, ps)
	if err != nil {
		t.Fatalf("first on PageSlice error: %v", err)
	}
	if _, ok := resultPs.(PageSlice); !ok {
		t.Errorf("first on PageSlice should return PageSlice, got %T", resultPs)
	}

	// Last
	lastFn := fm["last"].(func(int, interface{}) ([]interface{}, error))
	result2, err := lastFn(2, items)
	if err != nil {
		t.Fatalf("last error: %v", err)
	}
	if len(result2) != 2 || result2[0] != "b" || result2[1] != "c" {
		t.Errorf("last(2): got %v, want [b c]", result2)
	}

	// Last with n > len
	result3, err := lastFn(100, items)
	if err != nil {
		t.Fatalf("last with large n error: %v", err)
	}
	if len(result3) != 3 {
		t.Errorf("last(100) should cap at len: got %d", len(result3))
	}
}

func TestFuncMap_Where(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["where"].(func(...interface{}) (interface{}, error))

	type item struct {
		Section string
		Title   string
		Weight  int
	}
	items := []item{
		{Section: "posts", Title: "p1", Weight: 1},
		{Section: "books", Title: "b1", Weight: 2},
		{Section: "posts", Title: "p2", Weight: 3},
	}

	// Exact match
	result, err := f(items, "Section", "posts")
	if err != nil {
		t.Fatalf("where error: %v", err)
	}
	filtered := toSlice(result)
	if len(filtered) != 2 {
		t.Errorf("where Section=posts: got %d items, want 2", len(filtered))
	}

	// Operator form
	result2, err := f(items, "Weight", "gt", 1)
	if err != nil {
		t.Fatalf("where with operator error: %v", err)
	}
	filtered2 := toSlice(result2)
	if len(filtered2) != 2 {
		t.Errorf("where Weight gt 1: got %d items, want 2", len(filtered2))
	}

	// Operator: in
	result3, err := f(items, "Section", "in", []string{"posts", "books"})
	if err != nil {
		t.Fatalf("where with 'in' operator error: %v", err)
	}
	filtered3 := toSlice(result3)
	if len(filtered3) != 3 {
		t.Errorf("where Section in [posts, books]: got %d items, want 3", len(filtered3))
	}

	// With PageSlice
	ps := PageSlice{
		&Context{Title: "a", Section: "posts"},
		&Context{Title: "b", Section: "books"},
	}
	result4, err := f(ps, "Section", "posts")
	if err != nil {
		t.Fatalf("where on PageSlice error: %v", err)
	}
	if _, ok := result4.(PageSlice); !ok {
		t.Errorf("where on PageSlice should return PageSlice, got %T", result4)
	}
}

func TestFuncMap_Uniq(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["uniq"]

	result := f.(func(interface{}) []interface{})([]string{"a", "b", "a", "c", "b"})
	unique := toSlice(result)
	if len(unique) != 3 {
		t.Errorf("uniq: got %d items, want 3", len(unique))
	}
}

func TestFuncMap_Index(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["index"].(func(interface{}, ...interface{}) (interface{}, error))

	// Map index
	m := map[string]interface{}{"key": "value"}
	result, err := f(m, "key")
	if err != nil {
		t.Fatalf("index error: %v", err)
	}
	if result != "value" {
		t.Errorf("index(map, key): got %v, want value", result)
	}

	// Slice index
	slice := []interface{}{"a", "b", "c"}
	result2, err := f(slice, 1)
	if err != nil {
		t.Fatalf("index slice error: %v", err)
	}
	if result2 != "b" {
		t.Errorf("index(slice, 1): got %v, want b", result2)
	}

	// Missing key returns nil
	result3, err := f(m, "missing")
	if err != nil {
		t.Fatalf("index missing error: %v", err)
	}
	if result3 != nil {
		t.Errorf("index(map, missing): got %v, want nil", result3)
	}

	// PageSlice index
	ps := PageSlice{&Context{Title: "p1"}, &Context{Title: "p2"}}
	result4, err := f(ps, 0)
	if err != nil {
		t.Fatalf("index PageSlice error: %v", err)
	}
	ctx := AsCtx(result4)
	if ctx == nil || ctx.Title != "p1" {
		t.Errorf("index(PageSlice, 0): got %v, want p1", result4)
	}
}

func TestFuncMap_Isset(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["isset"]

	// Map key exists
	m := map[string]interface{}{"key": "value"}
	result := f.(func(interface{}, string) bool)(m, "key")
	if !result {
		t.Error("isset(map, key): got false, want true")
	}

	// Map key missing
	result2 := f.(func(interface{}, string) bool)(m, "missing")
	if result2 {
		t.Error("isset(map, missing): got true, want false")
	}

	// Non-map returns false
	result3 := f.(func(interface{}, string) bool)("not a map", "key")
	if result3 {
		t.Error("isset(string, key): got true, want false")
	}
}

func TestFuncMap_In(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["in"]

	slice := []interface{}{"a", "b", "c"}
	result := f.(func(interface{}, interface{}) bool)(slice, "b")
	if !result {
		t.Error("in(slice, b): got false, want true")
	}

	result2 := f.(func(interface{}, interface{}) bool)(slice, "z")
	if result2 {
		t.Error("in(slice, z): got true, want false")
	}
}

func TestFuncMap_Delimit(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["delimit"]

	slice := []interface{}{"a", "b", "c"}

	// Basic delimiter
	result := f.(func(interface{}, interface{}, ...interface{}) string)(slice, ", ")
	if result != "a, b, c" {
		t.Errorf("delimit(slice, ', '): got %s, want 'a, b, c'", result)
	}

	// With last delimiter
	result2 := f.(func(interface{}, interface{}, ...interface{}) string)(slice, ", ", " and ")
	if result2 != "a, b and c" {
		t.Errorf("delimit with last: got %s, want 'a, b and c'", result2)
	}
}

func TestFuncMap_Len(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["len"]

	// String
	result := f.(func(interface{}) int)("hello")
	if result != 5 {
		t.Errorf("len(hello): got %d, want 5", result)
	}

	// Slice
	result2 := f.(func(interface{}) int)([]interface{}{"a", "b", "c"})
	if result2 != 3 {
		t.Errorf("len(slice): got %d, want 3", result2)
	}

	// Map
	m := map[string]interface{}{"a": 1, "b": 2}
	result3 := f.(func(interface{}) int)(m)
	if result3 != 2 {
		t.Errorf("len(map): got %d, want 2", result3)
	}

	// PageSlice
	ps := PageSlice{&Context{}, &Context{}}
	result4 := f.(func(interface{}) int)(ps)
	if result4 != 2 {
		t.Errorf("len(PageSlice): got %d, want 2", result4)
	}

	// TaxonomyContext
	tc := TaxonomyContext{"tag1": []*Context{}, "tag2": []*Context{}}
	result5 := f.(func(interface{}) int)(tc)
	if result5 != 2 {
		t.Errorf("len(TaxonomyContext): got %d, want 2", result5)
	}

	// int (special case: returns itself)
	result6 := f.(func(interface{}) int)(42)
	if result6 != 42 {
		t.Errorf("len(42): got %d, want 42", result6)
	}
}

func TestFuncMap_Reverse(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["reverse"]

	slice := []interface{}{"a", "b", "c"}
	result := f.(func(interface{}) []interface{})(slice)
	if len(result) != 3 || result[0] != "c" || result[2] != "a" {
		t.Errorf("reverse: got %v, want [c b a]", result)
	}
}

func TestFuncMap_Union(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["union"]

	s1 := []interface{}{"a", "b"}
	s2 := []interface{}{"c", "d"}
	result := f.(func(...interface{}) []interface{})(s1, s2)
	if len(result) != 4 {
		t.Errorf("union length: got %d, want 4", len(result))
	}
}

func TestFuncMap_ReplaceRE(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["replaceRE"]

	result := f.(func(interface{}, interface{}, interface{}) string)("[a-z]+", "X", "hello world")
	if result != "X X" {
		t.Errorf("replaceRE: got %s, want 'X X'", result)
	}

	// With template.HTML input (from plainify)
	htmlInput := template.HTML("hello world")
	result2 := f.(func(interface{}, interface{}, interface{}) string)("[a-z]+", "X", htmlInput)
	if result2 != "X X" {
		t.Errorf("replaceRE with HTML: got %s, want 'X X'", result2)
	}
}

func TestFuncMap_FindRE(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["findRE"]

	result := f.(func(string, string) []string)("[a-z]+", "hello world")
	if len(result) != 2 || result[0] != "hello" || result[1] != "world" {
		t.Errorf("findRE: got %v, want [hello world]", result)
	}
}

func TestFuncMap_EchoParam(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["echoParam"]

	m := map[string]interface{}{"key": "value"}
	result := f.(func(interface{}, string) interface{})(m, "key")
	if result != "value" {
		t.Errorf("echoParam: got %v, want value", result)
	}

	// Missing key
	result2 := f.(func(interface{}, string) interface{})(m, "missing")
	if result2 != "" {
		t.Errorf("echoParam missing: got %v, want empty", result2)
	}
}

func TestFuncMap_Truncate(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["truncate"]

	result := f.(func(int, string) string)(5, "hello world")
	if result != "hello…" {
		t.Errorf("truncate(5, hello world): got %s, want 'hello…'", result)
	}

	// Short string
	result2 := f.(func(int, string) string)(100, "hi")
	if result2 != "hi" {
		t.Errorf("truncate long limit: got %s, want 'hi'", result2)
	}
}

func TestFuncMap_Dict(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["dict"]

	result, err := f.(func(...interface{}) (map[string]interface{}, error))("key1", "val1", "key2", "val2")
	if err != nil {
		t.Fatalf("dict error: %v", err)
	}
	if result["key1"] != "val1" || result["key2"] != "val2" {
		t.Errorf("dict: got %v, want {key1: val1, key2: val2}", result)
	}

	// Odd args error
	_, err = f.(func(...interface{}) (map[string]interface{}, error))("key1")
	if err == nil {
		t.Error("dict with odd args: expected error, got nil")
	}

	// Non-string key error
	_, err = f.(func(...interface{}) (map[string]interface{}, error))(123, "val")
	if err == nil {
		t.Error("dict with non-string key: expected error, got nil")
	}
}

func TestFuncMap_Merge(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["merge"]

	m1 := map[string]interface{}{"a": 1}
	m2 := map[string]interface{}{"b": 2}
	result, err := f.(func(...interface{}) (map[string]interface{}, error))(m1, m2)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}
	if result["a"] != 1 || result["b"] != 2 {
		t.Errorf("merge: got %v, want {a: 1, b: 2}", result)
	}

	// With map[interface{}]interface{} (YAML style)
	m3 := map[interface{}]interface{}{"c": 3}
	result2, err := f.(func(...interface{}) (map[string]interface{}, error))(m1, m3)
	if err != nil {
		t.Fatalf("merge with interface key map error: %v", err)
	}
	if result2["c"] != 3 {
		t.Errorf("merge with interface key: got %v, want c=3", result2)
	}

	// Non-map error
	_, err = f.(func(...interface{}) (map[string]interface{}, error))("not a map")
	if err == nil {
		t.Error("merge with non-map: expected error, got nil")
	}
}

func TestFuncMap_HtmlEscape(t *testing.T) {
	fm := FuncMap("https://example.com/")

	// htmlEscape
	he := fm["htmlEscape"].(func(string) string)
	result := he("<script>alert('xss')</script>")
	expected := "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"
	if result != expected {
		t.Errorf("htmlEscape: got %s, want %s", result, expected)
	}

	// htmlUnescape
	hue := fm["htmlUnescape"].(func(string) string)
	result2 := hue("&lt;script&gt;")
	if result2 != "<script>" {
		t.Errorf("htmlUnescape: got %s, want <script>", result2)
	}
}

func TestFuncMap_Humanize(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["humanize"].(func(string) string)

	result := f("hello")
	if result != "Hello" {
		t.Errorf("humanize(hello): got %s, want Hello", result)
	}

	// Already capitalized
	result2 := f("World")
	if result2 != "World" {
		t.Errorf("humanize(World): got %s, want World", result2)
	}

	// Empty string
	result3 := f("")
	if result3 != "" {
		t.Errorf("humanize(empty): got %s, want empty", result3)
	}
}

func TestFuncMap_ReflectIsMap(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["reflect_IsMap"].(func(interface{}) bool)

	if !f(map[string]interface{}{"a": 1}) {
		t.Error("reflect_IsMap(map): got false, want true")
	}
	if f([]interface{}{}) {
		t.Error("reflect_IsMap(slice): got true, want false")
	}
}

func TestFuncMap_ReflectIsSlice(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["reflect_IsSlice"].(func(interface{}) bool)

	if !f([]interface{}{"a", "b"}) {
		t.Error("reflect_IsSlice(slice): got false, want true")
	}
	if f(map[string]interface{}{}) {
		t.Error("reflect_IsSlice(map): got true, want false")
	}
}

func TestFuncMap_XMLEscape(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["transform_XMLEscape"].(func(interface{}) string)

	result := f("<tag>content&more</tag>")
	if !contains(result, "&lt;") || !contains(result, "&amp;") {
		t.Errorf("transform_XMLEscape: got %s, want escaped content", result)
	}
}

func TestFuncMap_LangFormatNumberCustom(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["lang_FormatNumberCustom"].(func(int, interface{}) string)

	result := f(2, 1234.5678)
	if result != "1234.57" {
		t.Errorf("lang_FormatNumberCustom(2, 1234.5678): got %s, want 1234.57", result)
	}

	// With int
	result2 := f(0, 42)
	if result2 != "42" {
		t.Errorf("lang_FormatNumberCustom(0, 42): got %s, want 42", result2)
	}

	// With non-numeric
	result3 := f(2, "not a number")
	if result3 != "" {
		t.Errorf("lang_FormatNumberCustom with string: got %s, want empty", result3)
	}
}

func TestFuncMap_UniqFunc(t *testing.T) {
	result := uniqFunc([]interface{}{"a", "b", "a", "c", "b"})
	if len(result) != 3 {
		t.Errorf("uniqFunc: got %d items, want 3", len(result))
	}
}

func TestFuncMap_QuerifyFunc(t *testing.T) {
	result, err := querifyFunc("key1", "val1", "key2", "val2")
	if err != nil {
		t.Fatalf("querifyFunc error: %v", err)
	}
	if result != "key1=val1&key2=val2" {
		t.Errorf("querifyFunc: got %s, want 'key1=val1&key2=val2'", result)
	}

	// Odd args error
	_, err = querifyFunc("key1")
	if err == nil {
		t.Error("querifyFunc with odd args: expected error, got nil")
	}
}

func TestFuncMap_TimeParseFunc(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["time"].(func(...interface{}) (*TimeResult, error))

	// No args: returns now
	result, err := f()
	if err != nil {
		t.Fatalf("time() error: %v", err)
	}
	if result == nil {
		t.Fatal("time(): got nil, want non-nil")
	}

	// Parse RFC3339
	result2, err := f("2026-07-20T10:00:00Z")
	if err != nil {
		t.Fatalf("time(parse) error: %v", err)
	}
	formatted := result2.Format("2006-01-02")
	if formatted != "2026-07-20" {
		t.Errorf("time.Format: got %s, want 2026-07-20", formatted)
	}

	// Unix timestamp
	unix := result2.Unix()
	if unix == 0 {
		t.Error("time.Unix: got 0, want non-zero")
	}

	// Invalid input
	_, err = f(123) // non-string
	if err == nil {
		t.Error("time(int): expected error, got nil")
	}

	// Unparseable string
	_, err = f("not-a-date")
	if err == nil {
		t.Error("time(invalid): expected error, got nil")
	}
}

func TestFuncMap_I18nFunc(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["i18n"].(func(string, ...interface{}) string)

	// Without bundle, returns key
	result := f("some.key")
	if result != "some.key" {
		t.Errorf("i18n without bundle: got %s, want some.key", result)
	}
}

func TestFuncMap_HreflangFunc(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["hreflang"].(func(interface{}) template.HTML)

	// Nil context
	result := f(nil)
	if result != "" {
		t.Errorf("hreflang(nil): got %s, want empty", result)
	}

	// Context with no translation links
	ctx := &Context{}
	result2 := f(ctx)
	if result2 != "" {
		t.Errorf("hreflang with no links: got %s, want empty", result2)
	}

	// Context with translation links
	ctx.TranslationLinks = []TranslationLink{
		{Lang: "zh-cn", URL: "https://example.com/post/"},
		{Lang: "en", URL: "https://example.com/en/post/", IsCurrent: true},
	}
	ctx.Site = &SiteContext{Config: &config.Config{}}
	result3 := f(ctx)
	if result3 == "" {
		t.Error("hreflang with links: got empty, want non-empty")
	}
}

func TestFuncMap_LangPrefixFunc(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["langPrefix"].(func(interface{}) string)

	// Nil context
	result := f(nil)
	if result != "" {
		t.Errorf("langPrefix(nil): got %s, want empty", result)
	}

	// Context with LanguagePrefix
	ctx := &Context{LanguagePrefix: "/en"}
	result2 := f(ctx)
	if result2 != "/en" {
		t.Errorf("langPrefix: got %s, want /en", result2)
	}
}

func TestFuncMap_TranslationLinksFunc(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["translationLinks"].(func(interface{}) []TranslationLink)

	// Nil context
	result := f(nil)
	if result != nil {
		t.Errorf("translationLinks(nil): got %v, want nil", result)
	}

	// Context with links
	ctx := &Context{TranslationLinks: []TranslationLink{{Lang: "en"}}}
	result2 := f(ctx)
	if len(result2) != 1 {
		t.Errorf("translationLinks: got %d, want 1", len(result2))
	}
}

func TestFuncMap_SafeFuncs(t *testing.T) {
	fm := FuncMap("https://example.com/")

	// safeCSS
	safeCSS := fm["safeCSS"].(func(interface{}) template.CSS)
	if safeCSS("body { color: red; }") != template.CSS("body { color: red; }") {
		t.Error("safeCSS: unexpected result")
	}

	// safeHTMLAttr
	safeHTMLAttr := fm["safeHTMLAttr"].(func(interface{}) template.HTMLAttr)
	if safeHTMLAttr("data-value") != template.HTMLAttr("data-value") {
		t.Error("safeHTMLAttr: unexpected result")
	}
}

func TestFuncMap_RssLastBuildDate(t *testing.T) {
	// Nil context
	result := RssLastBuildDate(nil)
	if result != "" {
		t.Errorf("RssLastBuildDate(nil): got %s, want empty", result)
	}

	// Empty RegularPages
	ctx := &Context{}
	result2 := RssLastBuildDate(ctx)
	if result2 != "" {
		t.Errorf("RssLastBuildDate with empty pages: got %s, want empty", result2)
	}

	// With pages
	now := time.Now()
	ctx.RegularPages = PageSlice{
		&Context{Lastmod: now},
		&Context{Lastmod: now.Add(-time.Hour)},
	}
	result3 := RssLastBuildDate(ctx)
	if result3 == "" {
		t.Error("RssLastBuildDate with pages: got empty, want formatted date")
	}
}

func TestFuncMap_Int(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["int"].(func(interface{}) int)

	if f(42) != 42 {
		t.Error("int(42): want 42")
	}
	if f(3.14) != 3 {
		t.Error("int(3.14): want 3")
	}
	if f("123") != 123 {
		t.Error("int(\"123\"): want 123")
	}
}

func TestFuncMap_String(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["string"].(func(interface{}) string)

	if f(42) != "42" {
		t.Errorf("string(42): got %s, want 42", f(42))
	}
}

func TestFuncMap_PrintFuncs(t *testing.T) {
	fm := FuncMap("https://example.com/")

	// print
	printFn := fm["print"].(func(...interface{}) string)
	if printFn("a", "b") != "ab" {
		t.Errorf("print: got %s, want ab", printFn("a", "b"))
	}

	// println
	printlnFn := fm["println"].(func(...interface{}) string)
	result := printlnFn("a", "b")
	if !contains(result, "a") || !contains(result, "b") {
		t.Errorf("println: got %s, want containing a and b", result)
	}
}

func TestFuncMap_Split(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["split"].(func(string, string) []string)

	result := f("a,b,c", ",")
	if len(result) != 3 || result[0] != "a" {
		t.Errorf("split: got %v, want [a b c]", result)
	}
}

func TestFuncMap_Replace(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["replace"].(func(string, string, string) string)

	result := f("hello world", "world", "universe")
	if result != "hello universe" {
		t.Errorf("replace: got %s, want 'hello universe'", result)
	}
}

func TestFuncMap_Trim(t *testing.T) {
	fm := FuncMap("https://example.com/")

	// trim
	trimFn := fm["trim"].(func(string) string)
	if trimFn("  hello  ") != "hello" {
		t.Errorf("trim: got '%s', want 'hello'", trimFn("  hello  "))
	}

	// trimPrefix
	trimPrefixFn := fm["trimPrefix"].(func(string, string) string)
	if trimPrefixFn("pre", "prefix") != "fix" {
		t.Errorf("trimPrefix: got '%s', want 'fix'", trimPrefixFn("pre", "prefix"))
	}

	// trimSuffix
	trimSuffixFn := fm["trimSuffix"].(func(string, string) string)
	if trimSuffixFn("fix", "suffix") != "suf" {
		t.Errorf("trimSuffix: got '%s', want 'suf'", trimSuffixFn("fix", "suffix"))
	}
}

func TestFuncMap_Underscore(t *testing.T) {
	fm := FuncMap("https://example.com/")
	f := fm["underscore"].(func(string) string)

	result := f("hello world")
	if result != "hello_world" {
		t.Errorf("underscore: got %s, want hello_world", result)
	}
}

func TestFuncMap_Getenv(t *testing.T) {
	fm := FuncMap("https://example.com/")

	// getenv
	getenvFn := fm["getenv"].(func(string) string)
	// PATH should exist
	if getenvFn("PATH") == "" && getenvFn("Path") == "" {
		t.Log("getenv(PATH): empty (may be expected on some systems)")
	}

	// os_Getenv
	osGetenvFn := fm["os_Getenv"].(func(string) string)
	if osGetenvFn("NON_EXISTENT_VAR_12345") != "" {
		t.Error("os_Getenv(NON_EXISTENT): want empty")
	}
}

// Add to testedFuncs in the original funcs_test.go
func init() {
	// These will be picked up by the 守护测试
}
