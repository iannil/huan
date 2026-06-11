// Package output handles writing rendered HTML and assets to the publish directory.
package output

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Writer writes files to the publish directory.
type Writer struct {
	publishDir string
	minifier   *Minifier
	written    int
	bytes      int64
}

// NewWriter creates a new Writer targeting publishDir without minification.
func NewWriter(publishDir string) *Writer {
	return &Writer{publishDir: publishDir}
}

// NewWriterWithMinify creates a Writer that minifies output before writing.
func NewWriterWithMinify(publishDir string) *Writer {
	return &Writer{publishDir: publishDir, minifier: NewMinifier()}
}

// URLToFilePath converts a URL path to an output file path under publishDir.
// Hugo convention: /foo/ → publishDir/foo/index.html
func URLToFilePath(url, publishDir string) string {
	clean := strings.TrimPrefix(url, "/")
	clean = strings.TrimSuffix(clean, "/")

	if clean == "" {
		return filepath.Join(publishDir, "index.html")
	}
	return filepath.Join(publishDir, clean, "index.html")
}

// PathToFilePath maps a path (without trailing slash) directly.
// e.g., ("sitemap.xml") → publishDir/sitemap.xml
// e.g., ("posts/index.xml") → publishDir/posts/index.xml
func PathToFilePath(path, publishDir string) string {
	clean := strings.TrimPrefix(path, "/")
	return filepath.Join(publishDir, clean)
}

// Write writes content to a file path under publishDir.
// Creates parent directories as needed. If a minifier is set, content is
// minified according to the file's media type before writing.
func (w *Writer) Write(relPath, content string) error {
	if w.minifier != nil {
		content = w.minifier.Minify(relPath, content)
	}

	fullPath := PathToFilePath(relPath, w.publishDir)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", fullPath, err)
	}

	w.written++
	w.bytes += int64(len(content))
	return nil
}

// WriteBytes writes raw bytes to a file path under publishDir.
// Minification is applied if a minifier is set.
func (w *Writer) WriteBytes(relPath string, data []byte) error {
	if w.minifier != nil {
		data = w.minifier.MinifyBytes(relPath, data)
	}

	fullPath := PathToFilePath(relPath, w.publishDir)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", fullPath, err)
	}
	w.written++
	w.bytes += int64(len(data))
	return nil
}

// CopyStatic copies all files from srcDir into publishDir, preserving relative paths.
func (w *Writer) CopyStatic(srcDir string) error {
	_, err := os.Stat(srcDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return filepath.Walk(srcDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		return w.copyFile(path, relPath)
	})
}

func (w *Writer) copyFile(src, relPath string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	dst := PathToFilePath(relPath, w.publishDir)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy to %s: %w", dst, err)
	}

	w.written++
	if info, err := os.Stat(src); err == nil {
		w.bytes += info.Size()
	}
	return nil
}

// CleanPublishDir removes the publish directory.
// Use cautiously - this is destructive.
func CleanPublishDir(publishDir string) error {
	return os.RemoveAll(publishDir)
}

// Stats returns the number of files written and total bytes.
func (w *Writer) Stats() (files int, bytes int64) {
	return w.written, w.bytes
}
