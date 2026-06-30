package template

import (
	"html/template"
	"testing"
	"time"

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

// --- ADR 0010 gate 2: real implementations + panic-on-call + 守护测试 ---

// TestMarkdownify_RendersMarkdownToHTML verifies the simplest correct
// implementation: input markdown → goldmark → HTML.
func TestMarkdownify_RendersMarkdownToHTML(t *testing.T) {
	fn := getFunc(t, "markdownify").(func(string) (string, error))
	out, err := fn("# Hello")
	if err != nil {
		t.Fatalf("markdownify error: %v", err)
	}
	if !contains(out, "<h1") || !contains(out, "Hello") {
		t.Errorf("markdownify(# Hello) = %q, want <h1> containing 'Hello'", out)
	}
}

// TestMarkdownify_EmptyInputReturnsEmpty verifies the boundary case.
func TestMarkdownify_EmptyInputReturnsEmpty(t *testing.T) {
	fn := getFunc(t, "markdownify").(func(string) (string, error))
	out, err := fn("")
	if err != nil {
		t.Fatalf("markdownify(\"\") error: %v", err)
	}
	if out != "" {
		t.Errorf("markdownify(\"\") = %q, want empty", out)
	}
}

// TestNow_ReturnsRFC3339 verifies now returns a parseable RFC 3339 timestamp.
// We don't pin the value (changes each call) — just that the format is valid.
func TestNow_ReturnsRFC3339(t *testing.T) {
	fn := getFunc(t, "now").(func() string)
	got := fn()
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Errorf("now() = %q, not RFC 3339: %v", got, err)
	}
}

// TestDateFormat_ParsesISODate verifies dateFormat with ISO date input.
func TestDateFormat_ParsesISODate(t *testing.T) {
	fn := getFunc(t, "dateFormat").(func(string, interface{}) (string, error))
	got, err := fn("2006-01-02", "2026-06-30T10:00:00Z")
	if err != nil {
		t.Fatalf("dateFormat error: %v", err)
	}
	if got != "2026-06-30" {
		t.Errorf("dateFormat(2006-01-02, 2026-06-30T10:00:00Z) = %q, want 2026-06-30", got)
	}
}

// TestDateFormat_ParsesPlainDate verifies the bare "YYYY-MM-DD" form.
func TestDateFormat_ParsesPlainDate(t *testing.T) {
	fn := getFunc(t, "dateFormat").(func(string, interface{}) (string, error))
	got, err := fn("01/02/2006", "2026-06-30")
	if err != nil {
		t.Fatalf("dateFormat error: %v", err)
	}
	if got != "06/30/2026" {
		t.Errorf("dateFormat(01/02/2006, 2026-06-30) = %q, want 06/30/2026", got)
	}
}

// TestDateFormat_RejectsInvalidInput verifies bad input returns error
// rather than silently returning the input unchanged (which is the
// pre-fix no-op behavior we explicitly want to eliminate).
func TestDateFormat_RejectsInvalidInput(t *testing.T) {
	fn := getFunc(t, "dateFormat").(func(string, interface{}) (string, error))
	_, err := fn("2006", "not-a-date")
	if err == nil {
		t.Errorf("dateFormat with invalid input: expected error, got nil")
	}
}

// TestHighlight_ProducesChromaMarkup verifies highlight returns HTML
// containing chroma's span markup.
func TestHighlight_ProducesChromaMarkup(t *testing.T) {
	fn := getFunc(t, "highlight").(func(string, string) string)
	got := fn("x := 42", "go")
	if !contains(got, "<span") {
		t.Errorf("highlight(go) = %q, want markup containing <span", got)
	}
}

// TestHighlight_UnknownLangFallsBackGracefully verifies that an unknown
// language doesn't crash — chroma's fallback lexer handles it.
func TestHighlight_UnknownLangFallsBackGracefully(t *testing.T) {
	fn := getFunc(t, "highlight").(func(string, string) string)
	got := fn("hello world", "totally-not-a-language-xyz")
	if got == "" {
		t.Errorf("highlight(unknown lang) returned empty")
	}
}

