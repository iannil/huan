package render

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gpdf-dev/gpdf/document"
	"github.com/gpdf-dev/gpdf/template"
	"github.com/iannil/huan-plugin-ebook-exporter/content"
	"github.com/iannil/huan-plugin-ebook-exporter/style"
)

// cjkFontFamily is the family name under which the CJK font is registered
// with gpdf; it is also installed as the default font family.
const cjkFontFamily = "notocjk"

// longLatinRun matches unbreakable Latin/alphanumeric runs that PDF text
// engines cannot wrap — long URLs, hashes, identifiers.
var longLatinRun = regexp.MustCompile(`[A-Za-z0-9^_]{20,}`)

// wrapLongLatin splits runs of unbreakable Latin characters longer than
// maxRunes (if > 0, else 60) into chunks of at most maxRunes characters,
// joined by spaces. gpdf has no soft-break API, so chunking at character
// boundaries with spaces gives the line breaker somewhere to wrap; 60 chars
// at ~11pt is well under the A4 text width. Text without long runs passes
// through unchanged.
func wrapLongLatin(s string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = 60
	}
	if !longLatinRun.MatchString(s) {
		return s
	}
	return longLatinRun.ReplaceAllStringFunc(s, func(m string) string {
		if len([]rune(m)) <= maxRunes {
			return m
		}
		var chunks []string
		rs := []rune(m)
		for len(rs) > maxRunes {
			chunks = append(chunks, string(rs[:maxRunes]))
			rs = rs[maxRunes:]
		}
		if len(rs) > 0 {
			chunks = append(chunks, string(rs))
		}
		return strings.Join(chunks, " ")
	})
}

// pdfText is the shared inline-text pipeline for PDF emission: strip
// inline markdown, then pre-wrap unbreakable long Latin runs so they cannot
// overflow the page's right edge.
func pdfText(s string) string {
	return wrapLongLatin(inlinePlain(s), 60)
}

// PDFOptions controls PDF rendering. FontPath points at a CJK-capable font
// (required for Chinese books; may be empty for English ones, in which case
// gpdf's built-in fonts are used).
type PDFOptions struct {
	FontPath string
}

// RenderPDF renders the book (in the given language) to a PDF file at outPath
// using the gpdf engine: cover page, TOC page, then one page per chapter.
func RenderPDF(book *content.BookEntry, lang content.Lang, outPath string, opts PDFOptions) error {
	options := []template.Option{
		template.WithPageSize(document.A4),
		template.WithMargins(document.UniformEdges(document.Mm(20))),
	}
	if opts.FontPath != "" {
		data, err := style.ReadFontData(opts.FontPath)
		if err != nil {
			return fmt.Errorf("read font: %w", err)
		}
		options = append(options, template.WithFont(cjkFontFamily, data))
		// Make every text use the CJK font by default so Chinese renders.
		options = append(options, template.WithDefaultFont(cjkFontFamily, 11))
	}
	doc := template.New(options...)

	renderCover(doc, book, lang)
	renderTOC(doc, book, lang)
	if err := renderChapters(doc, book, lang); err != nil {
		return err
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create pdf: %w", err)
	}
	defer f.Close()
	if err := doc.Render(f); err != nil {
		return fmt.Errorf("render pdf: %w", err)
	}
	return nil
}

// renderCover emits the cover page: title, subtitle and version metadata.
//
// Spacer rows and centered-text rows are separate AutoRows: batching a
// Spacer and an AlignCenter Text into one row makes gpdf place the text
// off the page top-right corner (invisible covers / overflowing part
// pages).
func renderCover(doc *template.Document, book *content.BookEntry, lang content.Lang) {
	title := book.TitleZH
	subtitle := book.SubtitleZH
	if lang == content.LangEN {
		title = book.TitleEN
		subtitle = ""
	}
	page := doc.AddPage()
	row := func(fn func(c *template.ColBuilder)) {
		page.AutoRow(func(r *template.RowBuilder) {
			r.Col(12, fn)
		})
	}
	row(func(c *template.ColBuilder) { c.Spacer(document.Pt(120)) })
	row(func(c *template.ColBuilder) {
		c.Text(TypographCJK(title), template.FontSize(28), template.Bold(), template.AlignCenter())
	})
	if subtitle != "" {
		row(func(c *template.ColBuilder) {
			c.Text(inlinePlain(subtitle), template.FontSize(14), template.AlignCenter())
		})
	}
	row(func(c *template.ColBuilder) { c.Spacer(document.Pt(40)) })
	row(func(c *template.ColBuilder) {
		c.Text("版本 "+book.Version+" · 更新于 "+book.LastUpdated, template.FontSize(10), template.AlignCenter())
	})
}

