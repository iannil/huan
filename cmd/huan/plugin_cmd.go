package main

import (
	"fmt"
	"strings"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/plugin"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage plugins",
		Long:  "Inspect plugins compiled into this huan binary and their effective configuration.",
	}
	cmd.AddCommand(newPluginListCmd())
	cmd.AddCommand(newPluginInfoCmd())
	return cmd
}

func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(sourceDir)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			registry, err := newPluginRegistry(cfg)
			if err != nil {
				return fmt.Errorf("plugin registry: %w", err)
			}
			printPluginList(registry)
			return nil
		},
	}
}

func newPluginInfoCmd() *cobra.Command {
	var showSecrets bool
	cmd := &cobra.Command{
		Use:   "info <name>",
		Short: "Show detailed info for a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(sourceDir)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			registry, err := newPluginRegistry(cfg)
			if err != nil {
				return fmt.Errorf("plugin registry: %w", err)
			}
			p, ok := registry.Get(args[0])
			if !ok {
				return fmt.Errorf("plugin %q not configured", args[0])
			}
			printPluginInfo(p, cfg.Plugins[args[0]], showSecrets)
			return nil
		},
	}
	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "show sensitive field values (default: mask as ***)")
	return cmd
}

func printPluginList(registry *plugin.Registry) {
	names := registry.SortedNames()
	if len(names) == 0 {
		fmt.Println("No plugins configured. Add a plugin under plugins: in huan.yaml.")
		return
	}
	fmt.Printf("%-20s %-25s %s\n", "NAME", "CAPABILITIES", "STATUS")
	for _, name := range names {
		p, _ := registry.Get(name)
		caps := capabilityLabels(p)
		fmt.Printf("%-20s %-25s %s\n", name, joinLabels(caps), "configured")
	}
}

// printPluginInfo prints metadata, capabilities, and effective config for one
// plugin. Sensitive fields are masked unless showSecrets is true.
func printPluginInfo(p plugin.Plugin, rawConfig map[string]any, showSecrets bool) {
	fmt.Printf("name: %s\n", p.Name())
	caps := capabilityLabels(p)
	if len(caps) > 0 {
		fmt.Printf("capabilities: %s\n", strings.Join(caps, ", "))
	} else {
		fmt.Println("capabilities: -")
	}
	fmt.Println("config:")
	configToPrint := rawConfig
	if !showSecrets {
		configToPrint = maskSensitive(rawConfig)
	}
	out, err := yaml.Marshal(configToPrint)
	if err != nil {
		fmt.Printf("  <error rendering config: %v>\n", err)
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		fmt.Printf("  %s\n", line)
	}
	if !showSecrets {
		fmt.Println("\n(use --show-secrets to reveal masked values)")
	}
}

// maskSensitive returns a copy of rawConfig with sensitive field values
// replaced with "***". Recognizes common sensitive field names; conservative
// (false-positive safe — over-masking is OK, leaking is not).
func maskSensitive(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	return maskSensitiveValue(raw).(map[string]any)
}

func maskSensitiveValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, sub := range val {
			if isSensitiveKey(k) {
				if s, ok := sub.(string); ok && s != "" {
					out[k] = "***"
				} else {
					out[k] = maskSensitiveValue(sub)
				}
			} else {
				out[k] = maskSensitiveValue(sub)
			}
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, sub := range val {
			out[i] = maskSensitiveValue(sub)
		}
		return out
	default:
		return v
	}
}

// isSensitiveKey returns true for key names that commonly indicate secrets.
// Match is case-insensitive against key substrings.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	sensitiveSubstrings := []string{
		"token",
		"secret",
		"password",
		"passwd",
		"apikey",
		"api_key",
		"accesskey",
		"access_key",
		"privatekey",
		"private_key",
		"credential",
	}
	for _, sub := range sensitiveSubstrings {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}
