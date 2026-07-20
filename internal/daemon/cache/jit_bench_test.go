package cache

import (
	"testing"
	"time"
)

// BenchmarkJITCache_Set measures Set performance under load.
func BenchmarkJITCache_Set(b *testing.B) {
	c := NewJITCache(10000, 5*time.Minute)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		path := "/posts/page-" + itoa(i%1000) + "/"
		c.Set(path, &JITEntry{
			Path:       path,
			HTML:       []byte("<html><body>benchmark page content</body></html>"),
			RenderedAt: time.Now(),
			TTL:        5 * time.Minute,
		})
	}
}

// BenchmarkJITCache_Get measures Get performance with LRU hits.
func BenchmarkJITCache_Get(b *testing.B) {
	c := NewJITCache(10000, 5*time.Minute)
	// Pre-populate
	for i := 0; i < 1000; i++ {
		path := "/posts/page-" + itoa(i) + "/"
		c.Set(path, &JITEntry{Path: path, HTML: []byte("x"), RenderedAt: time.Now(), TTL: 5 * time.Minute})
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = c.Get("/posts/page-" + itoa(i%1000) + "/")
	}
}

// BenchmarkJITCache_GetMiss measures Get performance for cache misses.
func BenchmarkJITCache_GetMiss(b *testing.B) {
	c := NewJITCache(10000, 5*time.Minute)
	// Empty cache
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = c.Get("/nonexistent/")
	}
}

// itoa is a minimal int-to-string for benchmarks (no fmt import).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}