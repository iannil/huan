package style

import (
	"encoding/binary"
	"fmt"
)

// maxRenderCmapSegments mirrors x/image font/sfnt's maxCmapSegments (see
// golang.org/x/image/font/sfnt/cmap.go): any cmap subtable — format-4 or
// format-12 — with more segments/groups than this is rejected by the render
// layer, so extracted fonts must not rely on such a subtable.
const maxRenderCmapSegments = 20000

// cmapSub is one cmap encoding record plus its subtable body.
type cmapSub struct {
	platformID, encodingID uint16
	data                   []byte
}

// normalizeCmap rewrites a cmap table so the render layer (x/image
// font/sfnt) can consume the extracted font. Extracted system fonts (e.g.
// STHeiti) ship cmap subtables that violate x/image's maxCmapSegments
// (20000) limit. Two reductions are applied, in order:
//
//  1. Every format-12 subtable has adjacent groups merged when they form a
//     contiguous character AND glyph run (end[i]+1 == start[i+1] &&
//     startGlyph[i+1] == startGlyph[i] + span + 1). This is lossless,
//     unconditional and idempotent; naively generated per-codepoint cmaps
//     collapse to a fraction of their groups.
//
//  2. A format-12 subtable that still exceeds maxRenderCmapSegments (real
//     sources like STHeiti fragment their groups with non-contiguous glyph
//     IDs, so step 1 cannot shrink them) gets a synthesized (0,4) format-4
//     subtable inserted immediately before it, covering its BMP codepoints
//     via idRangeOffset (which, unlike format-12 groups, can express
//     arbitrary per-character glyph IDs in compact character-run segments).
//     x/image picks the first subtable of the widest encoding, so it uses
//     the compact format-4, while spec-conformant consumers that prefer
//     format-12 keep the full (including supplementary-plane) coverage.
//
// Both steps are idempotent; subtables that need nothing are passed through
// byte-for-byte, and a cmap needing nothing is returned unchanged.
func normalizeCmap(cmap []byte) ([]byte, error) {
	if len(cmap) < 4 {
		return nil, fmt.Errorf("cmap table too short (%d bytes)", len(cmap))
	}
	numTables := int(binary.BigEndian.Uint16(cmap[2:]))
	if len(cmap) < 4+8*numTables {
		return nil, fmt.Errorf("truncated cmap header (%d tables, %d bytes)", numTables, len(cmap))
	}

	subs := make([]cmapSub, 0, numTables)
	hasUnicodeFormat4 := false
	for i := 0; i < numTables; i++ {
		rec := 4 + 8*i
		off := int(binary.BigEndian.Uint32(cmap[rec+4:]))
		if off >= len(cmap) {
			return nil, fmt.Errorf("cmap subtable %d: offset %d out of bounds", i, off)
		}
		n, err := cmapSubtableLength(cmap, off)
		if err != nil {
			return nil, fmt.Errorf("cmap subtable %d: %w", i, err)
		}
		data := cmap[off : off+n]
		pid := binary.BigEndian.Uint16(cmap[rec:])
		eid := binary.BigEndian.Uint16(cmap[rec+2:])
		if binary.BigEndian.Uint16(data[0:]) == 4 && (pid == 0 || pid == 3) {
			hasUnicodeFormat4 = true
		}
		subs = append(subs, cmapSub{platformID: pid, encodingID: eid, data: data})
	}

	// Pass 1: lossless strict merge of format-12 groups.
	bodies := make([][]byte, len(subs))
	mergedAny := false
	for i, s := range subs {
		if binary.BigEndian.Uint16(s.data[0:]) != 12 {
			bodies[i] = s.data
			continue
		}
		m, didMerge, err := mergeFormat12Groups(s.data)
		if err != nil {
			return nil, fmt.Errorf("cmap subtable %d ((%d,%d)): %w", i, s.platformID, s.encodingID, err)
		}
		mergedAny = mergedAny || didMerge
		bodies[i] = m
	}

	// Pass 2: synthesize BMP format-4 fallbacks for format-12 subtables that
	// are still oversized (unless a Unicode format-4 already exists to
	// shadow them). At most one is synthesized per cmap: a second (0,4)
	// record would be redundant for width-tied pickers and confusing for
	// spec-conformant ones.
	var final []cmapSub
	if !hasUnicodeFormat4 {
		synthesized := false
		for i := range subs {
			if !synthesized && isOversizeFormat12(bodies[i]) {
				if f4 := buildFormat4From12(bodies[i]); f4 != nil {
					final = append(final, cmapSub{platformID: 0, encodingID: 4, data: f4})
					synthesized = true
				}
			}
			final = append(final, cmapSub{platformID: subs[i].platformID, encodingID: subs[i].encodingID, data: bodies[i]})
		}
	}
	if final == nil {
		if !mergedAny {
			return cmap, nil // nothing to do
		}
		return rebuildCmap(cmap, subs, bodies), nil
	}
	return rebuildCmapRecords(cmap, final), nil
}

