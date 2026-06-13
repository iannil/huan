package main

import (
	"fmt"

	"github.com/iannil/huan/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Display project configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(sourceDir)
			if err != nil {
				return err
			}
			out, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			fmt.Print(string(out))
			return nil
		},
	}
}
