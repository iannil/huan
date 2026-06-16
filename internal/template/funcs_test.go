package template

import (
	"html/template"
	"testing"

	"github.com/iannil/huan/internal/config"
)

// getFunc returns a named function from FuncMap. Fatals if missing so test
// setup errors fail loudly instead of producing misleading RED/GREEN results.
func getFunc(t *testing.T, name string) interface{} {
	t.Helper()
	fm := FuncMap("")
	fn, ok := fm[name]
	if !ok {
		t.Fatalf("FuncMap missing %q", name)
	}
	return fn
}

func TestSectionExcludedFunc(t *testing.T) {
	cfg := &config.Config{
		Languages: map[string]config.LanguageConfig{
			"zh-cn": {BaseURL: ""},
			"en":    {BaseURL: "/en", ExcludeSections: []string{"books", "gallery"}},
		},
	}
	enCtx := &Context{Site: &SiteContext{Config: cfg, LanguageCode: "en"}}
	zhCtx := &Context{Site: &SiteContext{Config: cfg, LanguageCode: "zh-cn"}}

	if !sectionExcludedFunc(enCtx, "/books/") {
		t.Error("en /books/ should be excluded")
	}
	if !sectionExcludedFunc(enCtx, "/gallery/") {
		t.Error("en /gallery/ should be excluded")
	}
	if sectionExcludedFunc(enCtx, "/products/") {
		t.Error("en /products/ should NOT be excluded")
	}
	if sectionExcludedFunc(zhCtx, "/books/") {
		t.Error("zh-cn /books/ should NOT be excluded")
	}
	if sectionExcludedFunc(nil, "/books/") {
		t.Error("nil ctx should not be excluded")
	}
	if sectionExcludedFunc(enCtx, "") {
		t.Error("empty url should not be excluded")
	}
}

func TestMathFuncs_AddSupportsFloat(t *testing.T) {
	fn := getFunc(t, "add")
	got := fn.(func(a, b interface{}) interface{})(1.5, 2.5)
	want := 4.0
	if got != want {
		t.Errorf("add(1.5, 2.5) = %v (%T), want %v", got, got, want)
	}
	// int args still work and produce float64 (matches Hugo / Go template math).
	got = fn.(func(a, b interface{}) interface{})(2, 3)
	if got != 5.0 {
		t.Errorf("add(2, 3) = %v, want 5.0", got)
	}
}

func TestMathFuncs_SubSupportsFloat(t *testing.T) {
	fn := getFunc(t, "sub")
	got := fn.(func(a, b interface{}) interface{})(5.5, 2.0)
	want := 3.5
	if got != want {
		t.Errorf("sub(5.5, 2.0) = %v (%T), want %v", got, got, want)
	}
}

func TestMathFuncs_MulSupportsFloat(t *testing.T) {
	fn := getFunc(t, "mul")
	got := fn.(func(a, b interface{}) interface{})(2.5, 4)
	want := 10.0
	if got != want {
		t.Errorf("mul(2.5, 4) = %v (%T), want %v", got, got, want)
	}
}

func TestMathFuncs_DivSupportsFloat(t *testing.T) {
	fn := getFunc(t, "div")
	// Reproduces zhurongshuo list.html: {{ div $totalWords 10000.0 }}
	// where $totalWords is int (155000) and 10000.0 is float64.
	got := fn.(func(a, b interface{}) interface{})(155000, 10000.0)
	want := 15.5
	if got != want {
		t.Errorf("div(155000, 10000.0) = %v (%T), want %v", got, got, want)
	}
	// Pure-int args also yield float64 (Hugo semantics): 10/4 = 2.5, not 2.
	got = fn.(func(a, b interface{}) interface{})(10, 4)
	if got != 2.5 {
		t.Errorf("div(10, 4) = %v, want 2.5", got)
	}
}

func TestMathFuncs_ModStaysInt(t *testing.T) {
	fn := getFunc(t, "mod")
	// Hugo: modulo is integer-only — keep signature `func(int, int) int`.
	got := fn.(func(a, b int) int)(10, 3)
	want := 1
	if got != want {
		t.Errorf("mod(10, 3) = %v, want %v", got, want)
	}
}

func TestPlainify_NoTagsShortcut(t *testing.T) {
	// Input with no < or > returned as-is.
	in := "plain text no tags"
	got := string(plainify(in))
	if got != in {
		t.Errorf("plainify no-tags shortcut: got %q, want %q", got, in)
	}
}

