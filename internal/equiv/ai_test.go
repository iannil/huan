package equiv

import (
	"reflect"
	"testing"
)

func TestExtractAI_CapturesMainContentAndOutline(t *testing.T) {
	htmlSrc := `<html><body>
<header>site header</header>
<nav><a href="/a">A</a></nav>
<main>
  <article>
    <h1>Title</h1>
    <h2>Section</h2>
    <p>Body text</p>
  </article>
</main>
<aside>related</aside>
<footer>copyright</footer>
</body></html>`

	got := ExtractAI(htmlSrc)

	if got.MainText != "Title Section Body text" {
		t.Errorf("MainText: got %q", got.MainText)
	}
	expectedOutline := []Heading{{Level: 1, Text: "Title"}, {Level: 2, Text: "Section"}}
	if !reflect.DeepEqual(got.Outline, expectedOutline) {
		t.Errorf("Outline: got %v want %v", got.Outline, expectedOutline)
	}
	expectedSemantic := map[string]bool{"header": true, "nav": true, "main": true, "article": true, "aside": true, "footer": true}
	if !reflect.DeepEqual(got.Semantic, expectedSemantic) {
		t.Errorf("Semantic: got %v want %v", got.Semantic, expectedSemantic)
	}
	expectedLinks := []string{"/a"}
	if !reflect.DeepEqual(got.NavLinks, expectedLinks) {
		t.Errorf("NavLinks: got %v want %v", got.NavLinks, expectedLinks)
	}
}
