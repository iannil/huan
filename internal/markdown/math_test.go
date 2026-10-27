package markdown

import (
	"html"
	"strings"
	"testing"
)

func TestRendererMathPassthrough(t *testing.T) {
	for _, tt := range []struct{ name, src, want string }{
		{"backslash inline", `Text \(a_1 * b_2 + \{x\}\) end.`, `<p>Text \(a_1 * b_2 + \{x\}\) end.</p>`},
		{"dollar inline", `Text $a_1 * b_2 + \{x\}$ end.`, `<p>Text $a_1 * b_2 + \{x\}$ end.</p>`},
		{"backslash display", "\\[\n\\begin{aligned}\na &< b \\\\\nc &= d\n\\end{aligned}\n\\]", "\\[\n\\begin{aligned}\na &amp;&lt; b \\\\\nc &amp;= d\n\\end{aligned}\n\\]"},
		{"dollar display", "$$\na_b * c_d\n\n- x \\\\\n$$", "$$\na_b * c_d\n\n- x \\\\\n$$"},
		{"html is text", `\(<script>x</script> &amp;\)`, `\(&lt;script&gt;x&lt;/script&gt; &amp;amp;\)`},
		{"code span", "`\\(x\\) $a_b$`", "<code>\\(x\\) $a_b$</code>"},
		{"currency", `It costs $5 and **only** $10 today.`, `<p>It costs $5 and <strong>only</strong> $10 today.</p>`},
		{"escaped dollar", `Pay \$5 and **only** \$10.`, `<p>Pay $5 and <strong>only</strong> $10.</p>`},
		{"unclosed", `Before \(x **bold**`, `<p>Before (x <strong>bold</strong></p>`},
		{"quote", "> \\[\n> a_b * c_d\n> \\]", "\\[\na_b * c_d\n\\]"},
		{"inline display", `Before \[a_b * c_d\] after`, `Before \[a_b * c_d\] after`},
		{"table", "| Symbol | Meaning |\n| --- | --- |\n| \\(a_i + b_j + \\{x\\}\\) | **sum** |", `<td>\(a_i + b_j + \{x\}\)</td>`},
		{"list", "- \\[\n  a_i + b_j\n  \\]\n\nAfter **bold**.", `<p>After <strong>bold</strong>.</p>`},
		{"indented code", "    \\[\n    a_i\n    \\]", "<pre><code>\\[\na_i\n\\]"},
		{"unclosed block", "\\[\na_i\n\n# Heading\n\nThe literal \\] is prose.", `<h1 id="heading">Heading</h1>`},
		{"escaped close", `\(a \\) + b\)`, `\(a \\) + b\)`},
		{"escaped open", `\\(a **bold**\\)`, `\(a <strong>bold</strong>\)`},
		{"dollar whitespace", `$ x **bold** $`, `$ x <strong>bold</strong> $`},
		{"dollar number", `$2 + 2$`, `$2 + 2$`},
		{"multiline inline dollars", "Before $$a_i\nb_j$$ after **bold**.", "Before $$a_i\nb_j$$ after <strong>bold</strong>."},
		{"display closing in prose", "$$\na_i\nb_j$$ after **bold**.", "$$\na_i\nb_j$$ after <strong>bold</strong>."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRenderer(nil).Render(tt.src)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("want %q in output, got %q", tt.want, got)
			}
		})
	}
}

func TestRendererMathCodeFence(t *testing.T) {
	got, err := NewRenderer(nil).Render("```text\n\\[\nx_a * y_b\n\\]\n```\n\n\\(z_a * z_b\\)")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "<code") || !strings.Contains(got, `\(z_a * z_b\)`) || strings.Contains(got, "<em>") {
		t.Fatalf("code/math corrupted: %s", got)
	}
}

// Real formalization-paper equations: preserve every TeX byte after HTML decoding.
func TestRendererMathFormalizationEquations(t *testing.T) {
	for _, src := range []string{
		`\[
\begin{aligned}
\operatorname{StableD}(G,cl)\iff{}&G\ne\varnothing\land G\subseteq E_0\land cl\in\operatorname{Common}_0(G)\\
&\land\exists j_0,\ell.\ \forall j\ge j_0\ \forall s\in G:\
\mathrm{cfg}_j\land r_j(s)\ne\mathrm{None}\land cl(r_j(s))=\ell.
\end{aligned}
\]`,
		`\[
\operatorname{Open}(h')=
\begin{cases}
\operatorname{Open}(h)\cup\{d\},&\text{Raise},\\
\operatorname{Open}(h)\setminus\{d\},&\text{Resolve with Audit, or Withdraw},\\
\operatorname{Open}(h),&\text{otherwise}.
\end{cases}
\]`,
	} {
		got, err := NewRenderer(nil).Render(src)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html.UnescapeString(got), src) {
			t.Errorf("equation corrupted: %s", got)
		}
	}
}
