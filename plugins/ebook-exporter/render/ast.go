// Package render normalizes chapter markdown into DocUnit block lists
// consumed by the ebook format backends (EPUB, PDF, DOCX).
package render

import (
	"fmt"
	"os"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// BlockKind enumerates block types the render backends consume.
type BlockKind int

const (
	BlockHeading BlockKind = iota
	BlockParagraph
	BlockQuote
	BlockList          // ordered or unordered
	BlockCode          // fenced code (info string + text); "guide" info -> dropped upstream
	BlockTable         // GFM table (rows of cells)
	BlockThematicBreak // --- / ***
)

// Block is one normalized block. Text semantics: SOURCE text with inline
// markdown syntax preserved; backends interpret inline markup later.
type Block struct {
	Kind  BlockKind
	Level int        // heading level 1-6
	Text  string     // source text, inline markdown preserved
	Items []string   // list items (each with inline markdown preserved)
	Rows  [][]string // table rows (header first)
	Align []string   // table column alignments
	Lang  string     // code fence info string
}

// DocUnit is a normalized chapter document: a flat block list.
type DocUnit struct {
	Blocks []Block
}

var md = goldmark.New(
	goldmark.WithExtensions(extension.NewCJK(), extension.Table),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

// ParseChapter reads a chapter markdown file, strips YAML frontmatter,
// parses it with goldmark and returns the normalized block list.
func ParseChapter(path string) (*DocUnit, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read chapter: %w", err)
	}
	body := stripFrontmatter(string(raw))

	doc := md.Parser().Parse(text.NewReader([]byte(body)))
	du := &DocUnit{}
	collect(doc, []byte(body), du)
	typographBlocks(du)
	return du, nil
}

// typographBlocks applies CJK punctuation normalization (export-pipeline
// memory only) to every block's inline text. Fenced code blocks stay
// verbatim; TypographCJK itself skips strings without CJK.
func typographBlocks(du *DocUnit) {
	for i := range du.Blocks {
		b := &du.Blocks[i]
		if b.Kind == BlockCode {
			continue
		}
		b.Text = TypographCJK(b.Text)
		for j, item := range b.Items {
			b.Items[j] = TypographCJK(item)
		}
		for j, row := range b.Rows {
			for k, cell := range row {
				b.Rows[j][k] = TypographCJK(cell)
			}
		}
	}
}

// ShiftHeadings shifts all heading levels by the given amount, capped at 6.
func ShiftHeadings(du *DocUnit, levels int) {
	for i := range du.Blocks {
		if du.Blocks[i].Kind == BlockHeading {
			l := du.Blocks[i].Level + levels
			if l > 6 {
				l = 6
			}
			du.Blocks[i].Level = l
		}
	}
}

// stripFrontmatter removes a leading `---\n...\n---\n` pair, if present.
func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---\n") {
		return s
	}
	rest := s[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return s
	}
	after := rest[end+4:]
	after = strings.TrimPrefix(after, "\n")
	return strings.TrimPrefix(after, "\n")
}

// collect walks the top-level children of doc, emitting blocks. Child
// traversal into blockquote/list internals is handled manually via
// WalkSkipChildren returns.
func collect(doc gast.Node, src []byte, du *DocUnit) {
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		switch n := c.(type) {
		case *gast.Heading:
			du.Blocks = append(du.Blocks, Block{Kind: BlockHeading, Level: n.Level, Text: linesText(n, src)})
		case *gast.Paragraph:
			du.Blocks = append(du.Blocks, Block{Kind: BlockParagraph, Text: linesText(n, src)})
		case *gast.Blockquote:
			du.Blocks = append(du.Blocks, Block{Kind: BlockQuote, Text: quoteText(n, src)})
		case *gast.List:
			du.Blocks = append(du.Blocks, listBlock(n, src))
		case *gast.FencedCodeBlock:
			lang := string(n.Language(src))
			if lang == "guide" {
				continue // guide metadata block: dropped entirely
			}
			du.Blocks = append(du.Blocks, Block{Kind: BlockCode, Lang: lang, Text: codeText(n, src)})
		case *gast.ThematicBreak:
			du.Blocks = append(du.Blocks, Block{Kind: BlockThematicBreak})
		default:
			if tbl, ok := c.(*east.Table); ok {
				du.Blocks = append(du.Blocks, tableBlock(tbl, src))
			}
		}
	}
}

// linesText concatenates the raw source segments of a leaf block,
// preserving inline markdown syntax. Trailing newline is trimmed.
func linesText(n gast.Node, src []byte) string {
	var sb strings.Builder
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		sb.Write(seg.Value(src))
	}
	return strings.TrimSpace(sb.String())
}

// quoteText recursively collects paragraphs inside a blockquote,
// joining multiple paragraphs with newlines.
func quoteText(n gast.Node, src []byte) string {
	var parts []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch q := c.(type) {
		case *gast.Paragraph:
			parts = append(parts, linesText(q, src))
		case *gast.Heading:
			parts = append(parts, linesText(q, src))
		case *gast.Blockquote:
			parts = append(parts, quoteText(q, src))
		}
	}
	return strings.Join(parts, "\n")
}

// listBlock walks list items, taking each item's first paragraph text.
func listBlock(n *gast.List, src []byte) Block {
	b := Block{Kind: BlockList}
	for it := n.FirstChild(); it != nil; it = it.NextSibling() {
		for p := it.FirstChild(); p != nil; p = p.NextSibling() {
			// Tight list items use TextBlock; loose ones use Paragraph.
			if para, ok := p.(*gast.Paragraph); ok {
				b.Items = append(b.Items, linesText(para, src))
				break
			}
			if tb, ok := p.(*gast.TextBlock); ok {
				b.Items = append(b.Items, linesText(tb, src))
				break
			}
		}
	}
	return b
}

// codeText returns the fenced code body verbatim.
func codeText(n *gast.FencedCodeBlock, src []byte) string {
	var sb strings.Builder
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		sb.Write(seg.Value(src))
	}
	return sb.String()
}

// cellText extracts a table cell's source text. Table cells hold inline
// Text child nodes rather than Lines.
func cellText(n *east.TableCell, src []byte) string {
	var sb strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if txt, ok := c.(*gast.Text); ok {
			sb.Write(txt.Segment.Value(src))
		}
	}
	return strings.TrimSpace(sb.String())
}

// tableBlock collects rows (header first) and per-column alignments.
func tableBlock(n *east.Table, src []byte) Block {
	b := Block{Kind: BlockTable}
	for _, a := range n.Alignments {
		b.Align = append(b.Align, a.String())
	}
	// Header cells hang directly under TableHeader; body cells sit in
	// TableRow nodes under TableBody. Walk until we find cell sequences.
	var collectCells func(parent gast.Node) []string
	collectCells = func(parent gast.Node) []string {
		var cells []string
		for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
			if tc, ok := c.(*east.TableCell); ok {
				cells = append(cells, cellText(tc, src))
			}
		}
		return cells
	}
	var walkRows func(parent gast.Node)
	walkRows = func(parent gast.Node) {
		for row := parent.FirstChild(); row != nil; row = row.NextSibling() {
			if tr, ok := row.(*east.TableRow); ok {
				if cells := collectCells(tr); cells != nil {
					b.Rows = append(b.Rows, cells)
				}
				continue
			}
			if _, isHeader := row.(*east.TableHeader); isHeader {
				if cells := collectCells(row); cells != nil {
					b.Rows = append(b.Rows, cells)
				}
				continue
			}
			walkRows(row)
		}
	}
	walkRows(n)
	return b
}
