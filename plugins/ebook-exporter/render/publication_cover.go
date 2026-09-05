package render

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"strings"
	"sync"
	"unicode"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
	"github.com/iannil/huan-plugin-ebook-exporter/style"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// CoverFonts keeps publication artwork independent of EPUB body typography.
type CoverFonts struct{ CJK, Latin string }

const coverWidth, coverHeight = 595.3, 841.9
const coverScale = 300.0 / 72.0

var coverFontCache sync.Map

func loadCoverFont(path string, fallback []byte) (*sfnt.Font, error) {
	key := path
	if key == "" {
		key = fmt.Sprintf("builtin-%d", len(fallback))
	}
	if v, ok := coverFontCache.Load(key); ok {
		return v.(*sfnt.Font), nil
	}
	data := fallback
	if path != "" {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}
	f, err := opentype.Parse(data)
	if err != nil {
		if c, e := sfnt.ParseCollection(data); e == nil {
			f, err = c.Font(0)
		}
	}
	if err != nil {
		return nil, err
	}
	coverFontCache.Store(key, f)
	return f, nil
}

// wrapCoverText measures actual glyph advances. No truncation or ellipsis:
// long titles are reflowed and, if needed, reduced to fit the title area.
func wrapCoverText(text string, face font.Face, maxWidth float64) []string {
	var lines []string
	text = strings.TrimSpace(text)
	for text != "" {
		rs := []rune(text)
		cut := len(rs)
		lastSpace := 0
		for i, r := range rs {
			if unicode.IsSpace(r) {
				lastSpace = i
			}
			if float64(font.MeasureString(face, string(rs[:i+1])))/64 > maxWidth {
				cut = i
				if lastSpace > 0 && !unicode.Is(unicode.Han, r) && (i == 0 || !unicode.Is(unicode.Han, rs[i-1])) {
					cut = lastSpace
				}
				if cut == 0 {
					cut = 1
				}
				break
			}
		}
		// Closing punctuation belongs to the preceding phrase, never to
		// the start of the next display-title line.
		for cut > 1 && cut < len(rs) && strings.ContainsRune("，。；：！？、）》」』】〉〕,:!?;)]}", rs[cut]) {
			cut--
		}
		// Keep short Latin tokens such as AI/LLM together in Chinese titles.
		latinToken := func(r rune) bool { return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' }
		wordCut := cut
		for wordCut > 0 && wordCut < len(rs) && latinToken(rs[wordCut-1]) && latinToken(rs[wordCut]) {
			wordCut--
		}
		if wordCut > 0 {
			cut = wordCut
		}
		lines = append(lines, strings.TrimSpace(string(rs[:cut])))
		text = strings.TrimSpace(string(rs[cut:]))
	}
	// Avoid a single dangling Han character on the last cover-title line.
	if len(lines) > 1 {
		last, prev := []rune(lines[len(lines)-1]), []rune(lines[len(lines)-2])
		if len(last) == 1 && unicode.Is(unicode.Han, last[0]) && len(prev) > 1 && unicode.Is(unicode.Han, prev[len(prev)-1]) {
			lines[len(lines)-1] = string(prev[len(prev)-1]) + lines[len(lines)-1]
			lines[len(lines)-2] = string(prev[:len(prev)-1])
		}
	}
	return lines
}

func publicationCover(book *content.BookEntry, lang content.Lang, fonts CoverFonts) ([]byte, error) {
	if fonts.CJK == "" {
		fonts.CJK, _ = style.FindCJKFont("")
	}
	cjk, err := loadCoverFont(fonts.CJK, goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("cover CJK font: %w", err)
	}
	latin, err := loadCoverFont(fonts.Latin, goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("cover Latin font: %w", err)
	}
	sans, _ := loadCoverFont("", goregular.TTF)
	im := image.NewRGBA(image.Rect(0, 0, int(math.Round(coverWidth*coverScale)), int(math.Round(coverHeight*coverScale))))
	draw.Draw(im, im.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	red := color.RGBA{173, 47, 46, 255}
	ivory := color.RGBA{252, 251, 239, 255}
	draw.Draw(im, image.Rect(int(math.Round(425*coverScale)), 0, im.Bounds().Dx(), im.Bounds().Dy()), image.NewUniform(red), image.Point{}, draw.Src)
	for _, cy := range []float64{343, 421, 499} {
		cx, y, r := 510*coverScale, cy*coverScale, 18*coverScale
		for py := int(y - r - 1); py <= int(y+r+1); py++ {
			for px := int(cx - r - 1); px <= int(cx+r+1); px++ {
				// Four subpixel samples keep small circular edges smooth at print size.
				n := 0
				for _, dy := range []float64{.25, .75} {
					for _, dx := range []float64{.25, .75} {
						a, b := float64(px)+dx-cx, float64(py)+dy-y
						if a*a+b*b <= r*r {
							n++
						}
					}
				}
				if n > 0 {
					im.SetRGBA(px, py, color.RGBA{uint8((int(ivory.R)*n + int(red.R)*(4-n)) / 4), uint8((int(ivory.G)*n + int(red.G)*(4-n)) / 4), uint8((int(ivory.B)*n + int(red.B)*(4-n)) / 4), 255})
				}
			}
		}
	}
	text := func(x, y float64, s string, size float64, f *sfnt.Font, gray bool) error {
		face, e := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 300, Hinting: font.HintingNone})
		if e != nil {
			return e
		}
		defer face.Close()
		col := color.RGBA{31, 31, 31, 255}
		if gray {
			col = color.RGBA{102, 102, 102, 255}
		}
		d := font.Drawer{Dst: im, Src: image.NewUniform(col), Face: face, Dot: fixed.Point26_6{X: fixed.Int26_6(x * coverScale * 64), Y: fixed.Int26_6(y * coverScale * 64)}}
		d.DrawString(s)
		return nil
	}
	if err = text(52, 73, "祝融说。", 20, cjk, false); err != nil {
		return nil, err
	}
	text(53, 98, "ZHURONGSHUO", 9, sans, true)
	title, sub := inlinePlain(book.TitleZH), inlinePlain(book.SubtitleZH)
	f := cjk
	size := 60.0
	if lang == content.LangEN {
		if book.TitleEN != "" {
			title = inlinePlain(book.TitleEN)
		}
		sub = ""
		f = latin
		size = 36
		// An English edition may fall back to a Chinese collection title.
		for _, r := range title {
			if unicode.Is(unicode.Han, r) {
				f = cjk
				break
			}
		}
	}
	var lines []string
	titleHeight := 260.0
	if sub != "" {
		face, e := opentype.NewFace(cjk, &opentype.FaceOptions{Size: 13, DPI: 72})
		if e != nil {
			return nil, e
		}
		subLines := wrapCoverText(sub, face, 315)
		face.Close()
		titleHeight = math.Min(titleHeight, 615-283-22-float64(len(subLines))*20)
	}
	for ; size >= 18; size -= 1 {
		face, e := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72})
		if e != nil {
			return nil, e
		}
		lines = wrapCoverText(title, face, 320)
		face.Close()
		if float64(len(lines))*size*1.2 <= titleHeight {
			break
		}
	}
	if size < 18 {
		return nil, fmt.Errorf("cover title too long: %q", title)
	}
	y := 283.0
	for _, line := range lines {
		text(52, y, line, size, f, false)
		y += size * 1.2
	}
	if sub != "" {
		face, _ := opentype.NewFace(cjk, &opentype.FaceOptions{Size: 13, DPI: 72})
		subs := wrapCoverText(sub, face, 315)
		face.Close()
		y += 22
		if y+float64(len(subs))*20 > 615 {
			return nil, fmt.Errorf("cover subtitle too long: %q", sub)
		}
		for _, s := range subs {
			text(54, y, s, 13, cjk, true)
			y += 20
		}
	}
	if lang == content.LangEN {
		text(54, 651, "FORM NOT VOID,", 10, sans, true)
		text(54, 668, "MIND NO CORE", 10, sans, true)
		text(54, 727, "Rong Zhu", 13, latin, false)
	} else {
		text(54, 661, "法不净空，觉无性也。", 12, cjk, true)
		text(54, 727, "祝融 著", 13, cjk, false)
	}
	text(54, 777, "zhurongshuo.com", 10, sans, true)
	var out bytes.Buffer
	if err := png.Encode(&out, im); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func publicationCoverSVG(book *content.BookEntry, lang content.Lang, fonts CoverFonts) (string, error) {
	data, err := publicationCover(book, lang, fonts)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="595.3" height="841.9" viewBox="0 0 595.3 841.9"><title>%s</title><image width="595.3" height="841.9" xlink:href="data:image/png;base64,%s"/></svg>`, escapeHTML(pdfHeaderTitle(book, lang)), base64.StdEncoding.EncodeToString(data)), nil
}
