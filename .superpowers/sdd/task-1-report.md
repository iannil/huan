# Task 1 Report: EventSubscriber 接口 + LifecycleManager 集成

**Status:** COMPLETED

**Commit:** 93a2ff5

## Changes

### `internal/plugin/plugin.go`
- Added `context` and `eventbus` imports
- Added `EventSubscriber` interface with `SubscribedEvents()` and `HandleEvent()` methods

### `internal/plugin/lifecycle.go`
- Added `subscriptionEntry` type (holds `eventType` + `handlerID`)
- Added `subscriptionIDs map[string][]subscriptionEntry` field to `LifecycleManager`
- Initialized `subscriptionIDs` in `NewLifecycleManager`
- Added `subscribePluginEvents()` method: checks if plugin implements `EventSubscriber`, subscribes to declared events with handler ID format `"plugin:<name>:<eventType>"`
- Added `unsubscribePluginEvents()` method: removes all subscriptions for a plugin
- `Start()`: subscribes compiled plugins after watcher startup
- `Stop()`: unsubscribes all plugin event handlers before cleanup
- `Load()`: subscribes newly loaded .so plugin events after registration
- `Unload()`: unsubscribes plugin events before unregistering

### `internal/plugin/lifecycle_test.go`
- Added `testEventSubscriberPlugin` test helper implementing `EventSubscriber`
- `TestLifecycleManager_EventSubscriber_CompiledPlugin`: registers a compiled plugin that subscribes to `EventBuildCompleted` and `EventPluginLoaded`, verifies it receives the event
- `TestLifecycleManager_EventSubscriber_NotRequired`: verifies plugins without `EventSubscriber` don't cause panics

## Test Results

All 45 tests pass (including 2 new EventSubscriber tests).

## Build

`go build ./cmd/huan` succeeds.

## Concerns

- `subscribePluginEvents` calls `m.bus.Subscribe` but discards the returned handler ID, then creates its own `"plugin:<name>:<eventType>"` ID. This works because both the auto-generated ID and our custom ID are stored; our `unsubscribePluginEvents` only uses our custom ID. The auto-generated IDs from `bus.Subscribe` remain in the bus's internal counter but are never used for unsubscribe -- they are harmless but worth noting.
- The `Reload` method does not handle event subscriptions: after a reload, the new plugin instance should be subscribed and the old one unsubscribed. This is a gap for future work.
- The `PluginWatcher.handleCreateOrModify` also does not subscribe new plugins to events. This is a gap for future work.
