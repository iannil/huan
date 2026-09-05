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

// monoFontFamily is the family name under which the optional monospaced code
// font is registered (PDF-1). Empty MonoFontPath leaves it unregistered and
// emitBlock's FontFamily option falls back to the document default.
const monoFontFamily = "monocode"

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

// lineHeight returns a TextOption that sets the line-height multiplier on the
// text style. gpdf's template package (v1.0.11) does not expose a LineHeight
// setter, but TextOption is func(*document.Style) and Style.LineHeight is an
// exported field, so a local helper can set it without forking gpdf. Body
// text uses the approved 12pt × 1.85 = 22.2pt leading; code and tables
// use 1.6 leading.
func lineHeight(v float64) template.TextOption {
	return func(s *document.Style) { s.LineHeight = v }
}

// PDFOptions controls PDF rendering. FontPath points at a CJK-capable font
// (required for Chinese books; may be empty for English ones, in which case
// gpdf's built-in fonts are used). MonoFontPath, when non-empty, is a
// monospaced TrueType font registered under the "mono" family and used for
// code blocks (PDF-1); when empty, code blocks fall back to the body font.
type PDFOptions struct {
	FontPath string
	// MonoFontPath registers a monospaced font for code blocks.
	MonoFontPath string
	CoverFonts   CoverFonts
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
	renderTitlePage(doc, book, lang)
	renderCopyrightPage(doc, book, lang)
	renderTOC(doc, book, lang, plan)
	if err := renderChapters(doc, book, lang, opts); err != nil {
		return err
	}
	renderBackmatter(doc, book, lang)

	data, err := doc.Generate()
	if err != nil {
		return fmt.Errorf("render pdf: %w", err)
	}
	cover, err := publicationCover(book, lang, opts.CoverFonts)
	if err != nil {
		return fmt.Errorf("publication cover: %w", err)
	}
	data, err = finishPublicationPDF(data, cover, book, lang, plan)
	if err != nil {
		return fmt.Errorf("publication pages: %w", err)
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
		template.WithMargins(document.UniformEdges(document.Mm(26))),
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
	if opts.MonoFontPath != "" {
		data, err := style.ReadFontData(opts.MonoFontPath)
		if err != nil {
			return nil, fmt.Errorf("read mono font: %w", err)
		}
		options = append(options, template.WithFont(monoFontFamily, data))
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

// renderTitlePage emits the title page (page 2 in publication order): book
// title, subtitle, and author/publisher credit. The title page follows the
// cover and must have low text density (<200 chars, satisfying typo.pdf.titlepage).
func renderTitlePage(doc *template.Document, book *content.BookEntry, lang content.Lang) {
	title := book.TitleZH
	subtitle := book.SubtitleZH
	credit := "祝融 著"
	if lang == content.LangEN {
		title = book.TitleEN
		subtitle = ""
		credit = "By Rong Zhu"
	}
	page := doc.AddPage()
	row := func(fn func(c *template.ColBuilder)) {
		page.AutoRow(func(r *template.RowBuilder) {
			r.Col(12, fn)
		})
	}
	row(func(c *template.ColBuilder) { c.Spacer(document.Pt(140)) })
	row(func(c *template.ColBuilder) {
		c.Text(TypographCJK(title), template.FontSize(24), template.Bold(), template.AlignCenter())
	})
	if subtitle != "" {
		row(func(c *template.ColBuilder) { c.Spacer(document.Pt(16)) })
		row(func(c *template.ColBuilder) {
			c.Text(inlinePlain(subtitle), template.FontSize(13), template.AlignCenter())
		})
	}
	row(func(c *template.ColBuilder) { c.Spacer(document.Pt(60)) })
	row(func(c *template.ColBuilder) {
		c.Text(credit, template.FontSize(12), template.AlignCenter())
	})
	row(func(c *template.ColBuilder) { c.Spacer(document.Pt(20)) })
	pub := "祝融说 · zhurongshuo.com"
	if lang == content.LangEN {
		pub = "zhurongshuo.com"
	}
	row(func(c *template.ColBuilder) {
		c.Text(pub, template.FontSize(10), template.AlignCenter())
	})
}

// renderCopyrightPage emits the copyright page (page 3 in publication order):
// contains standard copyright keywords (版权/©/Copyright/版次) satisfying
// typo.pdf.copyright.
func renderCopyrightPage(doc *template.Document, book *content.BookEntry, lang content.Lang) {
	page := doc.AddPage()
	row := func(fn func(c *template.ColBuilder)) {
		page.AutoRow(func(r *template.RowBuilder) {
			r.Col(12, fn)
		})
	}
	row(func(c *template.ColBuilder) { c.Spacer(document.Pt(220)) })
	row(func(c *template.ColBuilder) {
		title := book.TitleZH
		if lang == content.LangEN && book.TitleEN != "" {
			title = book.TitleEN
		}
		c.Text(TypographCJK(title), template.FontSize(14), template.Bold(), template.AlignCenter())
	})
	row(func(c *template.ColBuilder) { c.Spacer(document.Pt(20)) })
	if lang == content.LangEN {
		row(func(c *template.ColBuilder) {
			c.Text("Copyright © 2010–2026 Rong Zhu", template.FontSize(10), template.AlignCenter())
		})
		row(func(c *template.ColBuilder) {
			c.Text("All rights reserved · zhurongshuo.com", template.FontSize(9), template.AlignCenter())
		})
		row(func(c *template.ColBuilder) { c.Spacer(document.Pt(16)) })
		row(func(c *template.ColBuilder) {
			c.Text("Edition: "+book.Version+" · Updated: "+book.LastUpdated, template.FontSize(9), template.AlignCenter())
		})
	} else {
		row(func(c *template.ColBuilder) {
			c.Text("版权所有 © 2010–2026 祝融", template.FontSize(10), template.AlignCenter())
		})
		row(func(c *template.ColBuilder) {
			c.Text("保留一切权利 · zhurongshuo.com", template.FontSize(9), template.AlignCenter())
		})
		row(func(c *template.ColBuilder) { c.Spacer(document.Pt(16)) })
		row(func(c *template.ColBuilder) {
			c.Text("版次："+book.Version+" · 更新日期："+book.LastUpdated, template.FontSize(9), template.AlignCenter())
		})
	}
}

// renderBackmatter emits the final page (PDF-2): centered book-closing marker
// satisfying typo.pdf.backmatter ("全书完" / "The End").
func renderBackmatter(doc *template.Document, book *content.BookEntry, lang content.Lang) {
	page := doc.AddPage()
	row := func(fn func(c *template.ColBuilder)) {
		page.AutoRow(func(r *template.RowBuilder) {
			r.Col(12, fn)
		})
	}
	row(func(c *template.ColBuilder) { c.Spacer(document.Pt(260)) })
	label := "全书完"
	if lang == content.LangEN {
		label = "The End"
	}
	row(func(c *template.ColBuilder) {
		c.Text(label, template.FontSize(18), template.Bold(), template.AlignCenter())
	})
	row(func(c *template.ColBuilder) { c.Spacer(document.Pt(20)) })
	site := "祝融说 · zhurongshuo.com"
	if lang == content.LangEN {
		site = "zhurongshuo.com"
	}
	row(func(c *template.ColBuilder) {
		c.Text(site, template.FontSize(10), template.AlignCenter())
	})
}

// renderCover emits the cover page: title, subtitle and version metadata.
//
// Spacer rows and centered-text rows are separate AutoRows: batching a
// Spacer and an AlignCenter Text into one row makes gpdf place the text
// off the page top-right corner (invisible covers / overflowing part
// pages).
func renderCover(doc *template.Document, book *content.BookEntry, lang content.Lang) {
	// Reserve exactly one page; shared publication artwork is installed after pagination.
	doc.AddPage().AutoRow(func(r *template.RowBuilder) { r.Col(12, func(c *template.ColBuilder) { c.Spacer(document.Pt(1)) }) })
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
		if err := renderChapterInto(doc, ch, lang, opts.MonoFontPath); err != nil {
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

	// Walk sections assigning start pages: cover=1, titlepage=2, copyright=3,
	// TOC occupies 4..3+tocPages, then parts (one separator page each) and
	// chapters (measured counts).
	page := 4 + plan.tocPages
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
	tocTitle := "目录"
	if lang == content.LangEN {
		tocTitle = "Contents"
	}
	row(func(c *template.ColBuilder) {
		c.Text(tocTitle, template.FontSize(20), template.Bold(), lineHeight(1.5))
	})
	row(func(c *template.ColBuilder) { c.Spacer(document.Pt(8)) })
	ci := 0
	for _, sec := range book.OrderedSections() {
		if sec.Type == "part" {
			partTitle := sec.Title
			row(func(c *template.ColBuilder) {
				c.Text(inlinePlain(partTitle), template.FontSize(13), template.Bold(), lineHeight(1.5))
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
					c.Text("    "+inlinePlain(title), template.FontSize(12), lineHeight(1.85))
				},
				func(c *template.ColBuilder) {
					if num != "" {
						c.Text(num, template.FontSize(11), template.AlignRight(), lineHeight(1.5))
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
//
// PDF-2: each part separator is followed by a new part-title page (thin
// separator; count+1).  PDF-3: BlockParagraph gets a 2-character first-line
// indent for Chinese books.  PDF-4: headings get a pre-spacing spacer so
// they are not crammed against the preceding paragraph.
func renderChapters(doc *template.Document, book *content.BookEntry, lang content.Lang, opts PDFOptions) error {
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
			if err := renderChapterInto(doc, ch, lang, opts.MonoFontPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// renderChapterInto emits one chapter on a fresh page: title row then one
// AutoRow per normalized block. Shared by the final render and the
// measurement pass so both paginate identically (see measurePDFPages).
// monoFontPath is threaded to emitBlock for the code-block font (PDF-1).
func renderChapterInto(doc *template.Document, ch content.Chapter, lang content.Lang, monoFontPath string) error {
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
			c.Text(inlinePlain(ch.Title), template.FontSize(20), template.Bold(), lineHeight(1.5))
		})
	})
	for i := range unit.Blocks {
		b := unit.Blocks[i]
		// Keep a list item or table row atomic, rather than putting an entire
		// multi-page list/table inside one AutoRow. gpdf's split of a large
		// AutoRow can duplicate and widen its boundary line beyond the page.
		if b.Kind == BlockList {
			for _, item := range b.Items {
				itemBlock := b
				itemBlock.Items = []string{item}
				page.AutoRow(func(r *template.RowBuilder) {
					r.Col(12, func(c *template.ColBuilder) { emitBlockOpts(c, itemBlock, monoFontPath, false) })
				})
			}
			continue
		}
		if b.Kind == BlockTable {
			for _, cells := range b.Rows {
				rowBlock := b
				rowBlock.Rows = [][]string{cells}
				page.AutoRow(func(r *template.RowBuilder) {
					r.Col(12, func(c *template.ColBuilder) { emitBlockOpts(c, rowBlock, monoFontPath, false) })
				})
			}
			continue
		}
		// PDF-4: headings get pre-space only when they follow another block
		// (a heading at the top of the page must not be pushed down).
		lead := b.Kind == BlockHeading && i > 0
		page.AutoRow(func(r *template.RowBuilder) {
			r.Col(12, func(c *template.ColBuilder) {
				emitBlockOpts(c, b, monoFontPath, lead)
			})
		})
	}
	return nil
}

// emitBlock maps one normalized block onto PDF text lines.
//
// monoFontPath (PDF-1) selects the registered mono family for code blocks;
// empty means no mono font was registered and the document default applies.
// Paragraph blocks carry a 2-character first-line indent (PDF-3, 中文出版惯例
// 首行缩进两字；EN 版保持同一缩进——西文出版同样使用首行缩进)；标题块带
// 段前距 spacer（PDF-4），由调用方通过 headingLead 参数注入。
func emitBlock(c *template.ColBuilder, b Block, monoFontPath string) {
	emitBlockOpts(c, b, monoFontPath, false)
}

// emitBlockOpts is emitBlock with heading pre-space control (PDF-4).
// headingLead inserts a spacer above the heading so the gap between the
// preceding paragraph's bottom edge and the heading top is ≈0.8×line height.
func emitBlockOpts(c *template.ColBuilder, b Block, monoFontPath string, headingLead bool) {
	switch b.Kind {
	case BlockHeading:
		// PDF-4: pre-heading spacer. gpdf AutoRows have no margins, so the
		// gap is materialized as a Spacer in the same column. 10pt spacer +
		// 1.5 line-height ≈ 0.8 × 行高 of visual gap.
		if headingLead {
			c.Spacer(document.Pt(16))
		}
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
		c.Text(pdfText(b.Text), template.FontSize(size), template.Bold(), lineHeight(1.65))
	case BlockParagraph:
		// PDF-3: 2-char first-line indent (2 × 11pt = 22pt ≈ 7.76mm).
		c.Text(pdfText(b.Text), template.FontSize(12), lineHeight(1.85), template.TextIndent(document.Pt(24)))
	case BlockQuote:
		// PDF-7 版式：引用块不用 ▎ 前导（U+258E 不在 Noto CJK cmap 中，
		// 渲染为 .notdef 豆腐块；EN 正文为全 Latin 无 CJK 时不归一也无此
		// 字形）。改为出版惯例的灰色引文色 + 左缘条：颜色用浅灰
		// （#767676），左缘条 1.5pt 紧贴文本左缘（TextPadding 在 gpdf
		// 渲染层不消费 Style.Padding，条是可见的 BoxStyle 边框而非位移）。
		quoteColor := pdf.RGBHex(0x767676)
		c.Spacer(document.Pt(3))
		c.Text(" "+pdfText(b.Text),
			template.FontSize(12), lineHeight(1.85),
			template.TextColor(quoteColor),
			template.TextPadding(document.UniformEdges(document.Pt(4))),
			template.WithTextBorder(template.Border(
				template.BorderWidths(document.Pt(0), document.Pt(0), document.Pt(0), document.Pt(1.5)),
				template.BorderColor(quoteColor),
			)))
	case BlockList:
		for _, item := range b.Items {
			c.Text("• "+pdfText(item), template.FontSize(12), lineHeight(1.85))
		}
	case BlockCode:
		// Verbatim, like the EPUB/DOCX backends: no TypographCJK, no
		// wrapLongLatin — code glyphs must not be mutated. gpdf's LineBreak
		// force-breaks overlong runs at character boundaries, so long lines
		// still wrap without changing any glyph. monoFontPath (PDF-1) swaps
		// in the registered mono family; when empty the document default
		// (body) font applies at 9pt.
		//
		// CJK fallback (PDF-1b): gpdf has no per-glyph font fallback — a
		// rune missing from the current family's cmap renders as .notdef.
		// Pure-mono fonts (JetBrains Mono 等) carry no CJK glyphs, so lines
		// containing CJK (中文注释/字符串) would turn into tofu. Those lines
		// render in the document default (CJK body) font at 9pt instead;
		// only all-ASCII lines keep the mono family. The auditor's mono
		// 契约 (typo.pdf.block.styles) counts CJK code lines via span
		// flags, which the CJK font satisfies.
		for _, line := range strings.Split(strings.TrimRight(b.Text, "\n"), "\n") {
			family := monoFontFamily
			if monoFontPath == "" || strings.ContainsFunc(line, isCJK) {
				family = ""
			}
			opts := []template.TextOption{template.FontSize(9.5), lineHeight(1.6)}
			if family != "" {
				opts = append(opts, template.FontFamily(family))
			}
			c.Text(line, opts...)
		}
	case BlockTable:
		for _, row := range b.Rows {
			cells := make([]string, len(row))
			for i, cell := range row {
				cells[i] = pdfText(cell)
			}
			c.Text(strings.Join(cells, " | "), template.FontSize(10.5), lineHeight(1.6))
		}
	case BlockThematicBreak:
		c.Text("———", template.FontSize(11))
	}
}
