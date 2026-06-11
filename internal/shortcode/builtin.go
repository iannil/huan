package shortcode

import (
	"fmt"
	"strings"
)

// AudioHandler implements the {{< audio src="..." title="..." >}} shortcode.
// Mirrors themes/zozo/layouts/shortcodes/audio.html.
func AudioHandler(ctx *Context) (string, error) {
	src := ctx.Params["src"]
	title := ctx.Params["title"]

	var sb strings.Builder
	sb.WriteString(`<div class="audio-player">`)
	if title != "" {
		sb.WriteString(fmt.Sprintf(`<p class="audio-title">%s</p>`, title))
	}
	sb.WriteString(`<audio controls preload="metadata">`)
	sb.WriteString(fmt.Sprintf(`<source src="%s" type="audio/mpeg">`, src))
	sb.WriteString(fmt.Sprintf(`<source src="%s" type="audio/ogg">`, src))
	sb.WriteString(fmt.Sprintf(`<source src="%s" type="audio/wav">`, src))
	sb.WriteString(`您的浏览器不支持音频播放。`)
	sb.WriteString(`</audio>`)
	sb.WriteString(`</div>`)

	return sb.String(), nil
}

// ImgHandler implements the {{< img src="..." title="..." >}} shortcode.
// Mirrors themes/zozo/layouts/shortcodes/img.html.
func ImgHandler(ctx *Context) (string, error) {
	src := ctx.Params["src"]
	title := ctx.Params["title"]

	return fmt.Sprintf(`<div class="fancybox"><a data-fancybox="gallery" href="%s" data-caption="%s"><img src="%s" /></a></div>`,
		src, title, src), nil
}
