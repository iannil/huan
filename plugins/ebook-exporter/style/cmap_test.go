package style

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/opentype"
)

// buildFormat12Cmap returns a cmap table with a single (3,10) format-12
// subtable carrying the given groups (start, end, startGlyph).
func buildFormat12Cmap(t *testing.T, groups [][3]uint32) []byte {
	t.Helper()
	sub := make([]byte, 16+12*len(groups))
	binary.BigEndian.PutUint16(sub[0:], 12) // format
	binary.BigEndian.PutUint32(sub[4:], uint32(len(sub)))
	binary.BigEndian.PutUint32(sub[12:], uint32(len(groups)))
	for i, g := range groups {
		rec := sub[16+12*i:]
		binary.BigEndian.PutUint32(rec[0:], g[0])
		binary.BigEndian.PutUint32(rec[4:], g[1])
		binary.BigEndian.PutUint32(rec[8:], g[2])
	}
	cmap := make([]byte, 12+len(sub))
	binary.BigEndian.PutUint16(cmap[0:], 0) // version
	binary.BigEndian.PutUint16(cmap[2:], 1) // numTables
	// encoding record: platform 3 (Windows), encoding 10 (UCS-4), offset 12.
	binary.BigEndian.PutUint16(cmap[4:], 3)
	binary.BigEndian.PutUint16(cmap[6:], 10)
	binary.BigEndian.PutUint32(cmap[8:], 12)
	copy(cmap[12:], sub)
	return cmap
}

// fragmentedGroups returns n single-codepoint groups starting at 0x4E00
// (kept above ASCII so sfnt's OS/2 x/H fallback misses instead of loading
// glyphs beyond numGlyphs): [base+i, base+i] -> glyph i+1. They merge into
// the single run [base, base+n-1] -> glyph 1.
func fragmentedGroups(n int) [][3]uint32 {
	const base = uint32(0x4E00)
	groups := make([][3]uint32, n)
	for i := range groups {
		groups[i] = [3]uint32{base + uint32(i), base + uint32(i), uint32(i + 1)}
	}
	return groups
}

// format12Groups parses the (3,10) format-12 subtable groups out of a cmap
// table (big-endian; single subtable layout per buildFormat12Cmap).
func format12Groups(t *testing.T, cmap []byte) [][3]uint32 {
	t.Helper()
	if len(cmap) < 12 {
		t.Fatalf("cmap too short: %d", len(cmap))
	}
	numTables := int(binary.BigEndian.Uint16(cmap[2:]))
	off := -1
	for i := 0; i < numTables; i++ {
		rec := cmap[4+8*i:]
		pid, eid := binary.BigEndian.Uint16(rec), binary.BigEndian.Uint16(rec[2:])
		if pid == 3 && eid == 10 {
			off = int(binary.BigEndian.Uint32(rec[4:]))
		}
	}
	if off < 0 || off+16 > len(cmap) {
		t.Fatalf("no (3,10) format-12 subtable (off=%d, len=%d)", off, len(cmap))
	}
	if format := binary.BigEndian.Uint16(cmap[off:]); format != 12 {
		t.Fatalf("subtable format = %d, want 12", format)
	}
	n := int(binary.BigEndian.Uint32(cmap[off+12:]))
	if off+16+12*n > len(cmap) {
		t.Fatalf("truncated format-12 groups: n=%d len=%d", n, len(cmap))
	}
	groups := make([][3]uint32, n)
	for i := range groups {
		rec := cmap[off+16+12*i:]
		groups[i] = [3]uint32{
			binary.BigEndian.Uint32(rec),
			binary.BigEndian.Uint32(rec[4:]),
			binary.BigEndian.Uint32(rec[8:]),
		}
	}
	return groups
}

// minimalRenderableTables returns the table set x/image font/sfnt requires
// beyond cmap (head/maxp/hhea/hmtx/loca/glyf/post) for opentype.Parse to
// accept the font: 2 empty glyphs, 1 hmetric, short loca.
func minimalRenderableTables(cmap []byte) map[string][]byte {
	head := tableHeadBytes()
	binary.BigEndian.PutUint16(head[18:], 1000) // unitsPerEm
	maxp := make([]byte, 32)
	binary.BigEndian.PutUint16(maxp[4:], 2) // numGlyphs
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:], 1) // numHMetrics
	hmtx := make([]byte, 4)                  // 4*numHMetrics
	loca := make([]byte, 6)                  // (numGlyphs+1) short offsets, all 0
	glyf := make([]byte, 4)
	post := make([]byte, 32)
	binary.BigEndian.PutUint32(post[0:], 0x00030000)
	return map[string][]byte{
		"cmap": cmap, "glyf": glyf, "head": head, "hhea": hhea,
		"hmtx": hmtx, "loca": loca, "maxp": maxp, "post": post,
	}
}

