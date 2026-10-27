package markdown

import (
	"bytes"
	"html"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Math preserves TeX as HTML-escaped text for a client-side math renderer.
// It is shared by page rendering and template markdownify. Code blocks and
// spans remain owned by goldmark; no source-level replacements are performed.
var Math goldmark.Extender = mathExtension{}

type mathExtension struct{}

func (mathExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithInlineParsers(util.Prioritized(mathInlineParser{}, 150)),
		parser.WithBlockParsers(util.Prioritized(mathBlockParser{}, 750)),
	)
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(mathRenderer{}, 100)))
}

var kindMathInline = ast.NewNodeKind("MathInline")
var kindMathBlock = ast.NewNodeKind("MathBlock")

type mathInline struct {
	ast.BaseInline
	value []byte
}

func (*mathInline) Kind() ast.NodeKind              { return kindMathInline }
func (n *mathInline) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type mathBlock struct {
	ast.BaseBlock
	close []byte
}

func (*mathBlock) Kind() ast.NodeKind              { return kindMathBlock }
func (*mathBlock) IsRaw() bool                     { return true }
func (n *mathBlock) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

func mathDelimiters(line []byte) (open, close []byte) {
	for _, pair := range [][2]string{{`\(`, `\)`}, {`\[`, `\]`}, {`$$`, `$$`}, {`$`, `$`}} {
		if bytes.HasPrefix(line, []byte(pair[0])) {
			return []byte(pair[0]), []byte(pair[1])
		}
	}
	return nil, nil
}

func mathEscaped(s []byte, pos int) bool {
	n := 0
	for pos > 0 && s[pos-1] == '\\' {
		n++
		pos--
	}
	return n%2 != 0
}

func mathSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }

type mathInlineParser struct{}

func (mathInlineParser) Trigger() []byte { return []byte{'\\', '$'} }
func (mathInlineParser) Parse(parent ast.Node, r text.Reader, pc parser.Context) ast.Node {
	line, _ := r.PeekLine()
	open, close := mathDelimiters(line)
	if open == nil {
		return nil
	}
	singleDollar := len(open) == 1
	if singleDollar && (len(line) <= 1 || mathSpace(line[1])) {
		return nil
	}
	startLine, startPos := r.Position()
	value := append([]byte(nil), open...)
	r.Advance(len(open))
	for {
		line, _ = r.PeekLine()
		if len(line) == 0 {
			break
		}
		for i := 0; i+len(close) <= len(line); i++ {
			if !bytes.HasPrefix(line[i:], close) || mathEscaped(line, i) {
				continue
			}
			if singleDollar {
				// Pandoc-style dollar boundaries: no whitespace before the close and
				// no digit after it. Thus "$5 and $10" remains ordinary prose.
				if (i == 0 && len(value) == 1) || (i > 0 && mathSpace(line[i-1])) {
					continue
				}
				after := i + 1
				if after < len(line) && line[after] >= '0' && line[after] <= '9' {
					continue
				}
			}
			value = append(value, line[:i+len(close)]...)
			r.Advance(i + len(close))
			return &mathInline{value: value}
		}
		if singleDollar {
			break
		}
		value = append(value, line...)
		r.AdvanceLine()
	}
	r.SetPosition(startLine, startPos)
	return nil
}

type mathBlockParser struct{}

func (mathBlockParser) Trigger() []byte { return []byte{'\\', '$'} }
func (mathBlockParser) Open(parent ast.Node, r text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := r.PeekLine()
	pos := pc.BlockIndent()
	if pos < 0 || pos >= len(line) {
		return nil, parser.NoChildren
	}
	open, close := mathDelimiters(line[pos:])
	if !(bytes.Equal(open, []byte(`\[`)) || bytes.Equal(open, []byte(`$$`))) || !util.IsBlank(line[pos+len(open):]) {
		return nil, parser.NoChildren
	}
	// Only claim a block with a closing delimiter. Otherwise ordinary Markdown
	// remains available, rather than consuming the rest of an incomplete page.
	hasClose := false
	for _, candidate := range bytes.Split(r.Source()[segment.Stop:], []byte{'\n'}) {
		// Container markers are stripped by goldmark when Continue is called.
		// Account for blockquotes during lookahead as well.
		candidate = bytes.TrimLeft(candidate, " \t>")
		if bytes.Equal(bytes.TrimSpace(candidate), close) {
			hasClose = true
			break
		}
	}
	if !hasClose {
		return nil, parser.NoChildren
	}
	n := &mathBlock{close: close}
	n.Lines().Append(segment.WithStart(segment.Start + pos))
	r.AdvanceToEOL()
	return n, parser.NoChildren
}
func (mathBlockParser) Continue(node ast.Node, r text.Reader, pc parser.Context) parser.State {
	n := node.(*mathBlock)
	line, segment := r.PeekLine()
	n.Lines().Append(segment)
	r.AdvanceToEOL()
	if bytes.Equal(bytes.TrimSpace(line), n.close) {
		return parser.Close
	}
	return parser.Continue | parser.NoChildren
}
func (mathBlockParser) Close(ast.Node, text.Reader, parser.Context) {}
func (mathBlockParser) CanInterruptParagraph() bool                 { return true }
func (mathBlockParser) CanAcceptIndentedLine() bool                 { return false }

type mathRenderer struct{}

func (mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMathInline, renderMath)
	reg.Register(kindMathBlock, renderMath)
}
func renderMath(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	if n, ok := node.(*mathInline); ok {
		_, err := w.WriteString(html.EscapeString(string(n.value)))
		return ast.WalkSkipChildren, err
	}
	if _, err := w.WriteString("<div class=\"math\">\n"); err != nil {
		return ast.WalkStop, err
	}
	for i := 0; i < node.Lines().Len(); i++ {
		segment := node.Lines().At(i)
		if _, err := w.WriteString(html.EscapeString(string(segment.Value(source)))); err != nil {
			return ast.WalkStop, err
		}
	}
	_, err := w.WriteString("</div>\n")
	return ast.WalkSkipChildren, err
}
