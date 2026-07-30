package main

import (
	"fmt"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/theme"
	"github.com/spf13/cobra"
)

func newThemeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "theme",
		Short: "Manage themes",
		Long:  "Activate, deactivate, and list theme plugins.",
	}
	cmd.AddCommand(newThemeActivateCmd())
	cmd.AddCommand(newThemeDeactivateCmd())
	cmd.AddCommand(newThemeListCmd())
	cmd.AddCommand(newThemeInfoCmd())
	return cmd
}

func newThemeActivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "activate <name>",
		Short: "Activate a theme plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(sourceDir)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			registry, err := newPluginRegistry(cfg, sourceDir, "")
			if err != nil {
				return fmt.Errorf("plugin registry: %w", err)
			}
			mgr := theme.NewManager(registry)
			if err := mgr.Activate(args[0]); err != nil {
				return err
			}
			fmt.Printf("Theme %q activated\n", args[0])
			return nil
		},
	}
}

func newThemeDeactivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deactivate",
		Short: "Deactivate the current theme",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(sourceDir)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			registry, err := newPluginRegistry(cfg, sourceDir, "")
			if err != nil {
				return fmt.Errorf("plugin registry: %w", err)
			}
			mgr := theme.NewManager(registry)
			mgr.Deactivate()
			fmt.Println("Theme deactivated")
			return nil
		},
	}
}

func newThemeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all available theme plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(sourceDir)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			registry, err := newPluginRegistry(cfg, sourceDir, "")
			if err != nil {
				return fmt.Errorf("plugin registry: %w", err)
			}
			mgr := theme.NewManager(registry)
			available := mgr.ListAvailable()
			if len(available) == 0 {
				fmt.Println("No theme plugins available.")
				return nil
			}
			fmt.Printf("%-20s %-10s %-15s %s\n", "NAME", "VERSION", "AUTHOR", "STATUS")
			for _, tp := range available {
				info := theme.DecodeInfo(tp.Info())
				status := "AVAILABLE"
				if mgr.ActiveName() == info.Name {
					status = "ACTIVE"
				}
				fmt.Printf("%-20s %-10s %-15s %s\n", info.Name, info.Version, info.Author, status)
			}
			return nil
		},
	}
}

func newThemeInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show detailed info for a theme plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(sourceDir)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			registry, err := newPluginRegistry(cfg, sourceDir, "")
			if err != nil {
				return fmt.Errorf("plugin registry: %w", err)
			}
			p, ok := registry.Get(args[0])
			if !ok {
				return fmt.Errorf("theme %q not found", args[0])
			}
			tp, ok := p.(theme.ThemePlugin)
			if !ok {
				return fmt.Errorf("plugin %q is not a theme", args[0])
			}
			info := theme.DecodeInfo(tp.Info())
			fmt.Printf("Name:        %s\n", info.Name)
			fmt.Printf("Version:     %s\n", info.Version)
			fmt.Printf("Author:      %s\n", info.Author)
			fmt.Printf("Description: %s\n", info.Description)
			fmt.Printf("Min Huan:    %s\n", info.MinHuanVer)
			fmt.Printf("Templates:   %d\n", len(tp.Templates()))
			fmt.Printf("Funcs:       %d\n", len(tp.FuncMap()))
			return nil
		},
	}
}
