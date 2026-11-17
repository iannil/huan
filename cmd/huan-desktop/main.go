// Command huan-desktop is the huan desktop app (MyGo shell, Phase 1).
package main

import (
	"fmt"
	"os"

	"github.com/iannil/huan/internal/desktop"
)

func main() {
	if err := desktop.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "huan-desktop:", err)
		os.Exit(1)
	}
}
