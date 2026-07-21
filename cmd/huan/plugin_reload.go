package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPluginReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload <name> <path>",
		Short: "Hot-reload a loaded plugin with a new .so file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			path := args[1]

			if err := callPluginAdminAPI("POST", "/admin/api/plugins/reload",
				map[string]string{"name": name, "path": path}); err == nil {
				return nil
			}

			return fmt.Errorf("plugin %q: reload failed or daemon not running", name)
		},
	}
}