//go:build !race

package template

import "testing"

// Race instrumentation randomly discards sync.Pool entries; allocation budgets
// apply to normal execution. Context isolation is tested under both modes.
// Unrelated layouts must not add a template-set copy to each warm render.
func TestRendererWarmAllocations(t *testing.T) {
	measure := func(extra int) float64 {
		r := rendererFixture(t, extra)
		ctx := &Context{Title: "page", Site: &SiteContext{Title: "site"}}
		return testing.AllocsPerRun(20, func() {
			if _, err := r.Render("page", ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
	small, large := measure(0), measure(100)
	if large > small+50 {
		t.Fatalf("warm render allocations scale with unused layouts: %.0f -> %.0f", small, large)
	}
}
