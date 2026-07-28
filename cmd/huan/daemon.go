package main

import (
	"fmt"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/daemon"
	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/theme"
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

		// Load config to get compiled plugins
		cfg, err := config.Load(sourceDir)
		if err != nil {
			return fmt.Errorf("daemon: load config: %w", err)
		}

		// Register compiled plugins into a shared registry
		var plugRegistry *plugin.Registry
		if !disablePlugin {
			plugRegistry, err = newPluginRegistry(cfg, sourceDir)
			if err != nil {
				return fmt.Errorf("daemon: register compiled plugins: %w", err)
			}
		}

		// Create ThemeManager from the plugin registry
		themeMgr := theme.NewManager(plugRegistry)
		if cfg.Theme != "" {
			if err := themeMgr.Activate(cfg.Theme); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "huan: theme activate %q: %v\n", cfg.Theme, err)
			}
		}

		return daemon.Run(daemon.Options{
			SourceDir:      sourceDir,
			ConfigPath:     configPath,
			Port:           port,
			Bind:           bind,
			TLSCert:        tlsCert,
			TLSKey:         tlsKey,
			Systemd:        systemd,
			BuildDrafts:    buildDrafts,
			PluginDir:      pluginDir,
			DisablePlugin:  disablePlugin,
			PluginRegistry: plugRegistry,
			ThemeManager:   themeMgr,
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