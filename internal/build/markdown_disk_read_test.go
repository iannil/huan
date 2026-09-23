package build

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestReadSizedMarkdownRecordDetectsChangedSize(t *testing.T) {
	for _, tc := range []struct {
		name  string
		size  int64
		data  string
		valid bool
	}{
		{"stable", 3, "abc", true},
		{"empty", 0, "", true},
		{"truncated", 4, "abc", false},
		{"grown", 2, "abc", false},
		{"invalid-size", -1, "", false},
		{"over-limit", maxMarkdownDiskEntry + 1, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := readSizedMarkdownRecord(bytes.NewBufferString(tc.data), tc.size)
			if ok != tc.valid || ok && string(got) != tc.data {
				t.Fatalf("read=%q %v", got, ok)
			}
		})
	}
}

func TestReadSizedMarkdownRecordOnlyProbesOneExtraByte(t *testing.T) {
	r := bytes.NewReader(make([]byte, 4096))
	if _, ok := readSizedMarkdownRecord(r, 128); ok {
		t.Fatal("growing record accepted")
	}
	if r.Len() != 4096-129 {
		t.Fatalf("read beyond bounded probe: %d bytes consumed", 4096-r.Len())
	}
}

func TestMarkdownDiskRejectsOversizedAndNonregularRecords(t *testing.T) {
	c := newTestMarkdownDisk(t, t.TempDir(), 1024)
	key := markdownKey{Raw: [32]byte{81}}
	path := c.disk.path(c.disk.id(key))
	if err := os.WriteFile(path, make([]byte, 1025), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.disk.load(key); ok {
		t.Fatal("oversized record accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.disk.load(key); ok {
		t.Fatal("directory accepted as record")
	}
}

func BenchmarkMarkdownDiskRead(b *testing.B) {
	for _, size := range []int{1024, 16 << 10, 128 << 10} {
		data := bytes.Repeat([]byte("x"), size)
		path := filepath.Join(b.TempDir(), "record")
		if err := os.WriteFile(path, data, 0600); err != nil {
			b.Fatal(err)
		}
		for _, sized := range []bool{false, true} {
			method := "readall"
			if sized {
				method = "sized"
			}
			b.Run(fmt.Sprintf("%d/%s", size, method), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					f, err := os.Open(path)
					if err != nil {
						b.Fatal(err)
					}
					var got []byte
					if sized {
						info, err := f.Stat()
						if err != nil || !info.Mode().IsRegular() || info.Size() > maxMarkdownDiskEntry {
							b.Fatal("invalid record metadata")
						}
						var ok bool
						got, ok = readSizedMarkdownRecord(f, info.Size())
						if !ok {
							b.Fatal("size changed")
						}
					} else {
						got, err = io.ReadAll(io.LimitReader(f, maxMarkdownDiskEntry+1))
						if err != nil {
							b.Fatal(err)
						}
					}
					if err := f.Close(); err != nil {
						b.Fatal(err)
					}
					if len(got) != size {
						b.Fatal("wrong read size")
					}
				}
			})
		}
	}
}
