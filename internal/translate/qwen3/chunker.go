package qwen3

import (
	"strings"
)

// Chunk represents a translatable section of a markdown body.
//
// A body is split into Chunks by level-2 headings (lines starting with
// "## "). Content before the first "## " becomes the preamble Chunk
// (Heading="", IsPreamble=true). If the body has no "## " at all, the
// entire body is a single preamble Chunk.
//
// Heading levels other than ## (e.g., ###, #) stay inside the parent
// Chunk's Body. This keeps each Chunk a coherent narrative unit (a
// section + its sub-sections).
type Chunk struct {
	// Index is the 1-based position in the document. Used for logging.
	Index int

	// Heading is the level-2 heading line that opens this Chunk,
	// including the "## " prefix. Empty for the preamble Chunk.
	Heading string

	// Body is the markdown content of this Chunk, EXCLUDING the Heading
	// line. For the preamble, this is everything before the first "## ".
	// For section chunks, this is everything from the line after Heading
	// up to (but not including) the next "## " line.
	//
	// Body is what gets sent to the LLM as TRANSLATE_NOW. Heading is
	// sent separately to ensure 1:1 translation of the heading line.
	Body string

	// IsPreamble is true for the optional pre-first-section Chunk.
	// Preamble chunks have empty Heading and IsPreamble=true.
	IsPreamble bool
}

// chunker splits a markdown body into Chunks by level-2 headings.
// Fenced code blocks (```...```) are respected — a "## " inside a code
// fence does NOT start a new Chunk.
//
// Returned Chunks are in document order. The body is always split (worst
// case: 1 preamble Chunk containing the entire body).
func splitBySection(body string) []Chunk {
	lines := strings.Split(body, "\n")

	var chunks []Chunk
	var preambleLines []string
	var currentHeading string
	var currentBodyLines []string
	inCodeFence := false

	flushCurrent := func() {
		if currentHeading == "" && len(currentBodyLines) == 0 {
			return
		}
		chunks = append(chunks, Chunk{
			Index:      len(chunks) + 1,
			Heading:    currentHeading,
			Body:       strings.TrimRight(strings.Join(currentBodyLines, "\n"), "\n"),
			IsPreamble: false,
		})
		currentHeading = ""
		currentBodyLines = nil
	}

	flushPreamble := func() {
		if len(preambleLines) == 0 {
			return
		}
		body := strings.TrimRight(strings.Join(preambleLines, "\n"), "\n")
		if strings.TrimSpace(body) == "" {
			preambleLines = nil
			return
		}
		chunks = append(chunks, Chunk{
			Index:      len(chunks) + 1,
			Heading:    "",
			Body:       body,
			IsPreamble: true,
		})
		preambleLines = nil
	}

	for _, line := range lines {
		// Track fenced code blocks. A line starting with ``` (possibly
		// with a language tag) toggles the fence state.
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "```") {
			inCodeFence = !inCodeFence
		}

		// A "## " at line start (NOT inside a code fence) opens a new
		// section Chunk.
		if !inCodeFence && isLevel2Heading(line) {
			// Close any open section Chunk first.
			if currentHeading != "" {
				flushCurrent()
			}
			// Then close any preamble.
			if currentHeading == "" {
				flushPreamble()
			}
			currentHeading = line
			currentBodyLines = nil
			continue
		}

		// Accumulate line into current context.
		if currentHeading != "" {
			currentBodyLines = append(currentBodyLines, line)
		} else {
			preambleLines = append(preambleLines, line)
		}
	}

	// Flush trailing buffers.
	if currentHeading != "" {
		flushCurrent()
	} else {
		// Body had no "## " at all — preamble is the whole body.
		flushPreamble()
	}

	if len(chunks) == 0 {
		// Defensive: empty body or whitespace-only input.
		return []Chunk{{Index: 1, Heading: "", Body: "", IsPreamble: true}}
	}

	// Re-number indexes after preamble flush ordering (preamble flushed
	// before any section, so indexes are already correct, but re-assign
	// to be safe).
	for i := range chunks {
		chunks[i].Index = i + 1
	}

	return chunks
}

// isLevel2Heading returns true if line is a markdown level-2 heading:
// starts with "## " (literal "##" followed by exactly one space, not "#"
// or "###"). Leading whitespace is not allowed (markdown spec).
func isLevel2Heading(line string) bool {
	// Must start with exactly "## " (two hashes + space).
	// Reject "##" without trailing content (empty heading is malformed
	// but we'll be lenient and accept it).
	// Reject "### " (level 3+).
	return strings.HasPrefix(line, "## ") || line == "##"
}

// Source reassembles the original source text of a Chunk by joining
// Heading + Body with a newline. Used for per-chunk quality checks
// (paragraph count, etc.) on the original source slice.
func (c Chunk) Source() string {
	if c.IsPreamble || c.Heading == "" {
		return c.Body
	}
	return c.Heading + "\n" + c.Body
}
