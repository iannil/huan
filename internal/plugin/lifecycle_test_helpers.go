package plugin

import (
	"context"
	"sync"

	"github.com/iannil/huan/internal/daemon/eventbus"
)

// lifecycleTestPlugin is a minimal plugin for lifecycle testing.
type lifecycleTestPlugin struct {
	name string
}

func (p *lifecycleTestPlugin) Name() string { return p.name }

// testEventSubscriberPlugin implements both Plugin and EventSubscriber.
type testEventSubscriberPlugin struct {
	name     string
	events   []eventbus.EventType
	received []eventbus.Event
	mu       sync.Mutex
}

func (p *testEventSubscriberPlugin) Name() string { return p.name }
func (p *testEventSubscriberPlugin) SubscribedEvents() []eventbus.EventType { return p.events }
func (p *testEventSubscriberPlugin) HandleEvent(ctx context.Context, event eventbus.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.received = append(p.received, event)
	return nil
}
func (p *testEventSubscriberPlugin) ReceivedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.received)
}

// Ensure testEventSubscriberPlugin satisfies EventSubscriber
var _ EventSubscriber = (*testEventSubscriberPlugin)(nil)

// testMetadataPlugin implements MetadataProvider for testing.
type testMetadataPlugin struct {
	lifecycleTestPlugin
	meta PluginMeta
}

func (p *testMetadataPlugin) PluginMetadata() PluginMeta { return p.meta }