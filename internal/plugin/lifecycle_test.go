package plugin

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iannil/huan/internal/daemon/eventbus"
)

func TestLifecycleManager_List_Empty(t *testing.T) {
	lm := NewLifecycleManager(NewRegistry(), NewLoader(t.TempDir()), eventbus.NewChannelBus())
	info := lm.List()
	if len(info) != 0 {
		t.Errorf("List() = %d items, want 0", len(info))
	}
}

func TestLifecycleManager_List_IncludesCompiled(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&lifecycleTestPlugin{name: "compiled-alpha"})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), eventbus.NewChannelBus())
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

func TestLifecycleManager_List_MetadataProvider(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&testMetadataPlugin{
		lifecycleTestPlugin: lifecycleTestPlugin{name: "meta-p"},
		meta: PluginMeta{
			Version: "1.0.0", Author: "test", License: "MIT",
			Tags: []string{"seo", "deploy"}, IsOfficial: true,
		},
	})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), eventbus.NewChannelBus())
	info := lm.List()
	if len(info) != 1 {
		t.Fatalf("List() = %d items, want 1", len(info))
	}
	if info[0].Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", info[0].Version)
	}
	if info[0].Author != "test" {
		t.Errorf("Author = %q, want test", info[0].Author)
	}
	if info[0].License != "MIT" {
		t.Errorf("License = %q, want MIT", info[0].License)
	}
	if len(info[0].Tags) != 2 || info[0].Tags[0] != "seo" {
		t.Errorf("Tags = %v", info[0].Tags)
	}
}

func TestLifecycleManager_Start_CompiledPlugins(t *testing.T) {
	t.Setenv("HUAN_HOME", t.TempDir()) // isolate from the real ~/.huan
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	_ = registry.Register(&lifecycleTestPlugin{name: "compiled"})

	var loadedCount atomic.Int32
	bus.Subscribe(eventbus.EventPluginLoaded, func(ctx context.Context, ev eventbus.Event) error {
		loadedCount.Add(1)
		return nil
	})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	err := lm.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer lm.Stop()

	time.Sleep(50 * time.Millisecond)
	if got := loadedCount.Load(); got != 1 {
		t.Errorf("expected 1 EventPluginLoaded, got %d", got)
	}
}

func TestLifecycleManager_Unload_CompiledPlugin(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&lifecycleTestPlugin{name: "compiled"})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), eventbus.NewChannelBus())
	err := lm.Unload("compiled")
	if err == nil {
		t.Fatal("expected error for compiled plugin unload")
	}
}

func TestLifecycleManager_Unload_NonExistent(t *testing.T) {
	lm := NewLifecycleManager(NewRegistry(), NewLoader(t.TempDir()), eventbus.NewChannelBus())
	err := lm.Unload("nonexistent")
	if err != ErrPluginNotFound {
		t.Errorf("Unload = %v, want ErrPluginNotFound", err)
	}
}

func TestLifecycleManager_Reload_CompiledPlugin(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&lifecycleTestPlugin{name: "compiled"})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), eventbus.NewChannelBus())
	err := lm.Reload("compiled", "/nonexistent.so", nil)
	if err == nil {
		t.Fatal("expected error for compiled plugin reload")
	}
}

func TestLifecycleManager_Reload_Rollback(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&lifecycleTestPlugin{name: "test-p"})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), eventbus.NewChannelBus())

	err := lm.Reload("test-p", "/nonexistent/plugin.so", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent .so path")
	}

	p, ok := registry.Get("test-p")
	if !ok {
		t.Fatal("original plugin should still be registered after rollback")
	}
	if p.Name() != "test-p" {
		t.Errorf("after rollback, Name() = %q", p.Name())
	}
}

func TestLifecycleManager_Stop_Cleanup(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	_ = registry.Register(&lifecycleTestPlugin{name: "compiled"})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	_ = lm.Start(context.Background())

	lm.Stop()

	_, ok := registry.Get("compiled")
	if !ok {
		t.Error("compiled plugins should survive Stop")
	}
}

func TestLifecycleManager_EventSubscriber_CompiledPlugin(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	plugin := &testEventSubscriberPlugin{
		name:   "test-es",
		events: []eventbus.EventType{eventbus.EventBuildCompleted},
	}
	_ = registry.Register(plugin)

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	_ = lm.Start(context.Background())
	defer lm.Stop()

	time.Sleep(50 * time.Millisecond)

	_ = bus.Publish(context.Background(), eventbus.Event{
		Type:      eventbus.EventBuildCompleted,
		Timestamp: time.Now(),
		Payload:   "test",
	})

	time.Sleep(50 * time.Millisecond)

	if got := plugin.ReceivedCount(); got == 0 {
		t.Error("plugin should receive subscribed event")
	}
}

func TestLifecycleManager_EventSubscriber_NotRequired(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&lifecycleTestPlugin{name: "no-es"})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), eventbus.NewChannelBus())
	err := lm.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	lm.Stop()
}

func TestLifecycleManager_EventSubscriber_EmptyEvents(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	plugin := &testEventSubscriberPlugin{
		name:   "empty-es",
		events: []eventbus.EventType{},
	}
	_ = registry.Register(plugin)

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	err := lm.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	lm.Stop()
}