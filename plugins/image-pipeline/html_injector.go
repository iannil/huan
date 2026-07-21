package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var imgTagRe = regexp.MustCompile(`<img\s[^>]*src="([^"]+)"[^>]*>`)

// InjectHTMLFiles processes all HTML files in the output directory,
// replacing img tags with srcset/picture-enhanced versions.
func InjectHTMLFiles(outputDir string, processed []ProcessedImage, cfg Config) error {
	// Build lookup map: original relPath -> ProcessedImage
	lookup := make(map[string]ProcessedImage)
	for _, p := range processed {
		lookup[p.Original.RelPath] = p
	}

	return filepath.Walk(outputDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		newContent := injectHTML(string(data), processed, cfg)
		if newContent != string(data) {
			if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
		}
		return nil
	})
}

// injectHTML processes a single HTML string, replacing img tags
// with srcset/picture-enhanced versions based on processed images.
func injectHTML(html string, processed []ProcessedImage, cfg Config) string {
	// Build lookup
	lookup := make(map[string]ProcessedImage)
	for _, p := range processed {
		lookup[p.Original.RelPath] = p
	}

	return imgTagRe.ReplaceAllStringFunc(html, func(match string) string {
		// Extract src attribute
		srcMatch := regexp.MustCompile(`src="([^"]+)"`).FindStringSubmatch(match)
		if len(srcMatch) < 2 {
			return match
		}
		src := srcMatch[1]

		// Normalize path: strip leading /
		relPath := strings.TrimPrefix(src, "/")

		pi, ok := lookup[relPath]
		if !ok {
			return match
		}

		// Skip if img already has srcset
		if strings.Contains(match, "srcset=") {
			return match
		}

		// Collect variants by format
		jpgVariants := filterVariants(pi.Variants, "jpg")
		webpVariants := filterVariants(pi.Variants, "webp")
		avifVariants := filterVariants(pi.Variants, "avif")

		// Sort by width ascending
		sortVariantsByWidth(jpgVariants)
		sortVariantsByWidth(webpVariants)
		sortVariantsByWidth(avifVariants)

		// Pick the largest jpg as the default src
		defaultSrc := pi.Original.RelPath
		var defaultWidth int
		if len(jpgVariants) > 0 {
			defaultSrc = jpgVariants[len(jpgVariants)-1].RelPath
			defaultWidth = jpgVariants[len(jpgVariants)-1].Width
		}

		// Build srcset string
		var srcsetParts []string
		for _, v := range jpgVariants {
			srcsetParts = append(srcsetParts, fmt.Sprintf("/%s %dw", v.RelPath, v.Width))
		}
		srcsetStr := strings.Join(srcsetParts, ", ")

		// Build sizes attribute
		sizesStr := ""
		if defaultWidth > 0 {
			sizesStr = fmt.Sprintf(`sizes="(max-width: %dpx) 100vw, %dpx"`, defaultWidth, defaultWidth)
		}

		// Build new img tag with preserved attributes
		preserved := stripImgAttrs(match, []string{"src", "srcset", "sizes", "loading"})

		var buf bytes.Buffer

		if cfg.InjectPicture && (len(webpVariants) > 0 || len(avifVariants) > 0) {
			buf.WriteString("<picture>\n")
			// AVIF sources first (preferred)
			if len(avifVariants) > 0 {
				last := avifVariants[len(avifVariants)-1]
				buf.WriteString(fmt.Sprintf("  <source srcset=\"/%s\" type=\"image/avif\">\n", last.RelPath))
			}
			// WebP sources
			if len(webpVariants) > 0 {
				last := webpVariants[len(webpVariants)-1]
				buf.WriteString(fmt.Sprintf("  <source srcset=\"/%s\" type=\"image/webp\">\n", last.RelPath))
			}
		}

		buf.WriteString(fmt.Sprintf("<img src=\"/%s\"", defaultSrc))
		if cfg.InjectSrcset && srcsetStr != "" {
			buf.WriteString(fmt.Sprintf(" srcset=\"%s\"", srcsetStr))
		}
		if sizesStr != "" {
			buf.WriteString(fmt.Sprintf(" %s", sizesStr))
		}
		if preserved != "" {
			buf.WriteString(fmt.Sprintf(" %s", preserved))
		}
		if cfg.InjectLazyLoading {
			buf.WriteString(" loading=\"lazy\"")
		}
		buf.WriteString(fmt.Sprintf(" width=\"%d\" height=\"%d\"", pi.Original.Width, pi.Original.Height))
		buf.WriteString(">")

		if cfg.InjectPicture && (len(webpVariants) > 0 || len(avifVariants) > 0) {
			buf.WriteString("\n</picture>")
		}

		return buf.String()
	})
}

// filterVariants returns variants matching the given format.
func filterVariants(variants []ImageVariant, format string) []ImageVariant {
	var out []ImageVariant
	for _, v := range variants {
		if v.Format == format {
			out = append(out, v)
		}
	}
	return out
}

// sortVariantsByWidth sorts variants by width ascending.
func sortVariantsByWidth(variants []ImageVariant) {
	sort.Slice(variants, func(i, j int) bool {
		return variants[i].Width < variants[j].Width
	})
}

// stripImgAttrs removes specified attributes from an img tag and returns
// the remaining attributes as a string.
func stripImgAttrs(imgTag string, remove []string) string {
	removeSet := make(map[string]bool)
	for _, a := range remove {
		removeSet[a] = true
	}

	// Extract attributes from the img tag
	attrRe := regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)
	matches := attrRe.FindAllStringSubmatch(imgTag, -1)
	var kept []string
	for _, m := range matches {
		if !removeSet[m[1]] {
			kept = append(kept, fmt.Sprintf("%s=\"%s\"", m[1], m[2]))
		}
	}
	return strings.Join(kept, " ")
}