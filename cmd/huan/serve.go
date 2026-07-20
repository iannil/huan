// serve.go — DEPRECATED, use 'huan dev' instead
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	// serveCmd is registered in main.go as deprecated
	// This file kept for backward compatibility.
	// Remove in next major version.
}

// runServe is kept as an alias for test compatibility.
func runServe(cmd *cobra.Command, args []string) error {
	fmt.Println("WARNING: 'huan serve' is deprecated, use 'huan dev' instead")
	return runDev(cmd, args)
}
