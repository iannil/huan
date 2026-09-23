package output

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriterEnsureDirMatchesMkdirAllErrors(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("retained"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"", file, filepath.Join(file, "child")} {
		want := os.MkdirAll(dir, 0o755)
		got := NewWriter(root).ensureDir(dir)
		var wantPath, gotPath *os.PathError
		if !errors.As(want, &wantPath) || !errors.As(got, &gotPath) {
			t.Fatalf("dir=%q errors: got %v, want %v", dir, got, want)
		}
		if gotPath.Op != wantPath.Op || gotPath.Path != wantPath.Path || !errors.Is(gotPath.Err, wantPath.Err) {
			t.Errorf("dir=%q: got %#v, want %#v", dir, gotPath, wantPath)
		}
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "retained" {
		t.Fatalf("file changed: %q, %v", data, err)
	}
}

func TestWriterEnsureDirExistingAndSymlinkDirectories(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	w := NewWriter(root)
	for _, dir := range []string{root, target, link, filepath.Join(link, "nested", "leaf")} {
		if err := w.ensureDir(dir); err != nil {
			t.Fatalf("ensureDir(%q): %v", dir, err)
		}
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("existing permissions changed: %v, %v", info, err)
	}
	if info, err := os.Stat(filepath.Join(target, "nested", "leaf")); err != nil || !info.IsDir() {
		t.Fatalf("symlink target missing: %v, %v", info, err)
	}
}

func TestWriterEnsureDirConcurrentNestedPaths(t *testing.T) {
	w := NewWriter(t.TempDir())
	var workers sync.WaitGroup
	for i := 0; i < 64; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			// Several different filenames share the same new leaf directory.
			name := fmt.Sprintf("posts/2026/%d/%d.txt", i%8, i)
			if err := w.WriteBytesPath(name, []byte(name)); err != nil {
				t.Error(err)
				return
			}
			data, err := os.ReadFile(filepath.Join(w.publishDir, name))
			if err != nil || string(data) != name {
				t.Errorf("output %s = %q, %v", name, data, err)
			}
		}(i)
	}
	workers.Wait()
}

func BenchmarkWriterDirectories(b *testing.B) {
	for _, layout := range []string{"fresh-tree", "existing-tree"} {
		for _, impl := range []string{"reference", "candidate"} {
			b.Run(layout+"/"+impl, func(b *testing.B) {
				root := filepath.Join(b.TempDir(), "output")
				paths := make([]string, 512)
				for i := range paths {
					paths[i] = filepath.Join(root, "posts", "2026", fmt.Sprintf("%d", i))
				}
				b.ReportAllocs()
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					b.StopTimer()
					if err := os.RemoveAll(root); err != nil {
						b.Fatal(err)
					}
					if layout == "existing-tree" {
						for _, path := range paths {
							if err := os.MkdirAll(path, 0o755); err != nil {
								b.Fatal(err)
							}
						}
					}
					w := NewWriter(root)
					b.StartTimer()
					for _, path := range paths {
						if impl == "candidate" {
							if err := w.ensureDir(path); err != nil {
								b.Fatal(err)
							}
						} else {
							if _, ok := w.dirs.Load(path); ok {
								continue
							}
							if err := os.MkdirAll(path, 0o755); err != nil {
								b.Fatal(err)
							}
							w.dirs.Store(path, struct{}{})
						}
					}
				}
			})
		}
	}
}
