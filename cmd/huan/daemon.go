package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the production content server",
	Long: `Start the production content server with mixed rendering (pre-render + JIT),
REST API, admin panel, and infrastructure features (TLS, health checks, metrics).

A long-running process that serves the site as a backend service.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("huan daemon: starting (v0.6.0) ...")
		// TODO: Phase 2 — wire up daemon.Run()
		_ = time.Now()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.Flags().String("port", "8080", "HTTP listen port")
	daemonCmd.Flags().String("bind", "0.0.0.0", "interface to bind")
	daemonCmd.Flags().String("config", "", "daemon config file path (daemon.yaml)")
	daemonCmd.Flags().String("tls-cert", "", "TLS certificate path")
	daemonCmd.Flags().String("tls-key", "", "TLS private key path")
	daemonCmd.Flags().Bool("systemd", false, "enable systemd notify integration")
	daemonCmd.Flags().BoolP("buildDrafts", "D", false, "include draft content")
}
