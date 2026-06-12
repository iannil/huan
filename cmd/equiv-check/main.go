// Command equiv-check runs 3-dimension equivalence comparison between two
// parallel directory trees (typically Hugo baseline vs huan output).
//
// Usage:
//   equiv-check -a <dirA> -b <dirB> [--mode byte|normalized|seo|ai|all]
//
// Exit codes:
//   0: all selected modes passed (or byte-only, which always passes)
//   1: one or more non-byte modes failed
//   2: usage error
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/iannil/huan/internal/equiv"
)

func main() {
	var dirA, dirB, mode string
	flag.StringVar(&dirA, "a", "", "directory A (huan output)")
	flag.StringVar(&dirB, "b", "", "directory B (Hugo baseline)")
	flag.StringVar(&mode, "mode", "all", "byte|normalized|seo|ai|all")
	flag.Parse()
	if dirA == "" || dirB == "" {
		fmt.Fprintln(os.Stderr, "missing -a or -b")
		os.Exit(2)
	}

	var modes []equiv.Mode
	switch mode {
	case "all":
		modes = []equiv.Mode{equiv.ModeNormalized, equiv.ModeSEO, equiv.ModeAI, equiv.ModeByte}
	case "byte":
		modes = []equiv.Mode{equiv.ModeByte}
	case "normalized":
		modes = []equiv.Mode{equiv.ModeNormalized}
	case "seo":
		modes = []equiv.Mode{equiv.ModeSEO}
	case "ai":
		modes = []equiv.Mode{equiv.ModeAI}
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", mode)
		os.Exit(2)
	}

	failed := false
	for _, m := range modes {
		rep, err := equiv.CompareDirs(m, dirA, dirB)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] error: %v\n", m, err)
			failed = true
			continue
		}
		fmt.Println(rep.FormatSummary())
		if !rep.Pass() {
			failed = true
			limit := len(rep.Differing)
			if limit > 10 {
				limit = 10
			}
			for _, f := range rep.Differing[:limit] {
				fmt.Printf("  diff: %s\n", f)
			}
		}
	}
	if failed {
		os.Exit(1)
	}
}
