// Package style internal: minimal sfnt/TTC structure probes used by the
// publication font resolution chain. Binary format is big-endian.
package style

import (
	"encoding/binary"
	"fmt"
)

const (
	sfntTrueType = 0x00010000
	sfntTrue     = 0x74727565 // 'true'
	sfntOTTO     = 0x4F54544F
	tagTTCF      = 0x74746366 // 'ttcf'
)

// IsCollection reports whether data is a TrueType Collection.
func IsCollection(data []byte) bool {
	return len(data) >= 4 && binary.BigEndian.Uint32(data[0:]) == tagTTCF
}

// fontOffsetAt returns the byte offset of font #index inside data
// (collections only).
func fontOffsetAt(data []byte, index int) (uint32, error) {
	if len(data) < 12 {
		return 0, fmt.Errorf("truncated ttc header")
	}
	numFonts := binary.BigEndian.Uint32(data[8:])
	if uint32(index) >= numFonts {
		return 0, fmt.Errorf("font index %d out of range (collection has %d)", index, numFonts)
	}
	return binary.BigEndian.Uint32(data[12+4*index:]), nil
}

// FontTrueTypeAt reports whether font #index of a collection has TrueType
// outlines. A non-collection input is treated as font 0 when index == 0.
func FontTrueTypeAt(data []byte, index int) (bool, error) {
	if !IsCollection(data) {
		if index != 0 {
			return false, fmt.Errorf("not a collection; only index 0 exists")
		}
		return hasTrueTypeOutlinesAt(data, 0)
	}
	off, err := fontOffsetAt(data, index)
	if err != nil {
		return false, err
	}
	return hasTrueTypeOutlinesAt(data, off)
}

// hasTrueTypeOutlinesAt parses the offset table at byte offset off.
func hasTrueTypeOutlinesAt(data []byte, off uint32) (bool, error) {
	if int(off)+12 > len(data) {
		return false, fmt.Errorf("offset table out of bounds")
	}
	ver := binary.BigEndian.Uint32(data[off:])
	if ver == sfntOTTO {
		return false, nil
	}
	if ver != sfntTrueType && ver != sfntTrue {
		return false, fmt.Errorf("unrecognized sfntVersion %#x", ver)
	}
	numTables := binary.BigEndian.Uint16(data[off+4:])
	for i := 0; i < int(numTables); i++ {
		rec := int(off) + 12 + 16*i
		if rec+16 > len(data) {
			return false, fmt.Errorf("truncated table directory")
		}
		if string(data[rec:rec+4]) == "glyf" {
			return true, nil
		}
	}
	return false, nil
}

// HasTrueTypeOutlines reports whether the font file data (ttf/otf/ttc) has
// TrueType outlines for its first font.
func HasTrueTypeOutlines(data []byte) bool {
	ok, err := FontTrueTypeAt(data, 0)
	return err == nil && ok
}