// isOversizeFormat12 reports whether data is a format-12 subtable whose
// group count exceeds the render layer's limit.
func isOversizeFormat12(data []byte) bool {
	return len(data) >= 16 &&
		binary.BigEndian.Uint16(data[0:]) == 12 &&
		binary.BigEndian.Uint32(data[12:]) > maxRenderCmapSegments
}

// rebuildCmap re-emits a cmap with the same encoding records as the source
// but new subtable bodies (and thus recomputed record offsets).
func rebuildCmap(cmap []byte, subs []cmapSub, bodies [][]byte) []byte {
	final := make([]cmapSub, len(subs))
	for i := range subs {
		final[i] = cmapSub{platformID: subs[i].platformID, encodingID: subs[i].encodingID, data: bodies[i]}
	}
	return rebuildCmapRecords(cmap, final)
}

// rebuildCmapRecords re-emits a cmap table: version and record count from
// the source header, each record's platform/encoding IDs from recs, and the
// subtable bodies packed 4-byte aligned after the records with recomputed
// offsets.
func rebuildCmapRecords(cmap []byte, recs []cmapSub) []byte {
	out := make([]byte, 4+8*len(recs), 4+8*len(recs)+len(cmap))
	copy(out, cmap[:2]) // version
	binary.BigEndian.PutUint16(out[2:], uint16(len(recs)))
	for i := range recs {
		binary.BigEndian.PutUint16(out[4+8*i:], recs[i].platformID)
		binary.BigEndian.PutUint16(out[4+8*i+2:], recs[i].encodingID)
	}
	body := out
	for i := range recs {
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		binary.BigEndian.PutUint32(out[4+8*i+4:], uint32(len(body)))
		body = append(body, recs[i].data...)
	}
	return body
}

// cmapSubtableLength returns the byte length of the cmap subtable starting
// at off, per its format's length field: u16 at +2 for formats 0/2/4/6, u32
// at +4 for formats 8/10/12/13 (reserved u16 precedes it), u32 at +2 for
// format 14 (no reserved field). Any anomaly — unknown format, sub-2-byte
// (cannot hold the format field) or out-of-bounds length — is an error so
// callers can leave the table untouched instead of slicing out of bounds.
func cmapSubtableLength(cmap []byte, off int) (int, error) {
	if off+2 > len(cmap) {
		return 0, fmt.Errorf("subtable header out of bounds")
	}
	switch format := binary.BigEndian.Uint16(cmap[off:]); format {
	case 8, 10, 12, 13:
		return cmapSubtableU32Length(cmap, off, 4)
	case 14:
		return cmapSubtableU32Length(cmap, off, 2)
	case 0, 2, 4, 6:
		if off+4 > len(cmap) {
			return 0, fmt.Errorf("subtable header out of bounds")
		}
		return cmapSubtableExtent(cmap, off, int(binary.BigEndian.Uint16(cmap[off+2:])))
	default:
		return 0, fmt.Errorf("unsupported subtable format %d", format)
	}
}

