package template

import (
	"bytes"
	htmltemplate "html/template"
	"strings"
	"testing"
)

func TestGenericTemplatePrimitives(t *testing.T) {
	fm := FuncMap("https://example.org/")
	for _, name := range []string{"parseYAML", "errorf", "textWidth", "textWrap", "textTruncate"} {
		if fm[name] == nil {
			t.Errorf("missing generic primitive %s", name)
		}
	}
	for _, name := range []string{"readingTime", "parseGuideYAML", "svgTextWidth", "svgTruncate", "svgWrap", "failRender", "guideChartTypes", "publicationRevisionNotice"} {
		if fm[name] != nil {
			t.Errorf("site policy exposed as core function %s", name)
		}
	}
}

func TestGenericYAMLAndErrors(t *testing.T) {
	for _, tc := range []struct{ name, source, want, errorText string }{
		{"nested YAML", `{{ $v := parseYAML "item:\n  name: example\n" }}{{ $v.item.name }}`, "example", ""},
		{"sequence", `{{ range parseYAML "- red\n- blue" }}{{ . }};{{ end }}`, "red;blue;", ""},
		{"malformed", `{{ parseYAML "item: [unclosed" }}`, "", "yaml:"},
		{"caller error", `{{ errorf "invalid item %d" 7 }}`, "", "invalid item 7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, err := htmltemplate.New("test").Funcs(FuncMap("")).Parse(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			err = tmpl.Execute(&out, nil)
			if tc.errorText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorText) {
					t.Fatalf("error = %v, want %q", err, tc.errorText)
				}
				if strings.Contains(err.Error(), "guide:") {
					t.Fatalf("site-specific error: %v", err)
				}
			} else if err != nil || out.String() != tc.want {
				t.Fatalf("output=%q error=%v", out.String(), err)
			}
		})
	}
}
