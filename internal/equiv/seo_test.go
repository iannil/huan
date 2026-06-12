package equiv

import (
	"reflect"
	"testing"
)

func TestExtractSEO_CapturesCoreFields(t *testing.T) {
	htmlSrc := `<!doctype html><html><head>
<title>Page T</title>
<meta name="description" content="desc">
<meta property="og:title" content="OG T">
<meta property="og:image" content="og.png">
<link rel="canonical" href="https://x.com/p/">
<meta name="robots" content="index,follow">
</head><body>
<h1>H1a</h1><h2>H2a</h2><h2>H2b</h2><h3>H3a</h3>
<script type="application/ld+json">{"@type":"Article","name":"X"}</script>
</body></html>`

	got := ExtractSEO(htmlSrc)

	if got.Title != "Page T" {
		t.Errorf("Title: got %q want %q", got.Title, "Page T")
	}
	if got.Description != "desc" {
		t.Errorf("Description: got %q want %q", got.Description, "desc")
	}
	if !reflect.DeepEqual(got.OG, map[string]string{"og:title": "OG T", "og:image": "og.png"}) {
		t.Errorf("OG: got %v", got.OG)
	}
	if got.Canonical != "https://x.com/p/" {
		t.Errorf("Canonical: got %q", got.Canonical)
	}
	if got.Robots != "index,follow" {
		t.Errorf("Robots: got %q", got.Robots)
	}
	if !reflect.DeepEqual(got.Headings, []Heading{{Level: 1, Text: "H1a"}, {Level: 2, Text: "H2a"}, {Level: 2, Text: "H2b"}, {Level: 3, Text: "H3a"}}) {
		t.Errorf("Headings: got %v", got.Headings)
	}
	if len(got.JSONLD) != 1 || got.JSONLD[0] == "" {
		t.Errorf("JSONLD: got %v", got.JSONLD)
	}
}

func TestSEOFields_EqualWhenMatching(t *testing.T) {
	a := SEOFields{Title: "T", Description: "D", OG: map[string]string{"og:title": "T"}}
	b := SEOFields{Title: "T", Description: "D", OG: map[string]string{"og:title": "T"}}
	if !a.Equal(b) {
		t.Errorf("expected equal")
	}
	c := SEOFields{Title: "T2", Description: "D", OG: map[string]string{"og:title": "T"}}
	if a.Equal(c) {
		t.Errorf("expected not equal (title differs)")
	}
}

func TestExtractSEO_FoldsTitleAndHeadingWhitespace(t *testing.T) {
	htmlSrc := `<html><head><title>Hello
	World</title></head><body><h1>Section
	One</h1></body></html>`
	got := ExtractSEO(htmlSrc)
	if got.Title != "Hello World" {
		t.Errorf("Title whitespace not folded: got %q", got.Title)
	}
	if len(got.Headings) != 1 || got.Headings[0].Text != "Section One" {
		t.Errorf("Heading whitespace not folded: got %+v", got.Headings)
	}
}
