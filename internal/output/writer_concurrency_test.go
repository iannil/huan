package output

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWriterStatsLockDoesNotBlockIO(t *testing.T) {
	w := NewWriter(t.TempDir())
	w.mu.Lock()
	locked := true
	defer func() {
		if locked {
			w.mu.Unlock()
		}
	}()
	done := make(chan error, 1)
	go func() { done <- w.Write("other/index.html", "complete") }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if data, err := os.ReadFile(PathToFilePath("other/index.html", w.publishDir)); err == nil && string(data) == "complete" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("statistics mutex blocked file I/O")
		}
		time.Sleep(time.Millisecond)
	}
	w.mu.Unlock()
	locked = false
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if files, size := w.Stats(); files != 1 || size != 8 {
		t.Fatalf("Stats = %d, %d", files, size)
	}
}

func TestWriterConcurrentDistinctPaths(t *testing.T) {
	w := NewWriterWithMinify(t.TempDir())
	sequential := NewWriterWithMinify(t.TempDir())
	opts := CanonifyOptions{BaseURL: "https://example.org/"}
	w.SetCanonify(opts)
	sequential.SetCanonify(opts)
	type fixture struct {
		path     string
		content  string
		expected []byte
	}
	fixtures := make([]fixture, 64)
	var expectedBytes int64
	for i := range fixtures {
		path := fmt.Sprintf("page-%d/index.html", i)
		content := fmt.Sprintf(`<html><head><style>body { color: #ffffff; }</style></head><body>  <a href="/page/%d/">Page %d</a><script>var n = %d;</script></body></html>`, i, i, i)
		if err := sequential.Write(path, content); err != nil {
			t.Fatal(err)
		}
		expected, err := os.ReadFile(PathToFilePath(path, sequential.publishDir))
		if err != nil {
			t.Fatal(err)
		}
		fixtures[i] = fixture{path: path, content: content, expected: expected}
		expectedBytes += int64(len(expected))
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, item := range fixtures {
		wg.Add(1)
		go func(f fixture) {
			defer wg.Done()
			<-start
			if err := w.Write(f.path, f.content); err != nil {
				t.Error(err)
				return
			}
			actual, err := os.ReadFile(PathToFilePath(f.path, w.publishDir))
			if err != nil {
				t.Error(err)
				return
			}
			if !bytes.Equal(actual, f.expected) {
				t.Errorf("%s differs from sequential output", f.path)
			}
		}(item)
	}
	close(start)
	wg.Wait()
	if files, size := w.Stats(); files != 64 || size != expectedBytes {
		t.Fatalf("Stats = %d, %d; want 64, %d", files, size, expectedBytes)
	}
}

func TestWriterConcurrentSamePathCompletePayload(t *testing.T) {
	w := NewWriter(t.TempDir())
	payloads := make([][]byte, 32)
	for i := range payloads {
		payloads[i] = []byte(strings.Repeat(fmt.Sprintf("payload-%02d-", i), 4096+i))
	}
	srcDir := t.TempDir()
	for i, data := range payloads {
		if err := os.WriteFile(filepath.Join(srcDir, fmt.Sprint(i)), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	var total int64
	for round := 0; round < 4; round++ {
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i, data := range payloads {
			total += int64(len(data))
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				path := "shared.bin"
				if i%2 == 0 {
					path = "nested/../shared.bin"
				}
				var err error
				switch i % 4 {
				case 0:
					err = w.Write(path, string(data))
				case 1:
					err = w.WriteBytes(path, data)
				case 2:
					err = w.WriteBytesPath(path, data)
				case 3:
					err = w.copyFile(filepath.Join(srcDir, fmt.Sprint(i)), path)
				}
				if err != nil {
					t.Error(err)
				}
			}()
		}
		close(start)
		wg.Wait()
		actual, err := os.ReadFile(PathToFilePath("shared.bin", w.publishDir))
		if err != nil {
			t.Fatal(err)
		}
		complete := false
		for _, payload := range payloads {
			if bytes.Equal(actual, payload) {
				complete = true
				break
			}
		}
		if !complete {
			t.Fatal("same-path writes left a partial or mixed payload")
		}
	}
	if files, size := w.Stats(); files != 128 || size != total {
		t.Fatalf("Stats = %d, %d; want 128, %d", files, size, total)
	}
}

func TestWriterFailedWritesDoNotCount(t *testing.T) {
	w := NewWriter(t.TempDir())
	if err := w.Write("ok.txt", "ok"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(PathToFilePath("directory", w.publishDir), 0755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(src, []byte("copy"), 0644); err != nil {
		t.Fatal(err)
	}
	calls := []func() error{
		func() error { return w.Write("directory", "bad") },
		func() error { return w.WriteBytes("directory", []byte("bad")) },
		func() error { return w.WriteBytesPath("directory", []byte("bad")) },
		func() error { return w.copyFile(src, "directory") },
		func() error { return w.copyFile(src+"-missing", "missing") },
		func() error { return w.Write("ok.txt/child", "bad") },
	}
	for i, call := range calls {
		if err := call(); err == nil {
			t.Errorf("call %d unexpectedly succeeded", i)
		}
	}
	if files, size := w.Stats(); files != 1 || size != 2 {
		t.Fatalf("Stats = %d, %d; want 1, 2", files, size)
	}
}
