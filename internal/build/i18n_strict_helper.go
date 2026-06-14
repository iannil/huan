package build

import (
	"crypto/sha256"
	"encoding/hex"
)

// sha256HexString returns the hex-encoded sha256 of data.
// Used by i18n_strict_test.go to compute expected source_hash values.
func sha256HexString(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