// absolutizeTableOffsets rewrites each table offset in the sfnt directory at
// fontBase to be absolute (TTC spec: offsets are relative to the TTC file
// start; buildTestFont writes them font-relative).
func absolutizeTableOffsets(t *testing.T, font []byte, fontBase uint32) {
	t.Helper()
	numTables := int(binary.BigEndian.Uint16(font[4:]))
	for i := 0; i < numTables; i++ {
		rec := 12 + 16*i
		off := binary.BigEndian.Uint32(font[rec+8:])
		binary.BigEndian.PutUint32(font[rec+8:], off+fontBase)
	}
}

// buildFragmentedCmapTTC packs two fonts into a TTC where font 1 carries a
// format-12 cmap with 25000 single-codepoint groups (> x/image's
// maxCmapSegments of 20000) that all merge into one run.
func buildFragmentedCmapTTC(t *testing.T) []byte {
	t.Helper()
	f0 := buildTestFont(t, map[string][]byte{"maxp": {0, 0, 50, 0}, "cmap": []byte("cmap")})
	f1 := buildTestFont(t, minimalRenderableTables(buildFormat12Cmap(t, fragmentedGroups(25000))))
	absolutizeTableOffsets(t, f0, 20)
	absolutizeTableOffsets(t, f1, 20+uint32(len(f0)))
	out := make([]byte, 12)
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], 2)
	out = binary.BigEndian.AppendUint32(out, 20) // header (12) + 2 offset entries
	out = binary.BigEndian.AppendUint32(out, uint32(20+len(f0)))
	return append(out, append(f0, f1...)...)
}

func TestNormalizeCmapMergesRuns(t *testing.T) {
	cmap := buildFormat12Cmap(t, fragmentedGroups(25000))
	got, err := normalizeCmap(cmap)
	if err != nil {
		t.Fatal(err)
	}
	groups := format12Groups(t, got)
	if len(groups) != 1 {
		t.Fatalf("nGroups after merge = %d, want 1", len(groups))
	}
	want := [3]uint32{0x4E00, 0x4E00 + 24999, 1}
	if groups[0] != want {
		t.Fatalf("merged group = %v, want %v", groups[0], want)
	}
}

func TestNormalizeCmapPassthrough(t *testing.T) {
	// Already-merged runs are a no-op: bytes must pass through unchanged.
	runs := [][3]uint32{{0x20, 0x7E, 1}, {0x4E00, 0x9FFF, 100}}
	cmap := buildFormat12Cmap(t, runs)
	got, err := normalizeCmap(cmap)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(cmap) {
		t.Fatalf("merged cmap must pass through unchanged")
	}
	groups := format12Groups(t, got)
	if len(groups) != 2 || groups[0] != runs[0] || groups[1] != runs[1] {
		t.Fatalf("groups altered: %v", groups)
	}
}

