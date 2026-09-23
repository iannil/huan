package output

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStringFilePreservesBytesTruncationAndPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	if err := os.WriteFile(path, []byte("previous long contents"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"中文\x00\xff\xfe\n", "x", ""} {
		if err := writeStringFile(path, value, 0644); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != value {
			t.Fatalf("bytes=%q err=%v want=%q", got, err, value)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("overwriting changed permissions: %v", info.Mode())
		}
	}
	created, control := filepath.Join(dir, "created.html"), filepath.Join(dir, "control.html")
	if err := writeStringFile(created, "created", 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(control, []byte("created"), 0644); err != nil {
		t.Fatal(err)
	}
	a, err := os.Stat(created)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(control)
	if err != nil {
		t.Fatal(err)
	}
	if a.Mode() != b.Mode() {
		t.Fatalf("create permissions differ: %v vs %v", a.Mode(), b.Mode())
	}
}

func TestWriteStringFileErrorsAndWriterStatistics(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{dir, filepath.Join(dir, "missing", "file")} {
		err := writeStringFile(path, "content", 0644)
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) || pathErr.Path != path {
			t.Fatalf("missing original path error: %v", err)
		}
	}
	w := NewWriter(dir)
	if err := os.Mkdir(filepath.Join(dir, "blocked.html"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := w.Write("blocked.html", "failed"); err == nil || !strings.Contains(err.Error(), "write ") {
		t.Fatalf("missing wrapped error: %v", err)
	}
	if files, bytes := w.Stats(); files != 0 || bytes != 0 {
		t.Fatalf("failed write counted: %d %d", files, bytes)
	}
	if err := w.Write("success.html", "成功"); err != nil {
		t.Fatal(err)
	}
	if files, bytes := w.Stats(); files != 1 || bytes != int64(len("成功")) {
		t.Fatalf("success stats: %d %d", files, bytes)
	}
}

func TestWriteStringFilePropagatesDeviceWriteError(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("/dev/full unavailable")
	}
	if err := writeStringFile("/dev/full", "must fail", 0644); err == nil {
		t.Fatal("device write error ignored")
	}
}

func BenchmarkWriteStringFile(b *testing.B) {
	for _, size := range []int{1024, 64 << 10} {
		body := strings.Repeat("x", size)
		for _, method := range []string{"bytes", "string"} {
			b.Run(method+"/"+map[int]string{1024: "1KiB", 64 << 10: "64KiB"}[size], func(b *testing.B) {
				path := filepath.Join(b.TempDir(), "page.html")
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var err error
					if method == "bytes" {
						err = os.WriteFile(path, []byte(body), 0644)
					} else {
						err = writeStringFile(path, body, 0644)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
