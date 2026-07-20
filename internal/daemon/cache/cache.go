package cache

import "time"

// RenderCache provides the interface for the full-cache operations.
// Phase 1: pass-through to JITCache. Phase 2: add full cache + incr cache.
type RenderCache struct {
	JIT  *JITCache
	Root string
}

// NewRenderCache creates a RenderCache.
func NewRenderCache(root string, jitMaxSize int, jitTTL time.Duration) *RenderCache {
	return &RenderCache{
		JIT:  NewJITCache(jitMaxSize, jitTTL),
		Root: root,
	}
}