package markdown

import (
	"bytes"

	"github.com/novel_ttl/huan/internal/config"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

// Renderer wraps goldmark for Markdown→HTML conversion.
type Renderer struct {
	gm goldmark.Markdown
}

// NewRenderer creates a goldmark-based Markdown renderer from config.
func NewRenderer(cfg *config.MarkupConfig) *Renderer {
	opts := []goldmark.Option{}

	if cfg != nil && cfg.Goldmark.Renderer.Unsafe {
		opts = append(opts, goldmark.WithRendererOptions(html.WithUnsafe()))
	}

	return &Renderer{
		gm: goldmark.New(opts...),
	}
}

// Render converts Markdown source to HTML.
func (r *Renderer) Render(src string) (string, error) {
	var buf bytes.Buffer
	if err := r.gm.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
