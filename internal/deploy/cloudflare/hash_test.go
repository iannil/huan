package cloudflare

import "testing"

// Reference vectors computed with python blake3 library (v1.0.8) using the
// exact algorithm from wrangler source:
//
//	import base64, blake3
//	h = blake3.blake3((base64.b64encode(content).decode() + ext).encode()).hexdigest()[:32]
//
// These are the ground truth. If the Go implementation diverges from these
// vectors, dedup against Cloudflare's asset store breaks entirely.
var hashVectors = []struct {
	name    string
	content []byte
	ext     string
	want    string
}{
	{
		name:    "empty content no ext",
		content: []byte(""),
		ext:     "",
		want:    "af1349b9f5f9a1a6a0404dea36dcc949",
	},
	{
		name:    "empty content with html ext",
		content: []byte(""),
		ext:     "html",
		want:    "bc88d1b19523ba52d6a2959dd93cf9c8",
	},
	{
		name:    "text content txt ext",
		content: []byte("hello world"),
		ext:     "txt",
		want:    "e2d19b823f138bc36bc735f95942b3c6",
	},
	{
		name:    "html content html ext",
		content: []byte("<html></html>"),
		ext:     "html",
		want:    "4752155c2c0c0320b40bca1d83e8380a",
	},
	{
		name:    "css content css ext",
		content: []byte("body { color: red; }"),
		ext:     "css",
		want:    "661cb873fe48562ead61c4bb50e21ff7",
	},
}

func TestHash_ReferenceVectors(t *testing.T) {
	for _, tc := range hashVectors {
		t.Run(tc.name, func(t *testing.T) {
			got := Hash(tc.content, tc.ext)
			if got != tc.want {
				t.Errorf("Hash(%q, %q) = %q, want %q", string(tc.content), tc.ext, got, tc.want)
			}
		})
	}
}

func TestHash_LengthIs32(t *testing.T) {
	got := Hash([]byte("anything"), "html")
	if len(got) != 32 {
		t.Errorf("len(Hash) = %d, want 32", len(got))
	}
}

func TestHash_LowercaseHex(t *testing.T) {
	got := Hash([]byte("anything"), "html")
	for _, r := range got {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !isHex {
			t.Errorf("Hash contains non-lowercase-hex char %q in %q", r, got)
		}
	}
}

func TestHash_SameInputSameOutput(t *testing.T) {
	a := Hash([]byte("identical"), "txt")
	b := Hash([]byte("identical"), "txt")
	if a != b {
		t.Errorf("Hash is non-deterministic: %q vs %q", a, b)
	}
}

func TestHash_DifferentContentDifferentOutput(t *testing.T) {
	a := Hash([]byte("alpha"), "txt")
	b := Hash([]byte("beta"), "txt")
	if a == b {
		t.Errorf("Hash collisions on different content: %q", a)
	}
}

func TestHash_DifferentExtDifferentOutput(t *testing.T) {
	// Same content but different ext must produce different hashes (the ext
	// is part of the blake3 input).
	a := Hash([]byte("same"), "html")
	b := Hash([]byte("same"), "txt")
	if a == b {
		t.Errorf("Hash collisions on different ext: %q", a)
	}
}

func TestHash_BinaryContent(t *testing.T) {
	// Binary content (e.g. PNG header bytes) should be handled correctly by
	// base64 encoding before hashing.
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	got := Hash(png, "png")
	if len(got) != 32 {
		t.Errorf("len(Hash(png)) = %d, want 32", len(got))
	}
}
