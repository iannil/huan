package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/iannil/huan/internal/equiv"
)

func main() {
	var dirA, dirB, mode, allowlistPath string
	flag.StringVar(&dirA, "a", "", "directory A (huan output)")
	flag.StringVar(&dirB, "b", "", "directory B (Hugo baseline)")
	flag.StringVar(&mode, "mode", "all", "byte|normalized|seo|ai|all")
	flag.StringVar(&allowlistPath, "allowlist", "", "file with allowed diff paths (one per line)")
	flag.Parse()
	if dirA == "" || dirB == "" {
		fmt.Fprintln(os.Stderr, "missing -a or -b")
		os.Exit(2)
	}

	allowlist := map[string]bool{}
	if allowlistPath != "" {
		f, err := os.Open(allowlistPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot open allowlist: %v\n", err)
			os.Exit(2)
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				allowlist[line] = true
			}
		}
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
		rep, err := equiv.CompareDirs(m, dirA, dirB, allowlist)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] error: %v\n", m, err)
			failed = true
			continue
		}
		fmt.Println(rep.FormatSummary())
		if !rep.Pass() {
			failed = true
			for _, f := range rep.Differing[:min(len(rep.Differing), 10)] {
				fmt.Printf("  diff: %s\n", f)
			}
		}
		for _, f := range rep.Whitelisted {
			fmt.Printf("  whitelisted: %s\n", f)
		}
	}
	if failed {
		os.Exit(1)
	}
}
