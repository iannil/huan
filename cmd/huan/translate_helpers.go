package main

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// sha256Hex returns the hex-encoded SHA256 of data.
// Used by translate_cmd.go for source_hash frontmatter.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// utcNow returns the current UTC time in RFC3339 format.
// Used by translate_cmd.go for translated_at frontmatter.
func utcNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
