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
	page := &content.Page{Access: "public"}
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

func TestExpandBlock(t *testing.T) {
	r := NewRegistry()
	page := &content.Page{Access: "public"}
	body := `Before {{< redact force="true" >}}secret content{{< /redact >}} After`

	out, err := r.Expand(body, page, nil)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	if !strings.Contains(out, "Before ") || !strings.Contains(out, " After") {
		t.Errorf("surrounding content lost: %s", out)
	}
	if !strings.Contains(out, `<span class="redacted">`) {
		t.Errorf("expected redacted span, got: %s", out)
	}
	if strings.Contains(out, "secret content") {
		t.Errorf("redaction failed, content visible: %s", out)
	}
}

func TestRedactFull(t *testing.T) {
	page := &content.Page{Access: "public"}
	ctx := &Context{
		Inner:  "hello world 你好",
		Params: map[string]string{"force": "true"},
		Page:   page,
	}

	out, err := RedactHandler(ctx)
	if err != nil {
		t.Fatalf("RedactHandler: %v", err)
	}

	// "hello world 你好" has 13 runes (h,e,l,l,o, space, w,o,r,l,d, space, 你, 好)
	// Actually: "hello world 你好" = 5 + 1 + 5 + 1 + 2 = 14 chars
	if !strings.Contains(out, "█") {
		t.Errorf("expected blocks in output: %s", out)
	}
}

func TestRedactRandom(t *testing.T) {
	page := &content.Page{Access: "public"}
	body := "alpha beta gamma delta epsilon"
	ctx := &Context{
		Inner:  body,
		Params: map[string]string{"force": "true", "random": "true", "ratio": "50"},
		Page:   page,
	}

	out, err := RedactHandler(ctx)
	if err != nil {
		t.Fatalf("RedactHandler: %v", err)
	}

	if !strings.Contains(out, `<span class="redacted">`) {
		t.Errorf("expected at least some redaction: %s", out)
	}
}

func TestRedactShow(t *testing.T) {
	page := &content.Page{Access: "public"}
	ctx := &Context{
		Inner:  "secret",
		Params: map[string]string{"show": "true"},
		Page:   page,
	}

	out, err := RedactHandler(ctx)
	if err != nil {
		t.Fatalf("RedactHandler: %v", err)
	}

	if out != "secret" {
		t.Errorf("show=true should leave content unchanged, got: %s", out)
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
