package main

import (
	"time"

	"github.com/iannil/huan/internal/build"
	"github.com/spf13/cobra"
)

func commandTimings(cmd *cobra.Command) *build.Timings {
	enabled, _ := cmd.Flags().GetBool("timings")
	if enabled {
		return build.NewTimings()
	}
	return nil
}
func reportTimings(cmd *cobra.Command, c *build.Timings, stage string, start time.Time, err error) {
	c.Record("cli", stage, time.Since(start), err != nil)
	c.Report(func(format string, args ...any) { cmd.Printf(format, args...) })
}
