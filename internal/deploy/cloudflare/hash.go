// Package cloudflare implements the Cloudflare deploy plugin (Cloudflare Pages
// direct-upload in PR1; R2 and Worker arrive in later PRs).
//
// See docs/adr/0002-cloudflare-deploy-plugin.md for protocol details and
// docs/adr/0003-unified-plugin-system.md for the plugin host abstraction.
package cloudflare

import (
	"encoding/base64"
	"strings"

	"github.com/zeebo/blake3"
)

// Hash computes a Cloudflare Pages asset hash.
//
// Per wrangler source (cloudflare/workers-sdk packages/deploy-helpers/src/
// deploy/helpers/hash.ts):
//
//	hash = blake3(base64(content) + ext).hex()[:32]
//
// where:
//   - content is the raw file bytes
//   - base64 is standard base64 encoding (with padding)
//   - ext is the file extension WITHOUT leading dot ("html" for "index.html",
//     "" for "Makefile" or any extensionless file)
//
// Output is the first 32 hex characters of the blake3 hex digest.
//
// Correctness is verified against python blake3 reference vectors in hash_test.go.
// A single byte error here invalidates the entire dedup mechanism.
func Hash(content []byte, ext string) string {
	base64Content := base64.StdEncoding.EncodeToString(content)
	combined := base64Content + ext
	digest := blake3.Sum256([]byte(combined))
	return hexDigest32(digest[:])
}

// hashFromBase64String is a variant for tests where the base64-encoded content
// is already known. Kept unexported to avoid API surface bloat in v1.
func hashFromBase64String(base64Content, ext string) string {
	combined := base64Content + ext
	digest := blake3.Sum256([]byte(combined))
	return hexDigest32(digest[:])
}

// hexDigest32 returns the first 32 hex characters (16 bytes) of the input.
// Cloudflare truncates the full 64-char blake3 hex to 32 chars for asset keys.
func hexDigest32(b []byte) string {
	const hexChars = "0123456789abcdef"
	// Only need first 16 bytes for 32 hex chars.
	if len(b) > 16 {
		b = b[:16]
	}
	var sb strings.Builder
	sb.Grow(32)
	for _, byt := range b {
		sb.WriteByte(hexChars[byt>>4])
		sb.WriteByte(hexChars[byt&0x0f])
	}
	return sb.String()
}