// cmapSubtableU32Length reads the u32 length field at off+at and validates
// the resulting extent.
func cmapSubtableU32Length(cmap []byte, off, at int) (int, error) {
	if off+at+4 > len(cmap) {
		return 0, fmt.Errorf("subtable header out of bounds")
	}
	return cmapSubtableExtent(cmap, off, int(binary.BigEndian.Uint32(cmap[off+at:])))
}

// cmapSubtableExtent validates a subtable's byte length against the cmap
// table bounds. A subtable must contain at least its 2-byte format field,
// so length 1 (which would panic the format read) is rejected.
func cmapSubtableExtent(cmap []byte, off, length int) (int, error) {
	if length < 2 || off+length > len(cmap) {
		return 0, fmt.Errorf("subtable length %d out of bounds at offset %d", length, off)
	}
	return length, nil
}

// cmapGroup is one format-12 character-to-glyph group.
type cmapGroup struct {
	startCharCode, endCharCode, startGlyphID uint32
}

// parseFormat12Groups decodes the group records of a format-12 subtable.
func parseFormat12Groups(sub []byte) ([]cmapGroup, error) {
	if len(sub) < 16 {
		return nil, fmt.Errorf("format-12 subtable too short (%d bytes)", len(sub))
	}
	n := int(binary.BigEndian.Uint32(sub[12:]))
	if len(sub) < 16+12*n {
		return nil, fmt.Errorf("truncated format-12 groups (%d groups, %d bytes)", n, len(sub))
	}
	groups := make([]cmapGroup, n)
	for i := range groups {
		g := sub[16+12*i:]
		groups[i] = cmapGroup{
			startCharCode: binary.BigEndian.Uint32(g),
			endCharCode:   binary.BigEndian.Uint32(g[4:]),
			startGlyphID:  binary.BigEndian.Uint32(g[8:]),
		}
	}
	return groups, nil
}

// mergeFormat12Groups rewrites a format-12 subtable, merging adjacent groups
// that form a contiguous character and glyph run. Groups must be sorted and
// non-overlapping (per the cmap spec). If nothing merges, the original slice
// is returned with didMerge=false.
func mergeFormat12Groups(sub []byte) (res []byte, didMerge bool, err error) {
	groups, err := parseFormat12Groups(sub)
	if err != nil {
		return nil, false, err
	}
	out := make([]cmapGroup, 0, len(groups))
	for _, g := range groups {
		if m := len(out); m > 0 &&
			out[m-1].endCharCode != ^uint32(0) && // end+1 must not wrap
			out[m-1].endCharCode+1 == g.startCharCode &&
			g.startGlyphID == out[m-1].startGlyphID+(out[m-1].endCharCode-out[m-1].startCharCode)+1 {
			out[m-1].endCharCode = g.endCharCode
			continue
		}
		out = append(out, g)
	}
	if len(out) == len(groups) {
		return sub, false, nil
	}

	res = make([]byte, 16+12*len(out))
	binary.BigEndian.PutUint16(res[0:], 12) // format
	binary.BigEndian.PutUint32(res[4:], uint32(len(res)))
	binary.BigEndian.PutUint32(res[8:], binary.BigEndian.Uint32(sub[8:])) // language
	binary.BigEndian.PutUint32(res[12:], uint32(len(out)))
	for i, g := range out {
		rec := res[16+12*i:]
		binary.BigEndian.PutUint32(rec[0:], g.startCharCode)
		binary.BigEndian.PutUint32(rec[4:], g.endCharCode)
		binary.BigEndian.PutUint32(rec[8:], g.startGlyphID)
	}
	return res, true, nil
}

