package template

import (
	"bytes"
	"html/template"
	"testing"
)

// The theme's books/practices dictionaries contain only Site.Pages entries
// populated from map[*content.Page]*Context. Products uses GetPage, whose
// static result is *Context (including a non-nil zero page on a miss).
func TestThemePagePresenceMatchesComparison(t *testing.T) {
	var typedNil *Context
	cases := []struct {
		name string
		page interface{}
		want string
	}{
		{"missing-map-entry", nil, "false"},
		{"typed-nil", typedNil, "false"},
		{"empty-page", &Context{}, "true"},
		{"full-page", &Context{Title: "Book", Content: template.HTML("<p>Body</p>"), RelPermalink: "/books/example/", Params: map[string]interface{}{"level": "beginner"}}, "true"},
		{"getpage-miss", (&SiteContext{}).GetPage("/products/missing"), "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, expression := range []string{"ne .Page nil", "not (not .Page)"} {
				tmpl := template.Must(template.New("presence").Funcs(FuncMap("")).Parse("{{ " + expression + " }}"))
				var out bytes.Buffer
				if err := tmpl.Execute(&out, map[string]interface{}{"Page": tc.page}); err != nil {
					t.Fatal(err)
				}
				if out.String() != tc.want {
					t.Fatalf("%s returned %q, want %q", expression, out.String(), tc.want)
				}
			}
		})
	}
}
