package shortcode

import (
	"strings"
	"testing"

	"github.com/iannil/huan/internal/content"
)

func TestParseParams(t *testing.T) {
	cases := []struct {
		in       string
		expected map[string]string
	}{
		{``, map[string]string{}},
		{`src="audio.mp3" title="Hello"`, map[string]string{"src": "audio.mp3", "title": "Hello"}},
		{`force="true" ratio="75"`, map[string]string{"force": "true", "ratio": "75"}},
		{`positional`, map[string]string{"0": "positional"}},
		{`a b c`, map[string]string{"0": "a", "1": "b", "2": "c"}},
	}
	for _, c := range cases {
		got := parseParams(c.in)
		for k, v := range c.expected {
			if got[k] != v {
				t.Errorf("parseParams(%q)[%q] = %q, want %q", c.in, k, got[k], v)
			}
		}
	}
}

func TestExpandInline(t *testing.T) {
	r := NewRegistry()
	page := &content.Page{}
	body := `Hello {{< audio src="test.mp3" title="Test" >}} World`

	out, err := r.Expand(body, page, nil)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	if !strings.Contains(out, `<audio controls`) {
		t.Errorf("expected audio tag, got: %s", out)
	}
	if !strings.Contains(out, `src="test.mp3"`) {
		t.Errorf("expected src=test.mp3, got: %s", out)
	}
	if !strings.Contains(out, `Hello `) || !strings.Contains(out, ` World`) {
		t.Errorf("expected surrounding content preserved, got: %s", out)
	}
}

func TestImg(t *testing.T) {
	ctx := &Context{
		Params: map[string]string{"src": "pic.jpg", "title": "Caption"},
	}
	out, err := ImgHandler(ctx)
	if err != nil {
		t.Fatalf("ImgHandler: %v", err)
	}
	if !strings.Contains(out, `href="pic.jpg"`) {
		t.Errorf("expected href in output: %s", out)
	}
	if !strings.Contains(out, `data-caption="Caption"`) {
		t.Errorf("expected data-caption: %s", out)
	}
}
