package style

import (
	"encoding/binary"
	"sort"
	"testing"
)

// tableHeadBytes returns a 54-byte head table with TrueType magic and
// version, per the OpenType spec prefix.
func tableHeadBytes() []byte {
	b := make([]byte, 54)
	binary.BigEndian.PutUint32(b[0:], 0x00010000) // version
	binary.BigEndian.PutUint32(b[12:], 0x5F0F3CF5)
	binary.BigEndian.PutUint32(b[28:], 0x00010000) // magic
	return b
}

// buildTestFont lays out a minimal single-font sfnt from the given tables.
// Table order follows sorted tags (sfnt requires ascending tag order).
func buildTestFont(t *testing.T, tables map[string][]byte) []byte {
	t.Helper()
	if _, ok := tables["head"]; !ok {
		tables["head"] = tableHeadBytes()
	}
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	n := len(tags)
	out := make([]byte, 0, 12+16*n+512)
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint32(hdr[0:], 0x00010000) // sfntVersion
	binary.BigEndian.PutUint16(hdr[4:], uint16(n))
	binary.BigEndian.PutUint16(hdr[6:], 16) // searchRange (numTables≤2 简化)
	binary.BigEndian.PutUint16(hdr[8:], 1)  // entrySelector
	binary.BigEndian.PutUint16(hdr[10:], uint16(n*16-16))
	out = append(out, hdr...)

	type rec struct {
		tag  string
		off  uint32
		len_ uint32
	}
	var recs []rec
	body := make([]byte, 0, 512)
	for _, tag := range tags {
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		data := tables[tag]
		recs = append(recs, rec{tag, uint32(12 + 16*n + len(body)), uint32(len(data))})
		body = append(body, data...)
	}
	for _, r := range recs {
		var rec16 [16]byte
		copy(rec16[0:], r.tag)
		binary.BigEndian.PutUint32(rec16[8:], r.off)
		binary.BigEndian.PutUint32(rec16[12:], r.len_)
		out = append(out, rec16[:]...)
	}
	out = append(out, body...)
	return out
}

// buildTestTTC packs two fonts into a 'ttcf' collection. Font 0 has no glyf
// (CFF-like), font 1 has glyf — index discrimination is under test.
func buildTestTTC(t *testing.T) []byte {
	t.Helper()
	f0 := buildTestFont(t, map[string][]byte{"maxp": {0, 0, 50, 0}, "cmap": []byte("cmap")})
	f1 := buildTestFont(t, map[string][]byte{"glyf": []byte("glyphdata"), "loca": {0, 0, 0, 4}})
	out := make([]byte, 12)
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], 2)
	out = binary.BigEndian.AppendUint32(out, 20) // header (12) + 2 offset entries
	out = binary.BigEndian.AppendUint32(out, uint32(20+len(f0)))
	return append(out, append(f0, f1...)...)
}

func TestHasTrueTypeOutlines(t *testing.T) {
	tt := buildTestFont(t, map[string][]byte{"glyf": []byte("g"), "loca": {0, 0, 0, 1}})
	if !HasTrueTypeOutlines(tt) {
		t.Fatal("ttf with glyf: want true")
	}
	cff := buildTestFont(t, map[string][]byte{"CFF ": []byte("cff")})
	// buildTestFont writes sfntVersion 0x00010000; patch to 'OTTO'
	binary.BigEndian.PutUint32(cff[0:], 0x4F54544F)
	if HasTrueTypeOutlines(cff) {
		t.Fatal("OTTO without glyf: want false")
	}
}

func TestFontTrueTypeAt(t *testing.T) {
	ttc := buildTestTTC(t)
	if !IsCollection(ttc) {
		t.Fatal("ttc magic: want IsCollection true")
	}
	if ok, err := FontTrueTypeAt(ttc, 0); err != nil || ok {
		t.Fatalf("font 0 (no glyf): want false, nil; got %v, %v", ok, err)
	}
	if ok, err := FontTrueTypeAt(ttc, 1); err != nil || !ok {
		t.Fatalf("font 1 (glyf): want true, nil; got %v, %v", ok, err)
	}
	if _, err := FontTrueTypeAt(ttc, 2); err == nil {
		t.Fatal("index out of range: want error")
	}
}

func TestFontTrueTypeAtTruncatedOffsetTable(t *testing.T) {
	// ttc header claims 2 fonts, but the buffer stops after the first
	// offset entry — reading font 1's offset must error, not panic.
	out := make([]byte, 12)
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], 2)
	out = binary.BigEndian.AppendUint32(out, 20) // only font 0's entry present
	if _, err := FontTrueTypeAt(out, 1); err == nil {
		t.Fatal("truncated offset table: want error, got nil")
	}
}
