// Package plugin defines the unified plugin host for huan extensions.
//
// Plugin is the minimal base interface every plugin satisfies. All base types
// are aliased from pkg/plugin so .so plugins and internal code share the same
// type identity. EventSubscriber stays defined here because it references
// internal eventbus types.
//
// Capability interfaces (e.g. deploy.Deployer) embed Plugin and add domain-
// specific methods. The Registry holds plugins keyed by Name(); Find[T] returns
// the subset implementing a given capability.
//
// See docs/adr/0003-unified-plugin-system.md for the architectural decisions.
package plugin

import (
	"context"

	"github.com/iannil/huan/internal/daemon/eventbus"
	pkgplugin "github.com/iannil/huan/pkg/plugin"
)

// All base types are aliased from pkg/plugin so .so plugins and huan internal
// code share the same type identity.
type Plugin = pkgplugin.Plugin
type PluginMeta = pkgplugin.PluginMeta
type MetadataProvider = pkgplugin.MetadataProvider
type SchemaProvider = pkgplugin.SchemaProvider
type Schema = pkgplugin.Schema
type FieldSchema = pkgplugin.FieldSchema

// Registry is aliased from pkg/plugin — struct with all methods (Register, Get,
// All, Unregister, Names, SortedNames) defined there.
type Registry = pkgplugin.Registry

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return pkgplugin.NewRegistry() }

// Find returns all registered plugins implementing capability T, in
// registration order. T is typically a capability interface.
func Find[T any](r *Registry) []T { return pkgplugin.Find[T](r) }

// EventSubscriber is an optional interface plugins can implement to subscribe
// to system events. The LifecycleManager registers these subscriptions when
// the plugin is loaded (compiled or .so) in serve/dev mode.
//
// This interface stays in internal/plugin because it references internal
// daemon/eventbus types that are not exported from pkg/plugin.
type EventSubscriber interface {
	// SubscribedEvents returns the event types this plugin wants to receive.
	// Return nil or empty slice to skip all events.
	SubscribedEvents() []eventbus.EventType

	// HandleEvent is called for each subscribed event. Returning an error
	// logs the failure but does not interrupt other handlers.
	HandleEvent(ctx context.Context, event eventbus.Event) error
}