func TestPlainify_PBlockBoundaryBecomesNewline(t *testing.T) {
	// </p> → \n (via placeholder)
	in := "<p>first</p><p>second</p>"
	want := "first\nsecond\n"
	got := string(plainify(in))
	if got != want {
		t.Errorf("plainify </p> boundary:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestPlainify_BrBecomesNewline(t *testing.T) {
	in := "<p>line1<br>line2</p>"
	want := "line1\nline2\n"
	got := string(plainify(in))
	if got != want {
		t.Errorf("plainify <br>:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestPlainify_NonPTagsDoNotGetNewline(t *testing.T) {
	// <h2>...</h2> does NOT get placeholder; surrounding \n becomes space.
	in := "<h2>title</h2>\n<p>body</p>"
	want := "title body\n"
	got := string(plainify(in))
	if got != want {
		t.Errorf("plainify non-p tags:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestPlainify_DedupsConsecutiveWhitespace(t *testing.T) {
	// \n\n → \n, "   " → " ", but mixed \n + space → first one wins.
	in := "<p>a</p>\n\n\n<p>b</p>"
	want := "a\nb\n"
	got := string(plainify(in))
	if got != want {
		t.Errorf("plainify dedup:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestPlainify_PreservesLeadingTrailingWhitespace(t *testing.T) {
	// Hugo does NOT TrimSpace; only consecutive whitespace is deduped.
	// Source `\n` → ` ` (pre-replace); leading `\n  ` → `   ` → dedup → ` `.
	// `</p>` → `\n`; trailing `  ` is whitespace after `\n`, so dropped.
	// Net: ` x\n` (one leading space, no trailing space).
	in := "\n  <p>x</p>  "
	want := " x\n"
	got := string(plainify(in))
	if got != want {
		t.Errorf("plainify preserves edges:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestPlainify_RealWorldZhurongshuoSummary(t *testing.T) {
	// zhurongshuo general/_index.md rendered: blockquote + h2 + h3 + paragraphs.
	in := "<blockquote>\n<p>法不净空，觉无性也。（2010-10-18）</p>\n</blockquote>\n<h2 id=\"一存在\">一、存在</h2>\n<h3 id=\"11动态存在\">1.1、动态存在</h3>\n<p>可能性基底时刻...</p>"
	want := " 法不净空，觉无性也。（2010-10-18）\n一、存在 1.1、动态存在 可能性基底时刻...\n"
	got := string(plainify(in))
	if got != want {
		t.Errorf("plainify zhurongshuo summary:\n  in:   %q\n  got:  %q\n  want: %q", in, got, want)
	}
}

func TestPlainify_HandlesEmptyAndNil(t *testing.T) {
	if got := string(plainify("")); got != "" {
		t.Errorf("plainify(\"\") = %q, want empty", got)
	}
	if got := string(plainify(nil)); got != "" {
		t.Errorf("plainify(nil) = %q, want empty", got)
	}
}

func TestPlainify_ReturnsHTMLNotEscaped(t *testing.T) {
	// plainify must return template.HTML so consumers like
	// `<meta name=description content="{{ .Summary | plainify }}">` don't get
	// the value auto-escaped by Go template (which would turn the `&` in
	// `&quot;` into `&amp;`, breaking byte-parity with Hugo).
	// Regression: goldmark body has `&quot;`; plainify must preserve it as-is.
	in := "<p>Robert Frost, &quot;Mending Wall&quot;</p>"
	want := "Robert Frost, &quot;Mending Wall&quot;\n"
	got := plainify(in)
	if string(got) != want {
		t.Errorf("plainify must preserve entities (not auto-escape):\n  got:  %q\n  want: %q", got, want)
	}
	// Verify return type is template.HTML (not plain string) — this is what
	// prevents Go template from auto-escaping.
	if _, ok := interface{}(got).(template.HTML); !ok {
		t.Errorf("plainify must return template.HTML, got %T", got)
	}
}

// TestSortFunc_NoFieldArgSortsByValue verifies that sort called without a field
// argument sorts the slice by element value (matching Hugo's behavior). Hugo's
// sort: "Returns the given sequence sorted in ascending order." Without a field
// arg, the elements themselves are the sort key.
func TestSortFunc_NoFieldArgSortsByValue(t *testing.T) {
	fn := getFunc(t, "sort").(func(interface{}, ...string) ([]interface{}, error))
	// Note: huan builds the input as a []interface{} from Scratch. Use that.
	in := []interface{}{"part-02", "part-01", "part-03"}
	out, err := fn(in)
	if err != nil {
		t.Fatalf("sort error: %v", err)
	}
	want := []interface{}{"part-01", "part-02", "part-03"}
	if len(out) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", out, want)
	}
	for i, v := range out {
		if v != want[i] {
			t.Errorf("sort without field arg: idx %d got %v, want %v (full: %v)", i, v, want[i], out)
		}
	}
}

// TestSortFunc_NoFieldArgSortsChinese verifies that sort without field arg
// at least produces a deterministic order for CJK strings (Hugo's sort falls
// back to byte-level when no collator is wired in for the template fn).
func TestSortFunc_NoFieldArgSortsChinese(t *testing.T) {
	fn := getFunc(t, "sort").(func(interface{}, ...string) ([]interface{}, error))
	in := []interface{}{"三", "二", "一"}
	out, err := fn(in)
	if err != nil {
		t.Fatalf("sort error: %v", err)
	}
	// Hugo byte-level: 一(U+4E00) < 三(U+4E09) < 二(U+4E8C)
	want := []interface{}{"一", "三", "二"}
	if len(out) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", out, want)
	}
	for i, v := range out {
		if v != want[i] {
			t.Errorf("sort CJK: idx %d got %v, want %v (full: %v)", i, v, want[i], out)
		}
	}
}