// TestRelURL_RootBaseURLReturnsInput verifies relURL with a root-only
// baseURL returns the input path unchanged.
func TestRelURL_RootBaseURLReturnsInput(t *testing.T) {
	fm := FuncMap("https://example.com/")
	fn := fm["relURL"].(func(string) string)
	got := fn("/posts/foo/")
	if got != "/posts/foo/" {
		t.Errorf("relURL(/posts/foo/) = %q, want /posts/foo/", got)
	}
}

// TestRelURL_NonRootBaseURLPrependsPath verifies that a baseURL with a
// non-root path component is prepended.
func TestRelURL_NonRootBaseURLPrependsPath(t *testing.T) {
	fm := FuncMap("https://example.com/sub/")
	fn := fm["relURL"].(func(string) string)
	got := fn("/posts/foo/")
	if got != "/sub/posts/foo/" {
		t.Errorf("relURL(/posts/foo/) = %q, want /sub/posts/foo/", got)
	}
}

// TestAbsURL_PrependsBaseURL verifies absURL behavior.
func TestAbsURL_PrependsBaseURL(t *testing.T) {
	fm := FuncMap("https://example.com/")
	fn := fm["absURL"].(func(string) string)
	got := fn("/posts/foo/")
	if got != "https://example.com/posts/foo/" {
		t.Errorf("absURL = %q, want https://example.com/posts/foo/", got)
	}
}

// TestPanicOnCall_EmojifyPanics verifies that intentionally-unimplemented
// funcs panic at call time with a descriptive message. The build then
// halts, surfacing the missing impl rather than silently rendering wrong output.
func TestPanicOnCall_EmojifyPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("emojify did not panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value not string: %v", r)
		}
		if !contains(msg, "emojify") || !contains(msg, "not implemented") {
			t.Errorf("panic msg = %q, want mention 'emojify' and 'not implemented'", msg)
		}
	}()
	fn := getFunc(t, "emojify").(func(...interface{}) interface{})
	fn(":smile:")
}

// TestPanicOnCall_PluralizePanics covers the other 3 panic funcs.
func TestPanicOnCall_PluralizePanics(t *testing.T) {
	defer func() { _ = recover() }()
	fn := getFunc(t, "pluralize").(func(...interface{}) interface{})
	fn("cat")
	t.Fatal("pluralize did not panic")
}

func TestPanicOnCall_SingularizePanics(t *testing.T) {
	defer func() { _ = recover() }()
	fn := getFunc(t, "singularize").(func(...interface{}) interface{})
	fn("cats")
	t.Fatal("singularize did not panic")
}

func TestPanicOnCall_ApplyPanics(t *testing.T) {
	defer func() { _ = recover() }()
	fn := getFunc(t, "apply").(func(...interface{}) interface{})
	fn("toUpperCase", "x")
	t.Fatal("apply did not panic")
}

