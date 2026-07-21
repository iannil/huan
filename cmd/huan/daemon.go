package main

import (
	"fmt"

	"github.com/iannil/huan/internal/daemon"
	"github.com/spf13/cobra"
)

	var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the production content server",
	Long: `Start the production content server with mixed rendering (pre-render + JIT),
REST API, admin panel, and infrastructure features (TLS, health checks, metrics).

A long-running process that serves the site as a backend service.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetString("port")
		bind, _ := cmd.Flags().GetString("bind")
		configPath, _ := cmd.Flags().GetString("config")
		tlsCert, _ := cmd.Flags().GetString("tls-cert")
		tlsKey, _ := cmd.Flags().GetString("tls-key")
		systemd, _ := cmd.Flags().GetBool("systemd")
		buildDrafts, _ := cmd.Flags().GetBool("buildDrafts")
		pluginDir, _ := cmd.Flags().GetString("plugin-dir")
		disablePlugin, _ := cmd.Flags().GetBool("disable-plugin")

		fmt.Println("huan daemon: starting (v0.6.0) ...")
		return daemon.Run(daemon.Options{
			SourceDir:     sourceDir,
			ConfigPath:    configPath,
			Port:          port,
			Bind:          bind,
			TLSCert:       tlsCert,
			TLSKey:        tlsKey,
			Systemd:       systemd,
			BuildDrafts:   buildDrafts,
			PluginDir:     pluginDir,
			DisablePlugin: disablePlugin,
		})
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
	daemonCmd.Flags().String("plugin-dir", "", "plugin directory (default: <sourceDir>/plugins)")
	daemonCmd.Flags().Bool("disable-plugin", false, "disable plugin loading")
}