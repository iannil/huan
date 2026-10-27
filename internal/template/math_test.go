package template

import (
	"strings"
	"testing"
)

func TestMarkdownifyMathPassthrough(t *testing.T) {
	for _, src := range []string{`\(x_a * y_b + \{z\}\)`, `$x_a * y_b + \{z\}$`, "\\[\nx_a \\\\\ny_b\n\\]", "$$\nx_a \\\\\ny_b\n$$"} {
		got, err := markdownifyFunc(src)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, src) {
			t.Errorf("math source %q corrupted: %q", src, got)
		}
	}
}
