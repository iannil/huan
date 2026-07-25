// Package plugin defines the unified plugin host for huan extensions.
//
// Plugin is the minimal base interface every plugin satisfies. Capability
// interfaces (e.g. deploy.Deployer) embed Plugin and add domain-specific
// methods. The Registry holds plugins keyed by Name(); Find[T] returns the
// subset implementing a given capability.
//
// See docs/adr/0003-unified-plugin-system.md for the architectural decisions.
package plugin

import (
	"context"
	"fmt"
	"sort"

	"github.com/iannil/huan/internal/daemon/eventbus"
)

// Plugin is the base interface every plugin satisfies. Capability interfaces
// (Deployer, future PaymentProvider, etc.) embed Plugin and add methods.
//
// Plugin intentionally has only Name(): config injection happens via the
// plugin's constructor (e.g. cloudflare.New(cfg)), not via an Init method.
// Lifecycle hooks (Start/Stop), when needed, live on the specific capability
// interface rather than the base.
type Plugin interface {
	// Name is the plugin's unique identifier. It matches the yaml key under
	// plugins: (e.g. Name()=="cloudflare" pairs with yaml plugins.cloudflare.*).
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
// deploy.Deployer.
//
// Example:
//
//	deployers := plugin.Find[deploy.Deployer](registry)
//	for _, d := range deployers { ... }
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

// EventSubscriber is an optional interface plugins can implement to subscribe
// to system events. The LifecycleManager registers these subscriptions when
// the plugin is loaded (compiled or .so) in serve/dev mode.
type EventSubscriber interface {
	// SubscribedEvents returns the event types this plugin wants to receive.
	// Return nil or empty slice to skip all events.
	SubscribedEvents() []eventbus.EventType

	// HandleEvent is called for each subscribed event. Returning an error
	// logs the failure but does not interrupt other handlers.
	HandleEvent(ctx context.Context, event eventbus.Event) error
}
// their metadata. Used by the LifecycleManager.List() and CLI/Admin UI.
type MetadataProvider interface {
	PluginMetadata() PluginMeta
}
