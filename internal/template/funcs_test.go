package template

import (
	"testing"
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
	got := plainify(in)
	if got != in {
		t.Errorf("plainify no-tags shortcut: got %q, want %q", got, in)
	}
}

func TestPlainify_PBlockBoundaryBecomesNewline(t *testing.T) {
	// </p> → \n (via placeholder)
	in := "<p>first</p><p>second</p>"
	want := "first\nsecond\n"
	got := plainify(in)
	if got != want {
		t.Errorf("plainify </p> boundary:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestPlainify_BrBecomesNewline(t *testing.T) {
	in := "<p>line1<br>line2</p>"
	want := "line1\nline2\n"
	got := plainify(in)
	if got != want {
		t.Errorf("plainify <br>:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestPlainify_NonPTagsDoNotGetNewline(t *testing.T) {
	// <h2>...</h2> does NOT get placeholder; surrounding \n becomes space.
	in := "<h2>title</h2>\n<p>body</p>"
	want := "title body\n"
	got := plainify(in)
	if got != want {
		t.Errorf("plainify non-p tags:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestPlainify_DedupsConsecutiveWhitespace(t *testing.T) {
	// \n\n → \n, "   " → " ", but mixed \n + space → first one wins.
	in := "<p>a</p>\n\n\n<p>b</p>"
	want := "a\nb\n"
	got := plainify(in)
	if got != want {
		t.Errorf("plainify dedup:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestPlainify_PreservesLeadingTrailingWhitespace(t *testing.T) {
	// Hugo does NOT trim; leading \n becomes leading space, trailing preserved.
	in := "\n  <p>x</p>  "
	want := "   x\n   "
	got := plainify(in)
	if got != want {
		t.Errorf("plainify preserves edges:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestPlainify_RealWorldZhurongshuoSummary(t *testing.T) {
	// zhurongshuo general/_index.md rendered: blockquote + h2 + h3 + paragraphs.
	in := "<blockquote>\n<p>法不净空，觉无性也。（2010-10-18）</p>\n</blockquote>\n<h2 id=\"一存在\">一、存在</h2>\n<h3 id=\"11动态存在\">1.1、动态存在</h3>\n<p>可能性基底时刻...</p>"
	want := " 法不净空，觉无性也。（2010-10-18）\n一、存在 1.1、动态存在 可能性基底时刻...\n"
	got := plainify(in)
	if got != want {
		t.Errorf("plainify zhurongshuo summary:\n  in:   %q\n  got:  %q\n  want: %q", in, got, want)
	}
}

func TestPlainify_HandlesEmptyAndNil(t *testing.T) {
	if got := plainify(""); got != "" {
		t.Errorf("plainify(\"\") = %q, want empty", got)
	}
	if got := plainify(nil); got != "" {
		t.Errorf("plainify(nil) = %q, want empty", got)
	}
}
