package render

import (
	"encoding/base64"
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-shiori/go-epub"
	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

// escapeHTML escapes a source string for safe XHTML embedding.
func escapeHTML(s string) string { return html.EscapeString(s) }

// uniqueFname returns fname, or fname-2, fname-3, … if fname was already
// used within this epub. It records the returned name as used.
func uniqueFname(fname string, used map[string]bool) string {
	if !used[fname] {
		used[fname] = true
		return fname
	}
	for n := 2; ; n++ {
		cand := fmt.Sprintf("%s-%d", fname, n)
		if !used[cand] {
			used[cand] = true
			return cand
		}
	}
}

// linkRe matches a markdown inline link: [text](url). Both parts are
// already HTML-escaped when applied to escaped input.
var linkRe = regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)

// replaceDelimited converts pairs of delim-wrapped spans into open/close
// tags. Escaped source input; minimal, non-nested handling. When the
// delimiter count is odd, delimiters are left literal to avoid emitting
// unbalanced tags.
func replaceDelimited(s, delim, open, close string) string {
	count := strings.Count(s, delim)
	if count < 2 || count%2 != 0 {
		return s
	}
	var sb strings.Builder
	isOpen := true
	for {
		idx := strings.Index(s, delim)
		if idx < 0 {
			sb.WriteString(s)
			break
		}
		sb.WriteString(s[:idx])
		if isOpen {
			sb.WriteString(open)
		} else {
			sb.WriteString(close)
		}
		isOpen = !isOpen
		s = s[idx+len(delim):]
	}
	return sb.String()
}

// protectDelimited replaces pairs of delim-wrapped spans with placeholder
// tokens; placeholder receives the span's content and returns the token to
// substitute (typically recording the rendered form as a side effect). Like
// replaceDelimited, odd delimiter counts leave delimiters literal.
func protectDelimited(s, delim string, placeholder func(content string) string) string {
	count := strings.Count(s, delim)
	if count < 2 || count%2 != 0 {
		return s
	}
	var sb strings.Builder
	for {
		idx := strings.Index(s, delim)
		if idx < 0 {
			sb.WriteString(s)
			break
		}
		rest := s[idx+len(delim):]
		end := strings.Index(rest, delim)
		sb.WriteString(s[:idx])
		sb.WriteString(placeholder(rest[:end]))
		s = rest[end+len(delim):]
	}
	return sb.String()
}

