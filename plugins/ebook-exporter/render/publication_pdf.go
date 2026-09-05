package render

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image/png"
	"io"
	"regexp"
	"strconv"

	"github.com/gpdf-dev/gpdf/pdf"
	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

var furnitureBlock = regexp.MustCompile(`(?s)BT\n.*?\nET`)
var furnitureFont = regexp.MustCompile(`/F\d+ 9 Tf\b`)
var furniturePos = regexp.MustCompile(`(?m)^([-\d.]+) ([-\d.]+) Td$`)

func pdfStreamBytes(r *pdf.Reader, obj pdf.Object) ([]byte, error) {
	obj, err := r.Resolve(obj)
	if err != nil {
		return nil, err
	}
	if arr, ok := obj.(pdf.Array); ok {
		var result []byte
		for _, o := range arr {
			b, e := pdfStreamBytes(r, o)
			if e != nil {
				return nil, e
			}
			result = append(result, b...)
			result = append(result, '\n')
		}
		return result, nil
	}
	s, ok := obj.(pdf.Stream)
	if !ok {
		return nil, fmt.Errorf("expected PDF stream, got %T", obj)
	}
	if filter, ok := s.Dict[pdf.Name("Filter")]; ok {
		if filter != pdf.Name("FlateDecode") {
			return nil, fmt.Errorf("unsupported PDF content filter: %v", filter)
		}
		z, e := zlib.NewReader(bytes.NewReader(s.Content))
		if e != nil {
			return nil, e
		}
		defer z.Close()
		return io.ReadAll(z)
	}
	return s.Content, nil
}

// finishPublicationPDF replaces the reserved cover page and fixes physical
// page numbering after pagination. Body text is never geometrically redacted.
func finishPublicationPDF(data, cover []byte, book *content.BookEntry, lang content.Lang, plan *pdfPagePlan) ([]byte, error) {
	r, err := pdf.NewReader(data)
	if err != nil {
		return nil, err
	}
	n, err := r.PageCount()
	if err != nil {
		return nil, err
	}
	m := pdf.NewModifier(r)
	im, err := png.Decode(bytes.NewReader(cover))
	if err != nil {
		return nil, err
	}
	var rgb bytes.Buffer
	z := zlib.NewWriter(&rgb)
	for y := 0; y < im.Bounds().Dy(); y++ {
		for x := 0; x < im.Bounds().Dx(); x++ {
			a, b, c, _ := im.At(x, y).RGBA()
			z.Write([]byte{byte(a >> 8), byte(b >> 8), byte(c >> 8)})
		}
	}
	z.Close()
	imageRef := m.AllocObject()
	m.SetObject(imageRef, pdf.Stream{Dict: pdf.Dict{pdf.Name("Type"): pdf.Name("XObject"), pdf.Name("Subtype"): pdf.Name("Image"), pdf.Name("Width"): pdf.Integer(im.Bounds().Dx()), pdf.Name("Height"): pdf.Integer(im.Bounds().Dy()), pdf.Name("ColorSpace"): pdf.Name("DeviceRGB"), pdf.Name("BitsPerComponent"): pdf.Integer(8), pdf.Name("Filter"): pdf.Name("FlateDecode")}, Content: rgb.Bytes()})
	numberFont := m.AllocObject()
	m.SetObject(numberFont, pdf.Dict{pdf.Name("Type"): pdf.Name("Font"), pdf.Name("Subtype"): pdf.Name("Type1"), pdf.Name("BaseFont"): pdf.Name("Helvetica")})
	noHeader := map[int]bool{0: true, 1: true, 2: true, n - 1: true}
	ci := 0
	physicalPage := 4 + plan.tocPages
	for _, s := range book.OrderedSections() {
		if s.Type == "part" {
			noHeader[physicalPage-1] = true
			physicalPage++
		}
		for range s.Chapters {
			physicalPage += plan.chapterCount[ci]
			ci++
		}
	}
	for i := 0; i < n; i++ {
		page, e := r.Page(i)
		if e != nil {
			return nil, e
		}
		d, e := r.ResolveDict(page.Ref)
		if e != nil {
			return nil, e
		}
		copyDict := func(v pdf.Dict) pdf.Dict {
			out := pdf.Dict{}
			for k, o := range v {
				out[k] = o
			}
			return out
		}
		d = copyDict(d)
		res, e := r.ResolveDict(d[pdf.Name("Resources")])
		if e != nil {
			return nil, e
		}
		res = copyDict(res)
		var body []byte
		if i == 0 {
			res[pdf.Name("XObject")] = pdf.Dict{pdf.Name("PublicationCover"): imageRef}
			body = []byte("q\n595.3 0 0 841.9 0 0 cm\n/PublicationCover Do\nQ\n")
		} else {
			raw, e := pdfStreamBytes(r, d[pdf.Name("Contents")])
			if e != nil {
				return nil, e
			}
			body = furnitureBlock.ReplaceAllFunc(raw, func(block []byte) []byte {
				if !furnitureFont.Match(block) {
					return block
				}
				pos := furniturePos.FindSubmatch(block)
				if pos == nil {
					return block
				}
				y, _ := strconv.ParseFloat(string(pos[2]), 64)
				if y < 110 {
					return nil
				}
				if y > coverHeight-110 {
					if noHeader[i] {
						return nil
					}
					block = bytes.Replace(block, []byte(" 9 Tf"), []byte(" 8.5 Tf"), 1)
					block = furniturePos.ReplaceAll(block, []byte(fmt.Sprintf("%s %.2f Td", pos[1], coverHeight-42)))
					return append([]byte("q\n0.4 0.4 0.38 rg\n"), append(block, []byte("\nQ\n")...)...)
				}
				return block
			})
			fonts, _ := r.ResolveDict(res[pdf.Name("Font")])
			fonts = copyDict(fonts)
			fonts[pdf.Name("PublicationNumber")] = numberFont
			res[pdf.Name("Font")] = fonts
			number := strconv.Itoa(i + 1)
			x := (coverWidth - float64(len(number))*9*.556) / 2
			body = append(body, []byte(fmt.Sprintf("\nq\n0.38 0.38 0.36 rg\nBT\n/PublicationNumber 9 Tf\n%.3f 36 Td\n(%s) Tj\nET\nQ\n", x, number))...)
		}
		streamRef := m.AllocObject()
		m.SetObject(streamRef, pdf.Stream{Dict: pdf.Dict{}, Content: body})
		d[pdf.Name("Contents")] = streamRef
		d[pdf.Name("Resources")] = res
		m.SetObject(page.Ref, d)
	}
	return m.Bytes()
}
