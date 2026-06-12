package equiv

import "testing"

func TestNormalizeHTML_FoldsWhitespaceBetweenTags(t *testing.T) {
	in := "<div>\n  <p>hello</p>\n</div>"
	want := "<div><p>hello</p></div>"
	got := NormalizeHTML(in)
	if got != want {
		t.Errorf("NormalizeHTML whitespace fold:\n got: %q\nwant: %q", got, want)
	}
}

func TestNormalizeHTML_SortsAttributes(t *testing.T) {
	in := `<a href="x" class="c" id="i">txt</a>`
	want := `<a class="c" href="x" id="i">txt</a>`
	got := NormalizeHTML(in)
	if got != want {
		t.Errorf("NormalizeHTML attr sort:\n got: %q\nwant: %q", got, want)
	}
}

func TestNormalizeHTML_VoidElementCanonical(t *testing.T) {
	in := `<div><br /><img src="a.png"></div>`
	want := `<div><br/><img src="a.png"/></div>`
	got := NormalizeHTML(in)
	if got != want {
		t.Errorf("NormalizeHTML void canonical:\n got: %q\nwant: %q", got, want)
	}
}