// renderTOC emits a table-of-contents page: plain chapter titles, one per
// line. gpdf has no TOC API, so no page numbers or links. Each line is its
// own AutoRow (see renderChapters for why blocks must not share a row).
func renderTOC(doc *template.Document, book *content.BookEntry, lang content.Lang) {
	page := doc.AddPage()
	row := func(fn func(c *template.ColBuilder)) {
		page.AutoRow(func(r *template.RowBuilder) {
			r.Col(12, fn)
		})
	}
	row(func(c *template.ColBuilder) {
		c.Text("目录", template.FontSize(20), template.Bold())
	})
	for _, sec := range book.OrderedSections() {
		if sec.Type == "part" {
			partTitle := sec.Title
			row(func(c *template.ColBuilder) {
				c.Text(inlinePlain(partTitle), template.FontSize(13), template.Bold())
			})
		}
		for _, ch := range sec.Chapters {
			title := ch.Title
			row(func(c *template.ColBuilder) {
				c.Text("    "+inlinePlain(title), template.FontSize(11))
			})
		}
	}
}

// renderChapters emits each chapter on its own page with normalized blocks.
//
// Every block is its own AutoRow: gpdf rows are atomic (BreakInside=
// BreakAvoid), so a paragraph that does not fit in the remaining page
// space moves whole to the next page. Batching a whole chapter into one
// AutoRow instead triggers a gpdf flow-split bug that renders the split
// remainder's first line wider than the page (overflowing the right edge)
// and duplicates it across the page break.
func renderChapters(doc *template.Document, book *content.BookEntry, lang content.Lang) error {
	for _, sec := range book.OrderedSections() {
		if sec.Type == "part" {
			// Part separator page. Spacer and centered title in separate
			// rows (see renderCover for the gpdf batching bug).
			page := doc.AddPage()
			part := sec
			page.AutoRow(func(r *template.RowBuilder) {
				r.Col(12, func(c *template.ColBuilder) { c.Spacer(document.Pt(160)) })
			})
			page.AutoRow(func(r *template.RowBuilder) {
				r.Col(12, func(c *template.ColBuilder) {
					c.Text(inlinePlain(part.Title), template.FontSize(24), template.Bold(), template.AlignCenter())
				})
			})
		}
		for _, ch := range sec.Chapters {
			src := ch.SourcePath
			if lang == content.LangEN && ch.ENPath != "" {
				src = ch.ENPath
			}
			du, err := ParseChapter(src)
			if err != nil {
				return fmt.Errorf("parse chapter %s: %w", src, err)
			}
			title, unit := ch, du
			page := doc.AddPage()
			page.AutoRow(func(r *template.RowBuilder) {
				r.Col(12, func(c *template.ColBuilder) {
					c.Text(inlinePlain(title.Title), template.FontSize(20), template.Bold())
				})
			})
			for i := range unit.Blocks {
				b := unit.Blocks[i]
				page.AutoRow(func(r *template.RowBuilder) {
					r.Col(12, func(c *template.ColBuilder) {
						emitBlock(c, b)
					})
				})
			}
		}
	}
	return nil
}

// emitBlock maps one normalized block onto PDF text lines.
func emitBlock(c *template.ColBuilder, b Block) {
	switch b.Kind {
	case BlockHeading:
		// Heading levels map to descending sizes; clamp levels 4-6 to 12pt.
		size := 12.0
		switch b.Level {
		case 1:
			size = 18
		case 2:
			size = 15
		case 3:
			size = 13
		}
		c.Text(pdfText(b.Text), template.FontSize(size), template.Bold())
	case BlockParagraph:
		c.Text(pdfText(b.Text), template.FontSize(11))
	case BlockQuote:
		c.Text("▎"+pdfText(b.Text), template.FontSize(11))
	case BlockList:
		for _, item := range b.Items {
			c.Text("• "+pdfText(item), template.FontSize(11))
		}
	case BlockCode:
		c.Text(pdfText(b.Text), template.FontSize(9))
	case BlockTable:
		for _, row := range b.Rows {
			cells := make([]string, len(row))
			for i, cell := range row {
				cells[i] = pdfText(cell)
			}
			c.Text(strings.Join(cells, " | "), template.FontSize(10))
		}
	case BlockThematicBreak:
		c.Text("———", template.FontSize(11))
	}
}
