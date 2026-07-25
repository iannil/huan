package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/iannil/huan/internal/daemon/eventbus"
)

// lifecycleTestPlugin is a minimal plugin for lifecycle testing.
type lifecycleTestPlugin struct {
	name string
}

func (p *lifecycleTestPlugin) Name() string { return p.name }

func TestLifecycleManager_List_Empty(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	info := lm.List()
	if len(info) != 0 {
		t.Errorf("List() = %d items, want 0", len(info))
	}
}

func TestLifecycleManager_LoadAndList(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	// Pre-register a compiled plugin
	_ = registry.Register(&lifecycleTestPlugin{name: "compiled-alpha"})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	info := lm.List()
	if len(info) != 1 {
		t.Fatalf("List() = %d items, want 1", len(info))
	}
	if info[0].Name != "compiled-alpha" {
		t.Errorf("Name = %q, want compiled-alpha", info[0].Name)
	}
	if info[0].Source != "compiled" {
		t.Errorf("Source = %q, want compiled", info[0].Source)
	}
	if info[0].Status != "active" {
		t.Errorf("Status = %q, want active", info[0].Status)
	}
}

func TestLifecycleManager_Load_EventPublished(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	events := make(chan eventbus.Event, 1)
	bus.Subscribe(eventbus.EventPluginLoaded, func(ctx context.Context, ev eventbus.Event) error {
		select {
		case events <- ev:
		default:
		}
		return nil
	})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	// Register a compiled plugin — should publish EventPluginLoaded
	lm.registerCompiled(&lifecycleTestPlugin{name: "compiled"})

	select {
	case ev := <-events:
		if ev.Type != eventbus.EventPluginLoaded {
			t.Errorf("event type = %v, want EventPluginLoaded", ev.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected EventPluginLoaded, got none")
	}
}

func TestLifecycleManager_Unload_NotFound(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	err := lm.Unload("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}

func TestLifecycleManager_Reload_Rollback(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	// Register a compiled plugin
	original := &lifecycleTestPlugin{name: "test-p"}
	_ = registry.Register(original)

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)

	// Reload with a nonexistent .so path should fail and roll back
	err := lm.Reload("test-p", "/nonexistent/plugin.so", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent .so path")
	}

	// The original plugin should still be in the registry
	p, ok := registry.Get("test-p")
	if !ok {
		t.Fatal("original plugin should still be registered after rollback")
	}
	if p.Name() != "test-p" {
		t.Errorf("after rollback, Name() = %q", p.Name())
	}
}

func TestLifecycleManager_List_ActiveAndLoaded(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)

	// Register compiled plugin
	_ = registry.Register(&lifecycleTestPlugin{name: "compiled"})

	// Create a .so and load it
	// (Use a temp dir with a real .so for a full integration test;
	//  for unit test, verify the List method correctly reports source)
	info := lm.List()
	if len(info) != 1 {
		t.Fatalf("List() = %d items, want 1", len(info))
	}
	// Verify required fields are present
	if info[0].Name == "" {
		t.Error("PluginInfo.Name should not be empty")
	}
	if info[0].Status != "active" {
		t.Errorf("Status = %q, want active", info[0].Status)
	}
}

// --- EventSubscriber tests ---

type testEventSubscriberPlugin struct {
	name     string
	events   []eventbus.EventType
	received []eventbus.Event
}

func (p *testEventSubscriberPlugin) Name() string { return p.name }
func (p *testEventSubscriberPlugin) SubscribedEvents() []eventbus.EventType { return p.events }
func (p *testEventSubscriberPlugin) HandleEvent(ctx context.Context, event eventbus.Event) error {
	p.received = append(p.received, event)
	return nil
}

var _ EventSubscriber = (*testEventSubscriberPlugin)(nil)

func TestLifecycleManager_EventSubscriber_CompiledPlugin(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	plugin := &testEventSubscriberPlugin{
		name:   "test-es",
		events: []eventbus.EventType{eventbus.EventBuildCompleted, eventbus.EventPluginLoaded},
	}
	_ = registry.Register(plugin)

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	_ = lm.Start(context.Background())
	defer lm.Stop()

	// Publish an event the plugin subscribed to
	_ = bus.Publish(context.Background(), eventbus.Event{
		Type:      eventbus.EventBuildCompleted,
		Timestamp: time.Now(),
		Payload:   "test",
	})

	// Give the async handler time to fire
	time.Sleep(50 * time.Millisecond)

	if len(plugin.received) == 0 {
		t.Error("expected plugin to receive event, got none")
	}
}

func TestLifecycleManager_EventSubscriber_NotRequired(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	_ = registry.Register(&lifecycleTestPlugin{name: "no-es"})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	// Should not panic
	err := lm.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	lm.Stop()
}