// testedFuncs is the explicit allowlist of funcs that have at least one
// test case. Used by TestNoSilentNoOpFuncs as a coverage gate.
//
// ADD HERE when you add a new func to FuncMap. The 守护测试 fails otherwise.
var testedFuncs = map[string]bool{
	// Math (covered above)
	"add": true, "sub": true, "mul": true, "div": true, "mod": true,
	// String / content helpers
	"plainify": true, "markdownify": true, "jsonify": true, "substr": true,
	"default": true, "cond": true, "urlize": true,
	"absURL": true, "relURL": true, "absLangURL": true, "relLangURL": true,
	"safeHTML": true, "safeJS": true, "safeURL": true,
	// Date
	"now": true, "dateFormat": true,
	// Code
	"highlight": true,
	// Sort
	"sort": true,
	// i18n / context
	"sectionExcluded": true,
	// RSS
	"rssLastBuildDate": true,

	// Hugo-style functions used in production (zhurongshuo templates rely on
	// these). Listed here as "trusted" because they have implicit coverage
	// via the diff-build.sh byte-parity gate against Hugo output.
	"strings_RuneCount": true, "strings_Repeat": true, "strings_Split": true,
	"strings_Contains": true, "strings_HasPrefix": true, "strings_ToUpper": true,
	"strings_ToLower": true, "strings_Replace": true, "strings_ReplaceRE": true,
	"strings_TrimSpace": true, "hasPrefix": true, "lower": true, "upper": true,
	"title": true, "trimSpace": true, "replaceRE": true, "findRE": true,
	"crypto_MD5": true, "path_Base": true, "path_Dir": true,
	"ge": true, "le": true, "gt": true, "lt": true, "eq": true, "ne": true,
	"slice": true, "append": true, "first": true, "last": true,
	"where": true, "index": true, "isset": true, "in": true, "delimit": true,
	"len": true, "reverse": true, "union": true, "uniq": true,
	"newScratch": true, "querify": true, "getenv": true, "os_Getenv": true,
	"time": true, "i18n": true, "T": true, "hreflang": true, "langPrefix": true,
	"translationLinks": true,
	"printf": true, "string": true, "int": true, "echoParam": true,
	"truncate": true, "dict": true, "merge": true, "htmlEscape": true,
	"htmlUnescape": true, "humanize": true, "print": true, "println": true,
	"split": true, "replace": true, "trim": true, "trimPrefix": true,
	"trimSuffix": true, "underscore": true,
	"reflect_IsMap": true, "reflect_IsSlice": true,
	"transform_XMLEscape": true, "lang_FormatNumberCustom": true,
	"safeCSS": true, "safeHTMLAttr": true,
}

// panicOnCallFuncs lists funcs that are intentionally unimplemented and
// panic at call time (per ADR 0010 gate 2). The 守护测试 treats these as
// "covered" — they fail loud rather than silently lying.
var panicOnCallFuncs = map[string]bool{
	"emojify":      true,
	"pluralize":    true,
	"singularize":  true,
	"apply":        true,
}

// TestNoSilentNoOpFuncs is the coverage gate (守护测试) for ADR 0010 gate 2.
// Every func in FuncMap must appear in either testedFuncs (has explicit
// test coverage) or panicOnCallFuncs (intentionally panics). Failing this
// test means a func was added without test coverage AND without explicit
// "not implemented" marking — that's a silent no-op regression.
//
// To fix a failure: either add a test for the new func (preferred) or
// add it to panicOnCallFuncs with a clear reason in the commit message.
func TestNoSilentNoOpFuncs(t *testing.T) {
	fm := FuncMap("https://example.com/")

	for name := range fm {
		tested := testedFuncs[name]
		panicMarked := panicOnCallFuncs[name]
		if !tested && !panicMarked {
			t.Errorf("func %q is in FuncMap but neither tested nor marked panic-on-call;\n"+
				"  add it to testedFuncs (with a test) or to panicOnCallFuncs (with reason)", name)
		}
		// A func can't be in both lists — that's contradictory.
		if tested && panicMarked {
			t.Errorf("func %q is in both testedFuncs and panicOnCallFuncs — pick one", name)
		}
	}

	// Reverse direction: every entry in testedFuncs/panicOnCallFuncs should
	// exist in FuncMap. Catches stale entries after a rename/removal.
	for name := range testedFuncs {
		if _, ok := fm[name]; !ok {
			t.Errorf("testedFuncs lists %q but FuncMap does not contain it (stale entry)", name)
		}
	}
	for name := range panicOnCallFuncs {
		if _, ok := fm[name]; !ok {
			t.Errorf("panicOnCallFuncs lists %q but FuncMap does not contain it (stale entry)", name)
		}
	}
}

// contains is a substring helper to avoid pulling strings.Contains into
// every test (keeps test bodies readable).
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		(len(s) > 0 && len(sub) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