// TestExtractTTCNormalizesCmap is the regression test for the STHeiti
// incident: extraction must rewrite a fragmented format-12 cmap down to
// merged runs so the render layer (x/image, maxCmapSegments=20000) accepts
// the standalone font.
func TestExtractTTCNormalizesCmap(t *testing.T) {
	src := filepath.Join(t.TempDir(), "font.ttc")
	if err := os.WriteFile(src, buildFragmentedCmapTTC(t), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ExtractTTC(src, 1, filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	groups := format12Groups(t, cmapTable(t, data))
	if len(groups) >= 20000 {
		t.Fatalf("extracted cmap still fragmented: %d groups", len(groups))
	}
	if _, err := opentype.Parse(data); err != nil {
		t.Fatalf("opentype.Parse on extracted font: %v", err)
	}
}

// cmapTable finds the cmap table bytes of a standalone sfnt font.
func cmapTable(t *testing.T, font []byte) []byte {
	t.Helper()
	numTables := int(binary.BigEndian.Uint16(font[4:]))
	for i := 0; i < numTables; i++ {
		rec := font[12+16*i:]
		if string(rec[:4]) == "cmap" {
			off := binary.BigEndian.Uint32(rec[8:])
			length := binary.BigEndian.Uint32(rec[12:])
			return font[off : off+length]
		}
	}
	t.Fatal("no cmap table")
	return nil
}

// TestNormalizeCmapSynthesizesFormat4 covers oversized format-12 subtables
// whose groups cannot shrink by merging (non-contiguous glyph IDs, the real
// STHeiti shape): a (0,4) format-4 BMP subtable must be synthesized ahead of
// the format-12 one, and x/image must resolve glyph IDs through it.
func TestNormalizeCmapSynthesizesFormat4(t *testing.T) {
	// 20001 single-codepoint groups (> maxRenderCmapSegments) with
	// non-contiguous glyph IDs: glyph = 2*i+7 jumps by 2 per codepoint, so
	// the strict merge rule never fires.
	groups := make([][3]uint32, 20001)
	for i := range groups {
		groups[i] = [3]uint32{uint32(0x4E00 + i), uint32(0x4E00 + i), uint32(2*i + 7)}
	}
	cmap := buildFormat12Cmap(t, groups)
	got, err := normalizeCmap(cmap)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) <= len(cmap)-1000 {
		t.Fatalf("expected rebuilt cmap, got %d bytes (source %d)", len(got), len(cmap))
	}
	// The synthesized record must precede the format-12 record so that
	// width-tied pickers take the format-4; the header's numTables must
	// count both records.
	numTables := int(binary.BigEndian.Uint16(got[2:]))
	if numTables != 2 {
		t.Fatalf("rebuilt numTables = %d, want 2 (format-4 + format-12)", numTables)
	}
	format4Seen := false
	for i := 0; i < numTables; i++ {
		rec := got[4+8*i:]
		pid, eid := binary.BigEndian.Uint16(rec), binary.BigEndian.Uint16(rec[2:])
		off := int(binary.BigEndian.Uint32(rec[4:]))
		format := binary.BigEndian.Uint16(got[off:])
		if pid == 0 && eid == 4 {
			if format != 4 || format4Seen {
				t.Fatalf("expected exactly one leading (0,4) format-4 record, got format=%d at %d", format, i)
			}
			format4Seen = true
		}
	}
	if !format4Seen {
		t.Fatal("no synthesized (0,4) format-4 subtable")
	}

	// x/image must parse the font and resolve BMP glyphs through the
	// format-4.
	font := buildTestFont(t, minimalRenderableTables(got))
	f, err := opentype.Parse(font)
	if err != nil {
		t.Fatalf("opentype.Parse with synthesized format-4: %v", err)
	}
	for _, r := range []rune{0x4E00, 0x4E4D, 0x4E00 + 20000} {
		gid, err := f.GlyphIndex(nil, r)
		if err != nil {
			t.Fatal(err)
		}
		want := uint32(r) - 0x4E00
		want = 2*want + 7
		if uint32(gid) != want {
			t.Fatalf("U+%04X: glyph index = %d, want %d", r, gid, want)
		}
	}
	// Supplementary-plane lookups resolve through the retained format-12 in
	// spec-conformant consumers; x/image itself returns 0 (notdef) there.
	if gid, err := f.GlyphIndex(nil, rune(0x20000)); err != nil || gid != 0 {
		t.Fatalf("U+20000 via format-4: glyph=%d err=%v, want 0,nil", gid, err)
	}
}

// TestNormalizeCmapFormat4ExactGlyphs is the regression test for the
// idRangeOffset arithmetic in synthesized format-4 subtables: each segment's
// glyph entries sit at a CUMULATIVE offset into the concatenated glyphIdArray
// (2*(segCount-i) + 2*charsBefore), not at the glyphIdArray start. The
// fixture is oversized (20001 groups > maxRenderCmapSegments) and spans two
// segments — a single-segment fixture (charsBefore=0) structurally cannot
// catch a wrong base. Exact glyph IDs are asserted across all segments.
func TestNormalizeCmapFormat4ExactGlyphs(t *testing.T) {
	groups := [][3]uint32{{0x41, 0x41, 10}} // segment A: one codepoint
	for i := 0; i < 20000; i++ {            // segment B: 20000 codepoints
		groups = append(groups, [3]uint32{uint32(0x4E00 + i), uint32(0x4E00 + i), uint32(2*i + 7)})
	}
	got, err := normalizeCmap(buildFormat12Cmap(t, groups))
	if err != nil {
		t.Fatal(err)
	}
	font := buildTestFont(t, minimalRenderableTables(got))
	f, err := opentype.Parse(font)
	if err != nil {
		t.Fatalf("opentype.Parse with synthesized format-4: %v", err)
	}
	// want = glyphID ground truth; group A maps 0x41->10, group B maps
	// 0x4E00+i -> 2*i+7, everything else unmapped.
	cases := []struct {
		r    rune
		want uint32
	}{
		{0x41, 10},                    // first char of segment A
		{0x42, 0},                     // unmapped gap
		{0x4DFF, 0},                   // unmapped gap
		{0x4E00, 7},                   // first char of segment B (charsBefore=1)
		{0x4E00 + 9999, 2*9999 + 7},   // middle of segment B
		{0x4E00 + 19999, 2*19999 + 7}, // last char of segment B
		{0xFFFF, 0},                   // terminator segment maps to glyph 0
	}
	for _, tc := range cases {
		gid, err := f.GlyphIndex(nil, tc.r)
		if err != nil {
			t.Fatalf("GlyphIndex(U+%04X): %v", tc.r, err)
		}
		if uint32(gid) != tc.want {
			t.Fatalf("U+%04X: glyph index = %d, want %d", tc.r, gid, tc.want)
		}
	}
}

// TestNormalizeCmapUnparseableSubtableNoPanic pins the degrade-don't-crash
// posture: a cmap carrying subtables whose length fields we cannot trust
// (format-14 Unicode-variation-sequences, as in PingFang/Hiragino, and a
// bogus-length format-12) must not panic normalizeCmap — it either passes
// the table through unchanged or returns an error that extraction degrades
// on, byte-for-byte safe either way.
func TestNormalizeCmapUnparseableSubtableNoPanic(t *testing.T) {
	build := func(t *testing.T, extra func(cmap []byte, subOff int) []byte) []byte {
		t.Helper()
		cmap := buildFormat12Cmap(t, [][3]uint32{{0x41, 0x41, 1}})
		return extra(cmap, 12) // format-12 subtable lives at offset 12
	}

	t.Run("format14", func(t *testing.T) {
		cmap := build(t, func(cmap []byte, subOff int) []byte {
			// Real format-14 layout: format u16, length u32 at +2,
			// numVarSelectorRecords u32 at +6.
			f14 := make([]byte, 16)
			binary.BigEndian.PutUint16(f14[0:], 14)
			binary.BigEndian.PutUint32(f14[2:], 16)
			// Append as a second record (0,5) and grow numTables.
			out := make([]byte, 0, len(cmap)+8+len(f14))
			out = append(out, cmap[:2]...)
			out = binary.BigEndian.AppendUint16(out, 2) // numTables
			out = append(out, cmap[4:12]...)            // record 0 (0,4)
			out = binary.BigEndian.AppendUint16(out, 0)
			out = binary.BigEndian.AppendUint16(out, 5)
			out = binary.BigEndian.AppendUint32(out, uint32(len(cmap)))
			out = append(out, cmap[12:]...)
			out = append(out, f14...)
			return out
		})
		got, err := normalizeCmap(cmap)
		if err != nil {
			return // degrade via error: acceptable
		}
		if string(got) != string(cmap) {
			t.Fatalf("unparseable-subtable cmap must pass through unchanged, got %d bytes (source %d)", len(got), len(cmap))
		}
	})

	t.Run("bogusLength", func(t *testing.T) {
		cmap := build(t, func(cmap []byte, subOff int) []byte {
			binary.BigEndian.PutUint32(cmap[subOff+4:], 0xFFFFFFF0) // length far beyond the table
			return cmap
		})
		if got, err := normalizeCmap(cmap); err == nil && string(got) != string(cmap) {
			t.Fatalf("bogus-length cmap must error or pass through unchanged, got %d bytes", len(got))
		}
	})
}

// TestNormalizeCmapIdempotent pins the contract that re-normalizing an
// already-normalized cmap changes nothing (the extraction cache may miss and
// rebuild repeatedly).
func TestNormalizeCmapIdempotent(t *testing.T) {
	// Fragmented oversized source: exercises both merge and synthesis.
	groups := make([][3]uint32, 20001)
	for i := range groups {
		groups[i] = [3]uint32{uint32(0x4E00 + i), uint32(0x4E00 + i), uint32(2*i + 7)}
	}
	once, err := normalizeCmap(buildFormat12Cmap(t, groups))
	if err != nil {
		t.Fatal(err)
	}
	twice, err := normalizeCmap(once)
	if err != nil {
		t.Fatal(err)
	}
	if string(twice) != string(once) {
		t.Fatalf("normalizeCmap not idempotent: %d vs %d bytes", len(once), len(twice))
	}
}

// stheitiSourceGroups extracts the (0,4) format-12 groups of the given font
// index straight from the TTC's cmap, as the glyph-ID ground truth for
// cross-checking extraction output.
func stheitiSourceGroups(t *testing.T, data []byte, fontIndex uint32) []cmapGroup {
	t.Helper()
	off := binary.BigEndian.Uint32(data[12+4*fontIndex:])
	numTables := int(binary.BigEndian.Uint16(data[off+4:]))
	var cmap []byte
	for j := 0; j < numTables; j++ {
		rec := off + 12 + 16*uint32(j)
		if string(data[rec:rec+4]) == "cmap" {
			cOff := binary.BigEndian.Uint32(data[rec+8:])
			cLen := binary.BigEndian.Uint32(data[rec+12:])
			cmap = data[cOff : cOff+cLen]
		}
	}
	if cmap == nil {
		t.Fatal("source has no cmap table")
	}
	so := int(binary.BigEndian.Uint32(cmap[4+8*0+4:]))
	groups, err := parseFormat12Groups(cmap[so:])
	if err != nil {
		t.Fatal(err)
	}
	return groups
}

// glyphInGroups binary-searches sorted format-12 groups for the glyph mapped
// to c (0 when unmapped).
func glyphInGroups(groups []cmapGroup, c uint32) uint32 {
	i, j := 0, len(groups)
	for i < j {
		h := (i + j) / 2
		g := groups[h]
		if c < g.startCharCode {
			j = h
		} else if g.endCharCode < c {
			i = h + 1
		} else {
			return g.startGlyphID + c - g.startCharCode
		}
	}
	return 0
}

// TestExtractTTCRealSTHeiti verifies the real-machine acceptance: extracting
// font 1 of STHeiti Light.ttc yields a font the render layer can parse, and
// glyph lookups through the normalized cmap match the source TTC exactly.
func TestExtractTTCRealSTHeiti(t *testing.T) {
	const src = "/System/Library/Fonts/STHeiti Light.ttc"
	srcData, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("no STHeiti Light.ttc on this machine: %v", err)
	}
	groups := stheitiSourceGroups(t, srcData, 1)
	got, err := ExtractTTC(src, 1, filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	f, err := opentype.Parse(data)
	if err != nil {
		t.Fatalf("opentype.Parse on extracted STHeiti: %v", err)
	}
	if n := f.NumGlyphs(); n == 0 {
		t.Fatal("extracted STHeiti has no glyphs")
	}
	// Exact-glyph probe: x/image resolves BMP codepoints through the
	// synthesized format-4, so each must match the source TTC's format-12
	// mapping. Sample covers run starts, middles, and gaps.
	for _, r := range []rune{'A', 'a', 'z', '0', '中', '文', '说', 0x3042, 0x9FA5, 0xFF21, 0x4E00, 0x12345 - 0x10000 + 0x4E00} {
		want := glyphInGroups(groups, uint32(r))
		if want == 0 {
			t.Logf("U+%04X unmapped in source; skipping", r)
			continue
		}
		gid, err := f.GlyphIndex(nil, r)
		if err != nil {
			t.Fatalf("GlyphIndex(U+%04X): %v", r, err)
		}
		if uint32(gid) != want {
			t.Fatalf("U+%04X: extracted glyph = %d, want %d (source cmap)", r, gid, want)
		}
	}
	// Supplementary-plane codepoints resolve to notdef through the BMP-only
	// format-4 (full coverage remains available to format-12 consumers).
	if gid, err := f.GlyphIndex(nil, rune(0x20000)); err != nil || gid != 0 {
		t.Fatalf("U+20000: glyph = %d, %v; want 0, nil", gid, err)
	}
	t.Logf("extracted STHeiti: %d glyphs, %d bytes", f.NumGlyphs(), len(data))
}