// replaceUnderscoreEm converts _emphasis_ pairs to <em> tags honoring the
// CommonMark flanking rule for underscores: an OPENER must have a word
// character after it and a non-word character before it; a CLOSER must
// have a word character before it and a non-word character after it.
// Intra-word underscores (NEEDS_FIX, snake_case) are therefore left
// literal — alternation-based pairing without this rule opens an <em>
// inside one **strong** span and closes it inside another, producing
// mismatched (not well-formed) XHTML. Additionally an opener is only
// emitted when a matching closer exists LATER in the string, so an
// opener-shaped underscore with no partner (math subscripts like
// \text{Error}_i, blank-fill underscores) never emits an unbalanced tag.
// Word characters: ASCII alphanumerics, '_' and CJK ideographs (goldmark
// treats CJK as word characters for '_' flanking); fullwidth punctuation
// is not a word character. Input is already HTML-escaped.
func replaceUnderscoreEm(s string) string {
	rs := []rune(s)
	isWord := func(r rune) bool {
		switch {
		case r == '_', r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			return true
		case r >= 0x4E00 && r <= 0x9FFF, r >= 0x3400 && r <= 0x4DBF, r >= 0xF900 && r <= 0xFAFF:
			return true // CJK ideographs
		}
		return false
	}
	type kindT int
	const (
		literal kindT = iota
		opener
		closer
	)
	kinds := make([]kindT, len(rs))
	for i, r := range rs {
		if r != '_' {
			continue
		}
		wordBefore := i > 0 && isWord(rs[i-1])
		wordAfter := i+1 < len(rs) && isWord(rs[i+1])
		switch {
		case !wordBefore && wordAfter:
			kinds[i] = opener
		case wordBefore && !wordAfter:
			kinds[i] = closer
		}
	}
	// Pair each opener with the first unconsumed closer after it; unmatched
	// delimiters stay literal so tags are always balanced.
	emitted := make([]bool, len(rs)) // true → emit tag at i
	var sb strings.Builder
	for i, r := range rs {
		if r != '_' || kinds[i] != opener {
			continue
		}
		for j := i + 1; j < len(rs); j++ {
			if rs[j] == '_' && kinds[j] == closer && !emitted[j] {
				emitted[i] = true
				emitted[j] = true
				break
			}
		}
	}
	for i, r := range rs {
		if r == '_' && emitted[i] {
			if kinds[i] == opener {
				sb.WriteString("<em>")
			} else {
				sb.WriteString("</em>")
			}
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// EPUBOptions controls EPUB rendering behavior.
type EPUBOptions struct {
	// EmbedFont embeds a CJK font into the EPUB (larger file, portable).
	EmbedFont bool
	// FontPath is the path of the font file to embed.
	FontPath string
}

// titlePageXHTML renders the book title page (spine start).
func titlePageXHTML(book *content.BookEntry, lang content.Lang) string {
	title := TypographCJK(book.TitleZH)
	subtitle := TypographCJK(book.SubtitleZH)
	if lang == content.LangEN {
		title = TypographCJK(book.TitleEN)
		// V1: no EN subtitle field exists (BookEntry has SubtitleZH only),
		// so the zh subtitle is not shown on EN title pages.
		subtitle = ""
	}
	var sb strings.Builder
	sb.WriteString("<h1>" + escapeHTML(title) + "</h1>")
	if subtitle != "" {
		sb.WriteString("<p class=\"subtitle\">" + escapeHTML(subtitle) + "</p>")
	}
	meta := make([]string, 0, 2)
	if book.Version != "" {
		meta = append(meta, escapeHTML(book.Version))
	}
	if book.LastUpdated != "" {
		meta = append(meta, escapeHTML(book.LastUpdated))
	}
	if len(meta) > 0 {
		sb.WriteString("<p class=\"meta\">" + strings.Join(meta, " · ") + "</p>")
	}
	return sb.String()
}

// RenderEPUB assembles the book into an EPUB file at outPath.
func RenderEPUB(book *content.BookEntry, lang content.Lang, outPath string, opts EPUBOptions) error {
	title := book.TitleZH
	if lang == content.LangEN {
		title = book.TitleEN
	}
	e, err := epub.NewEpub(TypographCJK(title))
	if err != nil {
		return fmt.Errorf("create epub: %w", err)
	}
	e.SetAuthor("iannil")
	if lang == content.LangEN {
		e.SetLang("en")
	} else {
		e.SetLang("zh-CN")
	}

	// Embed font first so its internal path can be referenced in CSS.
	fontRef := ""
	if opts.EmbedFont && opts.FontPath != "" {
		internal, ferr := e.AddFont(opts.FontPath, filepath.Base(opts.FontPath))
		if ferr != nil {
			return fmt.Errorf("embed font: %w", ferr)
		}
		fontRef = internal
	}

	cssPath, cerr := e.AddCSS(dataURL("text/css", epubCSS(lang, fontRef)), "style.css")
	if cerr != nil {
		return fmt.Errorf("add css: %w", cerr)
	}

	// Title page as the first section (spine start).
	if _, err := e.AddSection(titlePageXHTML(book, lang), "", "title", cssPath); err != nil {
		return fmt.Errorf("add title section: %w", err)
	}

	// usedFnames tracks internal section filenames so units that combine many
	// books (volume / complete) never reuse one — go-epub errors on duplicate
	// filenames when multiple books each contribute an "introduction".
	usedFnames := map[string]bool{}

	for _, sec := range book.OrderedSections() {
		switch sec.Type {
		case "part":
			// Part separator page, then each chapter as its own section.
			secTitle := TypographCJK(sec.Title)
			body := "<h1>" + escapeHTML(secTitle) + "</h1>"
			if _, err := e.AddSection(body, secTitle, uniqueFname("part-"+sec.ID, usedFnames), cssPath); err != nil {
				return fmt.Errorf("add part section: %w", err)
			}
			for i, ch := range sec.Chapters {
				src := ch.SourcePath
				if lang == content.LangEN && ch.ENPath != "" {
					src = ch.ENPath
				}
				b, perr := ParseChapter(src)
				if perr != nil {
					return fmt.Errorf("parse chapter %s: %w", src, perr)
				}
				chTitle := TypographCJK(ch.Title)
				cb := "<h1>" + escapeHTML(chTitle) + "</h1>" + blocksToXHTML(b)
				fname := fmt.Sprintf("%s-ch%02d", sec.ID, i+1)
				if _, err := e.AddSection(cb, chTitle, uniqueFname(fname, usedFnames), cssPath); err != nil {
					return fmt.Errorf("add chapter section: %w", err)
				}
			}
		default:
			// introduction / epilogue / appendix: single section per entry.
			for i, ch := range sec.Chapters {
				src := ch.SourcePath
				if lang == content.LangEN && ch.ENPath != "" {
					src = ch.ENPath
				}
				b, perr := ParseChapter(src)
				if perr != nil {
					return fmt.Errorf("parse chapter %s: %w", src, perr)
				}
				chTitle := TypographCJK(ch.Title)
				body := "<h1>" + escapeHTML(chTitle) + "</h1>" + blocksToXHTML(b)
				fname := sec.Type
				if fname == "" {
					fname = "section"
				}
				if len(sec.Chapters) > 1 {
					fname = fmt.Sprintf("%s-ch%02d", fname, i+1)
				}
				fname = uniqueFname(fname, usedFnames)
				if _, err := e.AddSection(body, chTitle, fname, cssPath); err != nil {
					return fmt.Errorf("add section %s: %w", fname, err)
				}
			}
		}
	}

	if err := e.Write(outPath); err != nil {
		return fmt.Errorf("write epub: %w", err)
	}
	return nil
}

// blocksToXHTML converts a DocUnit's blocks into an XHTML fragment.
// Lists are rendered as <ul> (DocUnit does not carry ordered-ness in V1).
func blocksToXHTML(du *DocUnit) string {
	var sb strings.Builder
	for _, b := range du.Blocks {
		switch b.Kind {
		case BlockHeading:
			lvl := b.Level
			if lvl < 1 {
				lvl = 1
			}
			if lvl > 6 {
				lvl = 6
			}
			fmt.Fprintf(&sb, "<h%d>%s</h%d>\n", lvl, inlineXHTML(b.Text), lvl)
		case BlockParagraph:
			sb.WriteString("<p>" + inlineXHTML(b.Text) + "</p>\n")
		case BlockQuote:
			var qs strings.Builder
			for i, para := range strings.Split(b.Text, "\n") {
				if i > 0 {
					qs.WriteString("<br/>\n")
				}
				qs.WriteString(inlineXHTML(para))
			}
			sb.WriteString("<blockquote>\n<p>" + qs.String() + "</p>\n</blockquote>\n")
		case BlockList:
			sb.WriteString("<ul>\n")
			for _, item := range b.Items {
				sb.WriteString("<li>" + inlineXHTML(item) + "</li>\n")
			}
			sb.WriteString("</ul>\n")
		case BlockCode:
			sb.WriteString("<pre><code>" + escapeHTML(strings.TrimRight(b.Text, "\n")) + "</code></pre>\n")
		case BlockTable:
			sb.WriteString("<table>\n")
			for i, row := range b.Rows {
				sb.WriteString("<tr>")
				tag := "td"
				if i == 0 {
					tag = "th"
				}
				for _, cell := range row {
					sb.WriteString("<" + tag + ">" + inlineXHTML(cell) + "</" + tag + ">")
				}
				sb.WriteString("</tr>\n")
			}
			sb.WriteString("</table>\n")
		case BlockThematicBreak:
			sb.WriteString("<hr/>\n")
		}
	}
	return sb.String()
}

// **strong**, *em* / _em_, `code`, [text](url).
//
// Code spans are protected FIRST (placeholder tokens): their content stays
// HTML-escaped (raw HTML like <em> in a code span renders as &lt;em&gt;)
// and emphasis delimiters inside them stay literal — an <em> must never
// open inside a <code> and close outside it, which produced mismatched
// tags. Links are extracted next so `_`/`*` inside URLs survive untouched;
// the link TEXT still goes through the emphasis rules.
func inlineXHTML(s string) string {
	s = TypographCJK(s)
	s = escapeHTML(s)
	var saved []string
	hold := func(rendered string) string {
		saved = append(saved, rendered)
		return fmt.Sprintf("\x00%d\x00", len(saved)-1)
	}
	// Protect code spans before any emphasis rule can corrupt them.
	s = protectDelimited(s, "`", func(content string) string {
		return hold("<code>" + content + "</code>")
	})

	// Protect links before other delimiters corrupt URLs.
	s = linkRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := linkRe.FindStringSubmatch(m)
		text := replaceDelimited(parts[1], "**", "<strong>", "</strong>")
		text = replaceDelimited(text, "*", "<em>", "</em>")
		text = replaceUnderscoreEm(text)
		rendered := `<a href="` + parts[2] + `">` + text + `</a>`
		return hold(rendered)
	})
	s = replaceDelimited(s, "**", "<strong>", "</strong>")
	s = replaceDelimited(s, "*", "<em>", "</em>")
	s = replaceUnderscoreEm(s)
	for i, l := range saved {
		s = strings.ReplaceAll(s, fmt.Sprintf("\x00%d\x00", i), l)
	}
	return s
}

// epubCSS builds the stylesheet. fontRef is the internal font path returned
// by AddFont (empty when no font is embedded).
func epubCSS(lang content.Lang, fontRef string) string {
	css := `body { font-family: "Noto Sans CJK SC", serif; line-height: 1.7; margin: 1em; }
h1 { font-size: 1.6em; margin: 1.5em 0 0.8em; page-break-before: always; }
h2 { font-size: 1.3em; margin: 1.2em 0 0.6em; }
h3 { font-size: 1.1em; margin: 1em 0 0.5em; }
p { text-indent: 2em; margin: 0.5em 0; }
p.subtitle, p.meta { text-indent: 0; text-align: center; color: #666; }
blockquote { margin: 0.8em 1.5em; color: #555; border-left: 3px solid #ccc; padding-left: 0.8em; }
pre { background: #f5f5f5; padding: 0.8em; overflow-x: auto; font-size: 0.9em; }
table { border-collapse: collapse; margin: 1em 0; }
td, th { border: 1px solid #999; padding: 0.4em 0.8em; }
`
	if lang == content.LangEN {
		css += "body { font-family: serif; }\np { text-indent: 0; }\n"
	}
	if fontRef != "" {
		css += fmt.Sprintf("@font-face { font-family: \"EmbeddedCJK\"; src: url(%s); }\nbody { font-family: \"EmbeddedCJK\", serif; }\n", fontRef)
	}
	return css
}

// dataURL encodes content as a base64 data: URL.
func dataURL(mime, content string) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString([]byte(content))
}
