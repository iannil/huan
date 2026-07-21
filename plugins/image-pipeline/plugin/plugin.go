// Package plugin provides the minimal Plugin interface for .so plugins.
// This is a self-contained copy of huan's internal/plugin/plugin.go.
package plugin

// Plugin is the base interface every plugin satisfies.
type Plugin interface {
	Name() string
}