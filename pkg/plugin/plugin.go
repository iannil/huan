// Package plugin provides canonical type definitions for the huan plugin system.
//
// Both huan internal code and .so plugins import these types, solving Go's
// cross-module type assertion problem for interface-based capability discovery.
//
// Plugin is the minimal base interface every plugin satisfies. Capability
// interfaces (e.g. pkg/plugin.Hook, pkg/plugin.ThemePlugin) embed Plugin and
// add domain-specific methods. The Registry holds plugins keyed by Name();
// Find[T] returns the subset implementing a given capability.
package plugin

import (
	"fmt"
	"sort"
)

// Plugin is the base interface every plugin satisfies. Capability interfaces
// embed Plugin and add methods.
//
// Plugin intentionally has only Name(): config injection happens via the
// plugin's constructor (e.g. seoinjector.New(cfg)), not via an Init method.
type Plugin interface {
	// Name is the plugin's unique identifier. It matches the yaml key under
	// plugins: (e.g. Name()=="seo_injector" pairs with yaml plugins.seo_injector.*).
	Name() string
}

// Registry holds plugins keyed by Name(). The order slice preserves
// registration order for deterministic iteration in Find[T] and All.
type Registry struct {
	plugins map[string]Plugin
	order   []string
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
	}
}

// Register adds a plugin to the registry. Returns an error if a plugin with
// the same Name() is already registered — duplicate registration is treated
// as a programming error rather than silently overwritten.
func (r *Registry) Register(p Plugin) error {
	if p == nil {
		return fmt.Errorf("plugin: register nil")
	}
	name := p.Name()
	if name == "" {
		return fmt.Errorf("plugin: empty name")
	}
	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin: duplicate registration %q", name)
	}
	r.plugins[name] = p
	r.order = append(r.order, name)
	return nil
}

// Get returns the plugin with the given name and a found flag.
func (r *Registry) Get(name string) (Plugin, bool) {
	p, ok := r.plugins[name]
	return p, ok
}

// All returns all registered plugins in registration order. The returned
// slice is a copy; callers may mutate it without affecting the registry.
func (r *Registry) All() []Plugin {
	out := make([]Plugin, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.plugins[name])
	}
	return out
}

// Unregister removes a plugin by name. Returns false if the name wasn't
// registered. After Unregister, the plugin is no longer returned by Get,
// All, Names, or Find[T].
func (r *Registry) Unregister(name string) bool {
	if _, exists := r.plugins[name]; !exists {
		return false
	}
	delete(r.plugins, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return true
}

// Names returns all registered plugin names in registration order.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Find returns all registered plugins implementing capability T, in
// registration order. T is typically a capability interface such as
// pkg/plugin.Hook or pkg/plugin.ThemePlugin.
//
// Example:
//
//	hooks := plugin.Find[pkgplugin.Hook](registry)
//	for _, h := range hooks { ... }
func Find[T any](r *Registry) []T {
	var out []T
	for _, p := range r.All() {
		if t, ok := p.(T); ok {
			out = append(out, t)
		}
	}
	return out
}

// SortedNames returns registered plugin names in lexicographic order. Useful
// for CLI listing where deterministic alphabetical output is preferred over
// registration order.
func (r *Registry) SortedNames() []string {
	out := r.Names()
	sort.Strings(out)
	return out
}

// PluginMeta carries human-readable metadata for a plugin.
type PluginMeta struct {
	Version    string   `json:"version"`
	Author     string   `json:"author"`
	RepoURL    string   `json:"repoURL"`
	License    string   `json:"license"`
	Tags       []string `json:"tags"`
	IsOfficial bool     `json:"isOfficial"`
}

// MetadataProvider is an optional interface plugins can implement to provide
// their metadata. Used by the LifecycleManager.List() and Admin/CLI UI.
type MetadataProvider interface {
	PluginMetadata() PluginMeta
}

// SchemaProvider is an optional interface plugins can implement to declare
// their config schema. Used by the registry for config validation.
type SchemaProvider interface {
	ConfigSchema() Schema
}

// Schema describes the full config shape a plugin expects.
type Schema struct {
	Fields []FieldSchema
}

// FieldSchema describes a single config field.
type FieldSchema struct {
	Key         string // 字段名，对应 yaml key
	Type        string // "string" | "int" | "bool" | "string_slice" | "map"
	Required    bool   // true = 必填，启动时校验
	Default     any    // 默认值（Required=false 时生效）
	Description string // 人类可读的说明
	Sensitive   bool   // true = 在 CLI info 中 mask 为 ***
	EnvVarHint  string // 建议的环境变量名，仅用于文档提示
}
