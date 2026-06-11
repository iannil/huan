package output

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"github.com/tdewolff/minify/v2/json"
	"github.com/tdewolff/minify/v2/svg"
	"github.com/tdewolff/minify/v2/xml"
)

// Minifier wraps tdewolff/minify with the same configuration Hugo uses.
type Minifier struct {
	m *minify.M
}

// NewMinifier creates a minifier matching Hugo's tdewolff settings:
//   HTML: keepWhitespace = false
//   CSS:  precision = 0
//   JS:   precision = 0
func NewMinifier() *Minifier {
	m := minify.New()

	// HTML: keepWhitespace=false (matches Hugo default)
	m.Add("text/html", &html.Minifier{
		KeepDocumentTags:    true,
		KeepEndTags:         true,
		KeepSpecialComments: true,
		KeepDefaultAttrVals: true,
		KeepWhitespace:      false,
	})

	// CSS: precision=0
	m.Add("text/css", &css.Minifier{
		Precision: 0,
	})

	// JS: precision=0
	m.Add("application/javascript", &js.Minifier{
		Precision: 0,
	})
	m.Add("text/javascript", &js.Minifier{
		Precision: 0,
	})

	// JSON
	m.Add("application/json", &json.Minifier{})

	// SVG
	m.Add("image/svg+xml", &svg.Minifier{
		KeepComments: false,
	})

	// XML (for RSS, sitemap)
	m.Add("application/xml", &xml.Minifier{
		KeepWhitespace: false,
	})
	m.Add("text/xml", &xml.Minifier{
		KeepWhitespace: false,
	})

	return &Minifier{m: m}
}

// Minify applies the right minifier based on the file extension.
// Returns the original content if minification fails or is disabled for the type.
func (mi *Minifier) Minify(relPath, content string) string {
	if mi == nil {
		return content
	}

	mediaType := mediaTypeForExt(relPath)
	if mediaType == "" {
		return content
	}

	out, err := mi.m.String(mediaType, content)
	if err != nil {
		return content
	}
	return out
}

// MinifyBytes is the byte-slice variant of Minify.
func (mi *Minifier) MinifyBytes(relPath string, data []byte) []byte {
	if mi == nil {
		return data
	}

	mediaType := mediaTypeForExt(relPath)
	if mediaType == "" {
		return data
	}

	var buf bytes.Buffer
	if err := mi.m.Minify(mediaType, &buf, bytes.NewReader(data)); err != nil {
		return data
	}
	return buf.Bytes()
}

// mediaTypeForExt maps a file path to its media type for minification.
func mediaTypeForExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js", ".mjs":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".xml":
		return "application/xml"
	default:
		return ""
	}
}
