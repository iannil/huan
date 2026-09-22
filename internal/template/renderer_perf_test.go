package template

import (
	"fmt"
	"html/template"
	"strings"
	"sync"
	"testing"
)

func rendererFixture(tb testing.TB, extra int) *Renderer {
	tb.Helper()
	fm := FuncMap("")
	fm["site"] = func() *SiteContext { return nil }
	fm["partial"] = func(string, any) (template.HTML, error) { return "", nil }
	fm["partialCached"] = fm["partial"]
	t := template.Must(template.New("page").Funcs(fm).Parse(`{{site.Title}}:{{partial "outer" .}}`))
	template.Must(t.New("partials/outer").Parse(`{{partialCached "inner" .}}`))
	template.Must(t.New("partials/inner").Parse(`<b>{{.Title}}</b>`))
	for i := 0; i < extra; i++ {
		template.Must(t.New(fmt.Sprintf("unused-%d", i)).Parse(`{{.Title}}`))
	}
	return NewRenderer(t, nil)
}

func TestRendererConcurrentContextIsolation(t *testing.T) {
	r := rendererFixture(t, 5)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				name := fmt.Sprintf("site-%d-%d", i, j)
				got, err := r.Render("page", &Context{Title: "<page>", Site: &SiteContext{Title: name}})
				want := name + ":<b>&lt;page&gt;</b>"
				if err != nil || got != want {
					t.Errorf("got %q, %v; want %q", got, err, want)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestRendererAlternatingTemplatesAndErrors(t *testing.T) {
	fm := FuncMap("")
	fm["site"] = func() *SiteContext { return nil }
	templates := template.Must(template.New("html").Funcs(fm).Parse(`<p>{{.Title}}/{{site.Title}}</p>`))
	template.Must(templates.New("js").Parse(`<script>const title={{.Title}};</script>`))
	template.Must(templates.New("bad").Parse(`{{.MissingField}}`))
	r := NewRenderer(templates, nil)
	for i := 0; i < 20; i++ {
		ctx := &Context{Title: "<x>", Site: &SiteContext{Title: "next"}}
		for _, tc := range []struct{ name, want string }{
			{"html", `<p>&lt;x&gt;/next</p>`},
			{"js", `<script>const title="\u003cx\u003e";</script>`},
		} {
			got, err := r.Render(tc.name, ctx)
			if err != nil || got != tc.want {
				t.Fatalf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
			}
		}
		if _, err := r.Render("bad", ctx); err == nil {
			t.Fatal("expected execution error")
		}
	}
}

func BenchmarkRenderer(b *testing.B) {
	r := rendererFixture(b, 50)
	ctx := &Context{Title: strings.Repeat("正文", 40), Site: &SiteContext{Title: "site"}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Render("page", ctx); err != nil {
			b.Fatal(err)
		}
	}
}
