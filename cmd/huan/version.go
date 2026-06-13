package main

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/iannil/huan/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("huan %s\n", version.StringWithVCS())
		},
	}
}

func newEnvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Display version and environment info",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("huan %s\n", version.StringWithVCS())
			vcs := version.VCS()
			if vcs.Available {
				dirty := "clean"
				if vcs.Dirty {
					dirty = "dirty"
				}
				fmt.Printf("vcs %s %s %s\n", vcs.SHA, vcs.CommitTime, dirty)
			} else {
				fmt.Println("vcs (unavailable)")
			}
			fmt.Printf("go %s\n", runtime.Version())
			fmt.Printf("platform %s/%s\n", runtime.GOOS, runtime.GOARCH)
			if info, ok := debug.ReadBuildInfo(); ok {
				for _, mod := range info.Deps {
					fmt.Printf("dep %s %s\n", mod.Path, mod.Version)
				}
			}
		},
	}
}

