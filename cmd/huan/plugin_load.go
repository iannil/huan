package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPluginLoadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "load <path>",
		Short: "Load a .so plugin at runtime",
		Long: `Load a .so plugin file into the running daemon.

If the daemon is running, this sends the load request via the Admin API.
Otherwise (daemon not running), it loads the plugin into a temporary
registry for development/testing purposes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pluginPath := args[0]

			// Try Admin API first
			if err := callPluginAdminAPI("POST", "/admin/api/plugins/load",
				map[string]string{"path": pluginPath}); err == nil {
				return nil
			}

			// Fallback: local load (dev mode)
			fmt.Printf("plugin: loading %s (local mode)...\n", pluginPath)
			loader := newLocalPluginLoader()
			p, err := loader.LoadPlugin(pluginPath)
			if err != nil {
				return fmt.Errorf("load plugin: %w", err)
			}
			fmt.Printf("plugin: loaded %q (name: %s)\n", pluginPath, p.Name())
			return nil
		},
	}
}