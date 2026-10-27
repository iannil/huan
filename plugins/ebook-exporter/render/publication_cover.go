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
	xdraw "golang.org/x/image/draw"
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
		// Opening punctuation belongs with the following phrase, never at
		// the end of a display-title line (e.g. 解构文明（上）).
		for cut > 1 && cut < len(rs) && strings.ContainsRune("（《「『【〈〔([{", rs[cut-1]) {
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

func publicationCover(book *content.BookEntry, lang content.Lang, fonts CoverFonts, publications ...content.Publication) ([]byte, error) {
	publication := content.Publication{}
	if len(publications) > 0 {
		publication = publications[0]
	}
	identity := publication.Identity(lang)
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
	if len(publication.BackgroundPNG) > 0 {
		background, err := png.Decode(bytes.NewReader(publication.BackgroundPNG))
		if err != nil {
			return nil, fmt.Errorf("cover background: %w", err)
		}
		xdraw.CatmullRom.Scale(im, im.Bounds(), background, background.Bounds(), draw.Over, nil)
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
	if err = text(52, 73, identity.CoverBrand, 20, cjk, false); err != nil {
		return nil, err
	}
	text(53, 98, identity.CoverBrandLatin, 9, sans, true)
	title, sub := inlinePlain(book.TitleZH), inlinePlain(book.SubtitleZH)
	chineseSub := ""
	if lang != content.LangEN {
		title, chineseSub = splitChineseCoverTitle(title)
	}
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
		if float64(len(lines))*size*1.2 <= titleHeight && (chineseSub == "" || len(lines) == 1) {
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
	if chineseSub != "" {
		face, e := opentype.NewFace(cjk, &opentype.FaceOptions{Size: 23, DPI: 72})
		if e != nil {
			return nil, e
		}
		subs := wrapCoverText(chineseSub, face, 320)
		face.Close()
		y += 12
		for _, line := range subs {
			if e := text(54, y, line, 23, cjk, false); e != nil {
				return nil, e
			}
			y += 34
		}
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
	taglineFont, creditFont := cjk, cjk
	taglineSize, taglineY := 12.0, 661.0
	if lang == content.LangEN {
		taglineFont, creditFont = sans, latin
		taglineSize, taglineY = 10, 651
	}
	for i, line := range identity.CoverTagline {
		text(54, taglineY+float64(i)*17, line, taglineSize, taglineFont, true)
	}
	text(54, 727, identity.CoverCredit, 13, creditFont, false)
	text(54, 777, identity.CoverFooter, 10, sans, true)
	var out bytes.Buffer
	if err := png.Encode(&out, im); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func publicationCoverSVG(book *content.BookEntry, lang content.Lang, fonts CoverFonts, publications ...content.Publication) (string, error) {
	data, err := publicationCover(book, lang, fonts, publications...)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="595.3" height="841.9" viewBox="0 0 595.3 841.9"><title>%s</title><image width="595.3" height="841.9" xlink:href="data:image/png;base64,%s"/></svg>`, escapeHTML(pdfHeaderTitle(book, lang)), base64.StdEncoding.EncodeToString(data)), nil
}

// A full Chinese book name is not one display-size text stream. Keep the
// short main title (including 上/下) intact and typeset the subtitle separately.
func splitChineseCoverTitle(title string) (string, string) {
	main, sub, ok := strings.Cut(title, "：")
	if !ok || strings.TrimSpace(main) == "" || strings.TrimSpace(sub) == "" {
		return title, ""
	}
	return strings.TrimSpace(main), strings.TrimSpace(sub)
}
