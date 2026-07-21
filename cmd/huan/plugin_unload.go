package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPluginUnloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unload <name>",
		Short: "Unload a loaded plugin at runtime",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if err := callPluginAdminAPI("POST", "/admin/api/plugins/unload",
				map[string]string{"name": name}); err == nil {
				return nil
			}

			return fmt.Errorf("plugin %q: not loaded or daemon not running", name)
		},
	}
}