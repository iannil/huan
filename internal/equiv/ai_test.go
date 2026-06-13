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

func TestExtractAI_FallbackToArticle(t *testing.T) {
	htmlSrc := `<html><body>
	<article>
	  <h1>Title</h1>
	  <p>Article content</p>
	</article>
	</body></html>`

	got := ExtractAI(htmlSrc)

	if got.MainText != "Title Article content" {
		t.Errorf("MainText: got %q", got.MainText)
	}
}

func TestExtractAI_FallbackToDivMain(t *testing.T) {
	htmlSrc := `<html><body>
	<div class="main">
	  <div class="content">
	    <h1>Title</h1>
	    <p>Div main content</p>
	  </div>
	</div>
	</body></html>`

	got := ExtractAI(htmlSrc)

	if got.MainText != "Title Div main content" {
		t.Errorf("MainText: got %q", got.MainText)
	}
}

func TestExtractAI_MainTakesPriorityOverArticle(t *testing.T) {
	htmlSrc := `<html><body>
	<main>Main content</main>
	<article>Article content</article>
	</body></html>`

	got := ExtractAI(htmlSrc)

	if got.MainText != "Main content" {
		t.Errorf("MainText: got %q, want main to take priority", got.MainText)
	}
}
