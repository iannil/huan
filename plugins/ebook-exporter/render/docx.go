package render

import (
	"fmt"
	"strings"

	docx "github.com/mmonterroca/docxgo/v2"
	"github.com/mmonterroca/docxgo/v2/domain"
	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

// DOCXOptions controls DOCX rendering behavior. V1 has no knobs;
// reserved for future use.
type DOCXOptions struct{}

// docxFirstLineIndentTwips is the body-paragraph first-line indent
// (2 CJK characters at 12pt ≈ 480 twips).
const docxFirstLineIndentTwips = 480

// docxCodeFontHalfPoints is the code font size in half-points (9pt).
const docxCodeFontHalfPoints = 18

// docxTitleFontSizePoints is the title-page title font size in points
// (ParagraphBuilder.FontSize takes points; domain Run.SetSize takes half-points).
const docxTitleFontSizePoints = 28

// Rendering conventions (documented per brief):
//   - part titles   -> Heading1
//   - chapter titles-> Heading1 (front/back matter) or Heading2 (chapters inside a part)
//   - in-chapter headings: level 2 -> Heading2, level 3 -> Heading3, etc.,
//     clamped to Heading3 (levels below 3 are flattened since the chapter
//     itself may already occupy Heading2).
//   - quote         -> "Quote" built-in style
//   - list          -> "• "-prefixed paragraphs with ListParagraph style
//   - code          -> Normal paragraphs, 9pt, no first-line indent
//   - table         -> V1 degradation: one paragraph per row, cells
//     tab-separated (docxgo TableBuilder deliberately not used in V1)
//   - TOC           -> plain Heading1 + chapter list (Word TOC fields need a
//     refresh pass inside Word; a plain list is deterministic)

// RenderDOCX assembles the book into a DOCX file at outPath.
// The builder only creates the document skeleton and metadata; all content
// is added through the domain layer so paragraph styles are uniform.
func RenderDOCX(book *content.BookEntry, lang content.Lang, outPath string, opts DOCXOptions) error {
	title := book.TitleZH
	if lang == content.LangEN {
		title = book.TitleEN
	}
	builder := docx.NewDocumentBuilder(
		docx.WithTitle(title),
		docx.WithAuthor("iannil"),
		docx.WithDefaultFont("Noto Sans CJK SC"),
	)
	// Title-page title paragraph goes through the builder: Build()
	// validates that the document is non-empty, so at least one paragraph
	// must exist before Build. The title is rendered big via run font size
	// (the builder has no style setter).
	builder.AddParagraph().Text(title).FontSize(docxTitleFontSizePoints).End()
	doc, err := builder.Build()
	if err != nil {
		return fmt.Errorf("build docx: %w", err)
	}

	meta := make([]string, 0, 2)
	if book.Version != "" {
		meta = append(meta, book.Version)
	}
	if book.LastUpdated != "" {
		meta = append(meta, book.LastUpdated)
	}
	if len(meta) > 0 {
		if err := docxAddText(doc, strings.Join(meta, " · "), domain.StyleIDSubtitle, 0, nil); err != nil {
			return err
		}
	}

	// Plain-text table of contents.
	tocLabel := "目录"
	if lang == content.LangEN {
		tocLabel = "Contents"
	}
	if err := docxAddText(doc, tocLabel, domain.StyleIDHeading1, 0, nil); err != nil {
		return err
	}
	for _, sec := range book.OrderedSections() {
		if err := docxAddText(doc, inlinePlain(sec.Title), domain.StyleIDNormal, 0, nil); err != nil {
			return err
		}
		for _, ch := range sec.Chapters {
			if err := docxAddText(doc, "  "+inlinePlain(ch.Title), domain.StyleIDNormal, 0, nil); err != nil {
				return err
			}
		}
	}

	for _, sec := range book.OrderedSections() {
		switch sec.Type {
		case "part":
			// Part title as Heading1; its chapters as Heading2.
			if err := docxAddText(doc, inlinePlain(sec.Title), domain.StyleIDHeading1, 0, nil); err != nil {
				return err
			}
			for _, ch := range sec.Chapters {
				if err := renderDOCXChapter(doc, ch, lang, domain.StyleIDHeading2); err != nil {
					return err
				}
			}
		default:
			// introduction / epilogue / appendix: chapter titles as Heading1.
			for _, ch := range sec.Chapters {
				if err := renderDOCXChapter(doc, ch, lang, domain.StyleIDHeading1); err != nil {
					return err
				}
			}
		}
	}

	if err := doc.SaveAs(outPath); err != nil {
		return fmt.Errorf("save docx: %w", err)
	}
	return nil
}

// docxChapterTitleLevel maps an in-chapter heading level to a DOCX style ID.
// Levels are clamped to Heading2..Heading3 since Heading1 is reserved for
// parts/sections and deeper levels are flattened in V1.
func docxChapterTitleLevel(level int) string {
	if level <= 2 {
		return domain.StyleIDHeading2
	}
	return domain.StyleIDHeading3
}

// renderDOCXChapter renders one chapter: title with the given heading style
// plus the body blocks. EN chapters fall back to SourcePath when ENPath is
// unset, matching the EPUB/PDF backends.
func renderDOCXChapter(doc domain.Document, ch content.Chapter, lang content.Lang, titleStyle string) error {
	src := ch.SourcePath
	if lang == content.LangEN && ch.ENPath != "" {
		src = ch.ENPath
	}
	du, err := ParseChapter(src)
	if err != nil {
		return fmt.Errorf("parse chapter %s: %w", src, err)
	}
	if err := docxAddText(doc, inlinePlain(ch.Title), titleStyle, 0, nil); err != nil {
		return err
	}
	for _, b := range du.Blocks {
		if b.Kind == BlockThematicBreak {
			continue
		}
		if err := docxBlock(doc, b); err != nil {
			return err
		}
	}
	return nil
}

// docxBlock renders a single block into the document.
func docxBlock(doc domain.Document, b Block) error {
	switch b.Kind {
	case BlockHeading:
		return docxAddText(doc, inlinePlain(b.Text), docxChapterTitleLevel(b.Level), 0, nil)
	case BlockParagraph:
		return docxAddText(doc, inlinePlain(b.Text), domain.StyleIDNormal, docxFirstLineIndentTwips, nil)
	case BlockQuote:
		// Quote style; V1 collapses multi-paragraph quotes with line breaks.
		para, err := doc.AddParagraph()
		if err != nil {
			return fmt.Errorf("add paragraph: %w", err)
		}
		if err := para.SetStyle(domain.StyleIDQuote); err != nil {
			return fmt.Errorf("set style: %w", err)
		}
		text := inlinePlain(strings.ReplaceAll(b.Text, "\n", " "))
		return docxSetRunText(para, text)
	case BlockList:
		for _, item := range b.Items {
			if err := docxAddText(doc, "• "+inlinePlain(item), domain.StyleIDListParagraph, 0, nil); err != nil {
				return err
			}
		}
		return nil
	case BlockCode:
		// V1: no monospace guarantee; degrade to 9pt Normal paragraphs.
		for _, line := range strings.Split(b.Text, "\n") {
			if err := docxAddText(doc, line, domain.StyleIDNormal, 0, func(r domain.Run) {
				r.SetSize(docxCodeFontHalfPoints)
			}); err != nil {
				return err
			}
		}
		return nil
	case BlockTable:
		// V1 degradation: one paragraph per row, cells joined with tabs.
		for _, row := range b.Rows {
			cells := make([]string, len(row))
			for i, c := range row {
				cells[i] = inlinePlain(c)
			}
			if err := docxAddText(doc, strings.Join(cells, "\t"), domain.StyleIDNormal, 0, nil); err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

// docxAddText adds a paragraph with the given style, optional first-line
// indent (twips), and optional run formatting hook.
func docxAddText(doc domain.Document, text, style string, firstLineTwips int, formatRun func(domain.Run)) error {
	para, err := doc.AddParagraph()
	if err != nil {
		return fmt.Errorf("add paragraph: %w", err)
	}
	if style != "" && style != domain.StyleIDNormal {
		if err := para.SetStyle(style); err != nil {
			return fmt.Errorf("set style %s: %w", style, err)
		}
	}
	if firstLineTwips > 0 {
		if err := para.SetIndentFirstLine(firstLineTwips); err != nil {
			return fmt.Errorf("set indent: %w", err)
		}
	}
	if err := docxSetRunText(para, text); err != nil {
		return err
	}
	if formatRun != nil {
		runs := para.Runs()
		if len(runs) > 0 {
			formatRun(runs[len(runs)-1])
		}
	}
	return nil
}

// docxSetRunText appends a single run with the given text to the paragraph.
func docxSetRunText(para domain.Paragraph, text string) error {
	run, err := para.AddRun()
	if err != nil {
		return fmt.Errorf("add run: %w", err)
	}
	if err := run.SetText(text); err != nil {
		return fmt.Errorf("set text: %w", err)
	}
	return nil
}
