package main

import (
	"strings"
	"testing"
)

func TestWrapSVG(t *testing.T) {
	got := wrapSVG(`<svg class="kroki"></svg>`, "mermaid", "diagram")
	want := `<figure class="diagram diagram-mermaid" role="img"><svg class="kroki"></svg></figure>`
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFallbackClientMermaid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FallbackMode = "client"
	b := diagramBlock{Full: "<div class=\"highlight\">x</div>", Lang: "mermaid", Source: "graph TD\nA-->B\n"}
	repl, needJS := fallbackReplacement(b, cfg)
	if !needJS {
		t.Errorf("mermaid client fallback should need JS")
	}
	if !strings.HasPrefix(repl, `<pre class="mermaid">`) || !strings.Contains(repl, "A--&gt;B") {
		t.Errorf("repl = %q", repl)
	}
}

func TestFallbackClientNonMermaidDegradesToCodeblock(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FallbackMode = "client"
	b := diagramBlock{Full: "<div class=\"highlight\">ORIG</div>", Lang: "d2", Source: "a -> b"}
	repl, needJS := fallbackReplacement(b, cfg)
	if needJS {
		t.Errorf("d2 cannot use mermaid.js")
	}
	if repl != b.Full {
		t.Errorf("non-mermaid client fallback should keep original block, got %q", repl)
	}
}

func TestInjectMermaidJSOnce(t *testing.T) {
	html := "<html><body><p>hi</p></body></html>"
	out := injectMermaidJS(html, "/js/mermaid.js")
	if strings.Count(out, "/js/mermaid.js") != 1 {
		t.Fatalf("expected 1 script, got %q", out)
	}
	// idempotent
	out2 := injectMermaidJS(out, "/js/mermaid.js")
	if strings.Count(out2, "/js/mermaid.js") != 1 {
		t.Errorf("double injection: %q", out2)
	}
}
