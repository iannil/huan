package plugin

import (
	"fmt"
	"io"
)

// PrintWarnings writes load warnings in the canonical CLI format. The exact
// wording is load-bearing: operators grep for "huan: plugin load warning".
func PrintWarnings(w io.Writer, ws []LoadWarning) {
	for _, warning := range ws {
		fmt.Fprintf(w, "huan: plugin load warning: %s: %v\n", warning.Name, warning.Err)
	}
}

// PrintConfigWarnings writes config-level plugin warnings
// (Registry.ConfigWarnings) in the canonical CLI format. The exact wording is
// load-bearing: operators grep for "huan: plugin config warning".
func PrintConfigWarnings(w io.Writer, ws []string) {
	for _, s := range ws {
		fmt.Fprintf(w, "huan: plugin config warning: %s\n", s)
	}
}
