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
