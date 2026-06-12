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
