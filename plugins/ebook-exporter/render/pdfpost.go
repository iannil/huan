package render

import (
	"fmt"
	"unicode/utf16"

	"github.com/gpdf-dev/gpdf/pdf"
)

// OutlineEntry is one bookmark in the PDF outline (document bookmarks panel).
// Page is 1-based; Level 1 is top level (parts, specials), 2 nests chapters
// under their part.
type OutlineEntry struct {
	Title string
	Page  int
	Level int
}

// pdfUTF16Hex encodes s as a PDF hex string with a UTF-16BE BOM
// (<FEFF...>), the portable way to carry CJK outline titles — PDFDocEncoding
// literal strings would mangle them.
func pdfUTF16Hex(s string) pdf.HexString {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2+2*len(u))
	b[0], b[1] = 0xFE, 0xFF
	for i, r := range u {
		b[2+2*i], b[3+2*i] = byte(r>>8), byte(r)
	}
	return pdf.HexString(b)
}

// injectOutline adds a PDF /Outlines bookmark tree (one entry per toc item)
// to an already-rendered PDF via an incremental update, using gpdf's pdf
// package (Reader to locate the catalog and page objects, Modifier to append
// the outline objects and republish the catalog). gpdf's template layer has
// no outline API, so this is the publication-quality post-step that makes
// get_toc()-style consumers (PyMuPDF, viewers) see bookmarks.
func injectOutline(data []byte, toc []OutlineEntry) ([]byte, error) {
	if len(toc) == 0 {
		return data, nil
	}
	for i, e := range toc {
		if e.Title == "" {
			return nil, fmt.Errorf("outline entry %d has empty title", i)
		}
		if e.Page < 1 {
			return nil, fmt.Errorf("outline entry %q has invalid page %d", e.Title, e.Page)
		}
		if e.Level < 1 {
			return nil, fmt.Errorf("outline entry %q has invalid level %d", e.Title, e.Level)
		}
	}

	r, err := pdf.NewReader(data)
	if err != nil {
		return nil, fmt.Errorf("parse rendered pdf: %w", err)
	}
	pageCount, err := r.PageCount()
	if err != nil {
		return nil, fmt.Errorf("count pages: %w", err)
	}
	pageRefs := make([]pdf.ObjectRef, pageCount)
	for i := 0; i < pageCount; i++ {
		pi, err := r.Page(i)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", i+1, err)
		}
		pageRefs[i] = pi.Ref
	}

	m := pdf.NewModifier(r)

	refs := make([]pdf.ObjectRef, len(toc))
	for i := range toc {
		refs[i] = m.AllocObject()
	}
	outlineRoot := m.AllocObject()

	// Parent chain: stack of most recent item refs per level.
	parents := make([]pdf.ObjectRef, len(toc))
	var stack []pdf.ObjectRef
	for i, e := range toc {
		for len(stack) >= e.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			parents[i] = outlineRoot
		} else {
			parents[i] = stack[len(stack)-1]
		}
		stack = append(stack, refs[i])
	}

	// Children bookkeeping for /First /Last /Count. /Prev and /Next must
	// link siblings only (same parent), never across levels — viewers walk
	// sibling chains, so cross-level links would duplicate the tree.
	first := map[pdf.ObjectRef]pdf.ObjectRef{}
	last := map[pdf.ObjectRef]pdf.ObjectRef{}
	siblingPrev := map[int]pdf.ObjectRef{} // item index -> previous sibling
	siblingNext := map[int]pdf.ObjectRef{}
	topCount := 0
	var topFirst, topLast pdf.ObjectRef
	lastChild := map[pdf.ObjectRef]int{} // parent -> index of its last child
	prevTop := -1
	for i := range toc {
		p := parents[i]
		if p == outlineRoot {
			topCount++
			if prevTop >= 0 {
				siblingNext[prevTop] = refs[i]
				siblingPrev[i] = refs[prevTop]
			}
			prevTop = i
			if topFirst.Number == 0 {
				topFirst = refs[i]
			}
			topLast = refs[i]
		} else {
			if j, ok := lastChild[p]; ok {
				siblingNext[j] = refs[i]
				siblingPrev[i] = refs[j]
			}
			lastChild[p] = i
			if f, ok := first[p]; !ok || f.Number == 0 {
				first[p] = refs[i]
			}
			last[p] = refs[i]
		}
	}

	for i, e := range toc {
		if e.Page > pageCount {
			return nil, fmt.Errorf("outline entry %q page %d beyond page count %d", e.Title, e.Page, pageCount)
		}
		d := pdf.Dict{
			pdf.Name("Title"): pdfUTF16Hex(e.Title),
			pdf.Name("Parent"): parents[i],
			// /XYZ with null coordinates keeps the reader's zoom while
			// jumping to the page.
			pdf.Name("Dest"): pdf.Array{
				pageRefs[e.Page-1], pdf.Name("XYZ"), pdf.Null{}, pdf.Null{}, pdf.Null{},
			},
		}
		if prev, ok := siblingPrev[i]; ok {
			d[pdf.Name("Prev")] = prev
		}
		if next, ok := siblingNext[i]; ok {
			d[pdf.Name("Next")] = next
		}
		if f, ok := first[refs[i]]; ok && f.Number > 0 {
			d[pdf.Name("First")] = f
			d[pdf.Name("Last")] = last[refs[i]]
			// Omit /Count: children default to collapsed, which is the
			// conventional initial state for long book TOCs.
		}
		m.SetObject(refs[i], d)
	}

	m.SetObject(outlineRoot, pdf.Dict{
		pdf.Name("Type"):  pdf.Name("Outlines"),
		pdf.Name("First"): topFirst,
		pdf.Name("Last"):  topLast,
		pdf.Name("Count"): pdf.Integer(topCount),
	})

	// Republish the catalog with /Outlines wired in.
	rootDict, err := r.ResolveDict(r.RootRef())
	if err != nil {
		return nil, fmt.Errorf("resolve catalog: %w", err)
	}
	newRoot := make(pdf.Dict, len(rootDict)+1)
	for k, v := range rootDict {
		newRoot[k] = v
	}
	newRoot[pdf.Name("Outlines")] = outlineRoot
	m.SetObject(r.RootRef(), newRoot)

	out, err := m.Bytes()
	if err != nil {
		return nil, fmt.Errorf("write outline update: %w", err)
	}
	return out, nil
}