// buildFormat4From12 synthesizes a cmap format-4 subtable covering the BMP
// codepoints of a format-12 subtable's groups. Segments are character-runs
// (adjacency in code space, regardless of glyph contiguity), carrying their
// per-character glyph IDs in an idRangeOffset glyph array — the only
// format-4 mechanism that expresses arbitrary glyph mappings. A nil return
// means the table does not fit format-4's u16 fields (too many BMP
// characters); the caller then keeps the format-12 subtable as-is.
func buildFormat4From12(sub []byte) []byte {
	groups, err := parseFormat12Groups(sub)
	if err != nil {
		return nil
	}
	type segment struct {
		start, end uint16
	}
	var segs []segment
	var glyphs []uint16 // glyphIdArray, concatenated per segment in order
	for _, g := range groups {
		if g.startCharCode > 0xFFFF {
			break // groups are sorted; the rest is supplementary-plane
		}
		end := uint16(min(g.endCharCode, 0xFFFF))
		if n := len(segs); n > 0 && uint32(segs[n-1].end)+1 == g.startCharCode {
			segs[n-1].end = end // extend the current character run
		} else {
			segs = append(segs, segment{start: uint16(g.startCharCode), end: end})
		}
		for c := int64(g.startCharCode); c <= int64(end); c++ {
			glyphs = append(glyphs, uint16(g.startGlyphID+uint32(c)-g.startCharCode))
		}
	}
	if len(segs) == 0 {
		return nil
	}
	// Standard terminating segment: 0xFFFF -> glyph 0 via idDelta 1.
	segs = append(segs, segment{start: 0xFFFF, end: 0xFFFF})
	segCount := len(segs)

	length := 14 + 8*segCount + 2 + 2*len(glyphs)
	if length > 0xFFFF || segCount > maxRenderCmapSegments {
		return nil // does not fit format-4; keep format-12 as-is
	}

	out := make([]byte, length)
	binary.BigEndian.PutUint16(out[0:], 4)
	binary.BigEndian.PutUint16(out[2:], uint16(length))
	binary.BigEndian.PutUint16(out[6:], uint16(2*segCount)) // segCountX2
	// searchRange/entrySelector/rangeShift per spec (ignored by x/image,
	// required by stricter parsers).
	searchRange := 1
	entrySelector := 0
	for searchRange*2 <= segCount {
		searchRange *= 2
		entrySelector++
	}
	searchRange *= 2
	binary.BigEndian.PutUint16(out[8:], uint16(searchRange))
	binary.BigEndian.PutUint16(out[10:], uint16(entrySelector))
	binary.BigEndian.PutUint16(out[12:], uint16(2*segCount-searchRange))

	ends := out[14:]
	starts := out[14+2*segCount+2:]
	deltas := out[14+4*segCount+2:]
	ranges := out[14+6*segCount+2:]
	glyphArray := out[14+8*segCount+2:]
	// charsBefore is the cumulative character count of the segments that
	// precede i in glyphIdArray; a segment's glyph entries sit at byte
	// offset 2*charsBefore into the array.
	charsBefore := 0
	for i, seg := range segs {
		binary.BigEndian.PutUint16(ends[2*i:], seg.end)
		binary.BigEndian.PutUint16(starts[2*i:], seg.start)
		if i == segCount-1 {
			binary.BigEndian.PutUint16(deltas[2*i:], 1) // 0xFFFF -> 0
		} else {
			// Arbitrary per-char glyphs via the glyph array. Per spec the
			// idRangeOffset value (in bytes) is measured from this very
			// slot, so it spans the remaining idRangeOffset entries down
			// to glyphIdArray (2*(segCount-i)) PLUS this segment's offset
			// into the concatenated glyph array (2*charsBefore).
			binary.BigEndian.PutUint16(ranges[2*i:], uint16(2*(segCount-i)+2*charsBefore))
		}
		charsBefore += int(seg.end) - int(seg.start) + 1
	}
	for i, g := range glyphs {
		binary.BigEndian.PutUint16(glyphArray[2*i:], g)
	}
	return out
}
