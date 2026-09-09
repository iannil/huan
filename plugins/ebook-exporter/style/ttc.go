// Package style internal: TTC→standalone-TTF extraction with a content-keyed
// on-disk cache. Pure Go; table data is copied verbatim, only the table
// directory offsets and head.checkSumAdjustment are recomputed.
package style

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// ExtractTTC returns the path of a standalone TrueType font for font #index
// of the collection at sourcePath, extracting (and caching) under cacheDir.
// A non-collection source is passed through unchanged.
func ExtractTTC(sourcePath string, index int, cacheDir string) (string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("font source: %w", err)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read font source: %w", err)
	}
	if !IsCollection(data) {
		if index != 0 {
			return "", fmt.Errorf("%s: not a collection; only index 0 exists", sourcePath)
		}
		return sourcePath, nil
	}
	off, err := fontOffsetAt(data, index)
	if err != nil {
		return "", fmt.Errorf("%s: %w", sourcePath, err)
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("font cache dir: %w", err)
	}
	key := fmt.Sprintf("%s|%d|%d", sourcePath, info.ModTime().UnixNano(), index)
	sum := sha256.Sum256([]byte(key))
	cachePath := filepath.Join(cacheDir, fmt.Sprintf("%x.ttf", sum[:16]))

	if prev, err := os.ReadFile(cachePath); err == nil && HasTrueTypeOutlines(prev) {
		return cachePath, nil
	}

	font, err := rebuildStandalone(data, off)
	if err != nil {
		return "", fmt.Errorf("extract font %d of %s: %w", index, sourcePath, err)
	}
	if err := os.WriteFile(cachePath, font, 0o644); err != nil {
		return "", fmt.Errorf("write extracted font: %w", err)
	}
	return cachePath, nil
}

// rebuildStandalone copies font #at offset off (within collection data)
// into a standalone sfnt with rewritten directory offsets and a freshly
// computed head.checkSumAdjustment.
func rebuildStandalone(data []byte, off uint32) ([]byte, error) {
	if int(off)+12 > len(data) {
		return nil, fmt.Errorf("offset table out of bounds")
	}
	ver := binary.BigEndian.Uint32(data[off:])
	if ver != sfntTrueType && ver != sfntTrue {
		return nil, fmt.Errorf("sfntVersion %#x is not TrueType (CFF cannot serve PDF)", ver)
	}
	numTables := int(binary.BigEndian.Uint16(data[off+4:]))

	// 12-byte offset table is copied verbatim (numTables unchanged, so
	// searchRange/entrySelector/rangeShift stay valid).
	out := make([]byte, 12, 12+16*numTables+64)
	copy(out, data[off:off+12])

	// Pass 1: collect table data (4-byte padded), pass 2: directory.
	type entry struct {
		tag         string
		off, length uint32
	}
	var entries []entry
	body := make([]byte, 0, 64)
	for i := 0; i < numTables; i++ {
		rec := int(off) + 12 + 16*i
		if rec+16 > len(data) {
			return nil, fmt.Errorf("truncated table directory")
		}
		tag := string(data[rec : rec+4])
		tOff := binary.BigEndian.Uint32(data[rec+8:])
		tLen := binary.BigEndian.Uint32(data[rec+12:])
		if int(tOff)+int(tLen) > len(data) {
			return nil, fmt.Errorf("table %s out of bounds", tag)
		}
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		entries = append(entries, entry{tag, uint32(len(body)), tLen})
		body = append(body, data[tOff:int(tOff)+int(tLen)]...)
	}
	for _, e := range entries {
		var rec16 [16]byte
		copy(rec16[0:], e.tag)
		// checkSum is preserved from the source record; recomputed below
		// alongside offsets for simplicity of the rebuild.
		binary.BigEndian.PutUint32(rec16[8:], uint32(12+16*numTables)+e.off)
		binary.BigEndian.PutUint32(rec16[12:], e.length)
		out = append(out, rec16[:]...)
	}
	out = append(out, body...)

	// head.checkSumAdjustment fixup: zero it, sum the whole font, then write
	// 0xB1B0AFBA - sum at head+8.
	if err := fixHeadAdjustment(out); err != nil {
		return nil, err
	}
	return out, nil
}

func fixHeadAdjustment(font []byte) error {
	numTables := binary.BigEndian.Uint16(font[4:])
	for i := 0; i < int(numTables); i++ {
		rec := 12 + 16*i
		if string(font[rec:rec+4]) != "head" {
			continue
		}
		hOff := binary.BigEndian.Uint32(font[rec+8:])
		hLen := binary.BigEndian.Uint32(font[rec+12:])
		if int(hOff)+12 > len(font) || hLen < 12 {
			return fmt.Errorf("head table too short")
		}
		binary.BigEndian.PutUint32(font[hOff+8:], 0)
		var sum uint32
		for i := 0; i+4 <= len(font); i += 4 {
			sum += binary.BigEndian.Uint32(font[i:])
		}
		binary.BigEndian.PutUint32(font[hOff+8:], 0xB1B0AFBA-sum)
		return nil
	}
	return fmt.Errorf("no head table")
}
