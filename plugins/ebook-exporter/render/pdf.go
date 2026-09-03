package render

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gpdf-dev/gpdf/document"
	"github.com/gpdf-dev/gpdf/pdf"
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
// using the gpdf engine: cover page, TOC page (with page numbers), then one
// page per chapter, with a running header, footer page numbers and an
// injected /Outlines bookmark tree.
//
// Pagination plan: chapter page numbers are measured first (each chapter is
// rendered standalone — chapters start on a fresh page via AddPage, so their
// page counts are independent of preceding content), then the final document
// is rendered once with exact TOC numbers, and the outline is injected as a
// post-step (gpdf's template layer has no outline API).
func RenderPDF(book *content.BookEntry, lang content.Lang, outPath string, opts PDFOptions) error {
	plan, err := measurePDFPages(book, lang, opts)
	if err != nil {
		return fmt.Errorf("measure pages: %w", err)
	}

	doc, err := newPDFDocument(book, lang, opts)
	if err != nil {
		return err
	}

	renderCover(doc, book, lang)
	renderTOC(doc, book, lang, plan)
	if err := renderChapters(doc, book, lang); err != nil {
		return err
	}

	data, err := doc.Generate()
	if err != nil {
		return fmt.Errorf("render pdf: %w", err)
	}
	data, err = injectOutline(data, plan.outline)
	if err != nil {
		return fmt.Errorf("inject outline: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("write pdf: %w", err)
	}
	return nil
}

// newPDFDocument builds a template document with the shared page setup plus
// the running header (book title) and footer (page number). Header/Footer
// are Document methods that must be registered before AddPage — they apply
// to every page, and their measured heights shrink the body area, so every
// measurement render must use this same builder.
func newPDFDocument(book *content.BookEntry, lang content.Lang, opts PDFOptions) (*template.Document, error) {
	options := []template.Option{
		template.WithPageSize(document.A4),
		template.WithMargins(document.UniformEdges(document.Mm(20))),
	}
	if opts.FontPath != "" {
		data, err := style.ReadFontData(opts.FontPath)
		if err != nil {
			return nil, fmt.Errorf("read font: %w", err)
		}
		options = append(options, template.WithFont(cjkFontFamily, data))
		// Make every text use the CJK font by default so Chinese renders.
		options = append(options, template.WithDefaultFont(cjkFontFamily, 11))
	}
	doc := template.New(options...)
	headerTitle := pdfHeaderTitle(book, lang)
	if headerTitle != "" {
		doc.Header(func(p *template.PageBuilder) {
			p.AutoRow(func(r *template.RowBuilder) {
				r.Col(12, func(c *template.ColBuilder) {
					c.Text(headerTitle, template.FontSize(9), template.AlignCenter())
				})
			})
		})
	}
	doc.Footer(func(p *template.PageBuilder) {
		p.AutoRow(func(r *template.RowBuilder) {
			r.Col(12, func(c *template.ColBuilder) {
				// Pure digits only: the footer band must not mix other text
				// with the page number (publication footers scan for it).
				c.PageNumber(template.FontSize(9), template.AlignCenter())
			})
		})
	})
	return doc, nil
}

// pdfHeaderTitle picks the running-header title: the edition language's
// title, falling back to the ZH title when the EN title is missing (a book
// without an EN title must not silently lose its running header).
func pdfHeaderTitle(book *content.BookEntry, lang content.Lang) string {
	if lang == content.LangEN && book.TitleEN != "" {
		return book.TitleEN
	}
	return book.TitleZH
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

// pdfPagePlan is the measured pagination of a book: how many pages the TOC
// occupies, the 1-based start page of every flattened chapter, and the
// outline entries derived from the same walk.
type pdfPagePlan struct {
	tocPages     int
	chapterPage  map[int]int // flattened chapter index -> 1-based start page
	chapterCount map[int]int // flattened chapter index -> measured page count
	outline      []OutlineEntry
}

// flattenChapters returns all chapters in assembly order (specials' only
// chapter, then each part's children, in OrderedSections order).
func flattenChapters(book *content.BookEntry) []content.Chapter {
	var out []content.Chapter
	for _, sec := range book.OrderedSections() {
		out = append(out, sec.Chapters...)
	}
	return out
}

// measurePDFPages renders each chapter into a throwaway document with the
// exact same page setup (header/footer included) as the final render and
// counts its pages. Because every chapter starts on a fresh AddPage, page
// counts are independent of preceding content, so the standalone measurement
// equals the in-document pagination. The TOC's own page count is measured
// the same way; entry numbers live in their own grid column, so their width
// never changes line wrapping and the measurement is stable in one pass.
func measurePDFPages(book *content.BookEntry, lang content.Lang, opts PDFOptions) (*pdfPagePlan, error) {
	plan := &pdfPagePlan{
		chapterPage:  map[int]int{},
		chapterCount: map[int]int{},
	}
	chapters := flattenChapters(book)
	for i, ch := range chapters {
		doc, err := newPDFDocument(book, lang, opts)
		if err != nil {
			return nil, err
		}
		if err := renderChapterInto(doc, ch, lang); err != nil {
			return nil, err
		}
		data, err := doc.Generate()
		if err != nil {
			return nil, fmt.Errorf("measure chapter %q: %w", ch.Title, err)
		}
		r, err := pdf.NewReader(data)
		if err != nil {
			return nil, fmt.Errorf("measure chapter %q: %w", ch.Title, err)
		}
		n, err := r.PageCount()
		if err != nil {
			return nil, fmt.Errorf("count pages of chapter %q: %w", ch.Title, err)
		}
		if n < 1 {
			n = 1
		}
		plan.chapterCount[i] = n
	}

	// TOC page count (numbers are placeholders; layout is number-independent).
	tocDoc, err := newPDFDocument(book, lang, opts)
	if err != nil {
		return nil, err
	}
	renderTOC(tocDoc, book, lang, plan) // chapterPage empty -> placeholder numbers
	tocData, err := tocDoc.Generate()
	if err != nil {
		return nil, fmt.Errorf("measure toc: %w", err)
	}
	tr, err := pdf.NewReader(tocData)
	if err != nil {
		return nil, fmt.Errorf("measure toc: %w", err)
	}
	plan.tocPages, err = tr.PageCount()
	if err != nil {
		return nil, fmt.Errorf("count toc pages: %w", err)
	}
	if plan.tocPages < 1 {
		plan.tocPages = 1
	}

	// Walk sections assigning start pages: cover is page 1, the TOC occupies
	// pages 2..1+tocPages, then parts (one separator page each) and chapters
	// (measured counts).
	page := 2 + plan.tocPages
	ci := 0
	for _, sec := range book.OrderedSections() {
		if sec.Type == "part" {
			plan.outline = append(plan.outline, OutlineEntry{Title: inlinePlain(sec.Title), Page: page, Level: 1})
			page++
		}
		for _, ch := range sec.Chapters {
			level := 1
			if sec.Type == "part" {
				level = 2
			}
			plan.chapterPage[ci] = page
			plan.outline = append(plan.outline, OutlineEntry{Title: inlinePlain(ch.Title), Page: page, Level: level})
			page += plan.chapterCount[ci]
			ci++
		}
	}
	return plan, nil
}

// renderTOC emits the table-of-contents pages: entry titles with page
// numbers (from the measured plan; blank placeholders during measurement).
// The number lives in its own grid column so its width cannot change line
// wrapping — that keeps the measured TOC page count identical to the final
// render. Each line is its own AutoRow (see renderChapters for why blocks
// must not share a row).
func renderTOC(doc *template.Document, book *content.BookEntry, lang content.Lang, plan *pdfPagePlan) {
	page := doc.AddPage()
	row := func(cols ...func(c *template.ColBuilder)) {
		page.AutoRow(func(r *template.RowBuilder) {
			for i, fn := range cols {
				width := 10
				if i > 0 {
					width = 2
				}
				r.Col(width, fn)
			}
		})
	}
	row(func(c *template.ColBuilder) {
		c.Text("目录", template.FontSize(20), template.Bold())
	})
	ci := 0
	for _, sec := range book.OrderedSections() {
		if sec.Type == "part" {
			partTitle := sec.Title
			row(func(c *template.ColBuilder) {
				c.Text(inlinePlain(partTitle), template.FontSize(13), template.Bold())
			})
		}
		for _, ch := range sec.Chapters {
			title := ch.Title
			num := ""
			if p, ok := plan.chapterPage[ci]; ok {
				num = fmt.Sprintf("%d", p)
			}
			row(
				func(c *template.ColBuilder) {
					c.Text("    "+inlinePlain(title), template.FontSize(11))
				},
				func(c *template.ColBuilder) {
					if num != "" {
						c.Text(num, template.FontSize(11), template.AlignRight())
					}
				},
			)
			ci++
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
			if err := renderChapterInto(doc, ch, lang); err != nil {
				return err
			}
		}
	}
	return nil
}

// renderChapterInto emits one chapter on a fresh page: title row then one
// AutoRow per normalized block. Shared by the final render and the
// measurement pass so both paginate identically (see measurePDFPages).
func renderChapterInto(doc *template.Document, ch content.Chapter, lang content.Lang) error {
	src := ch.SourcePath
	if lang == content.LangEN && ch.ENPath != "" {
		src = ch.ENPath
	}
	unit, err := ParseChapter(src)
	if err != nil {
		return fmt.Errorf("parse chapter %s: %w", src, err)
	}
	page := doc.AddPage()
	page.AutoRow(func(r *template.RowBuilder) {
		r.Col(12, func(c *template.ColBuilder) {
			c.Text(inlinePlain(ch.Title), template.FontSize(20), template.Bold())
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
		// Verbatim, like the EPUB/DOCX backends: no TypographCJK, no
		// wrapLongLatin — code glyphs must not be mutated. gpdf's LineBreak
		// force-breaks overlong runs at character boundaries, so long lines
		// still wrap without changing any glyph.
		c.Text(strings.TrimRight(b.Text, "\n"), template.FontSize(9))
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
