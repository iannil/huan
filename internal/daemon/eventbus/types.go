package eventbus

import (
	"context"
	"time"
)

// EventType identifies the kind of event.
type EventType int

const (
	EventContentChanged  EventType = iota // 内容变更（文件被修改/创建/删除）
	EventCacheUpdated                     // 缓存已更新（Serving 可刷新）
	EventBuildStarted                     // 构建开始
	EventBuildCompleted                   // 构建完成
	EventBuildFailed                      // 构建失败
	EventServerStart                      // 服务启动
	EventServerShutdown                   // 服务关闭中
)

// String returns the human-readable name of this event type.
func (et EventType) String() string {
	switch et {
	case EventContentChanged:
		return "content_changed"
	case EventCacheUpdated:
		return "cache_updated"
	case EventBuildStarted:
		return "build_started"
	case EventBuildCompleted:
		return "build_completed"
	case EventBuildFailed:
		return "build_failed"
	case EventServerStart:
		return "server_start"
	case EventServerShutdown:
		return "server_shutdown"
	default:
		return "unknown"
	}
}

// Event carries the event type, timestamp, and payload.
type Event struct {
	Type      EventType
	Timestamp time.Time
	Payload   any
}

// Handler processes an event. Returning an error logs the failure but does
// not interrupt other handlers for the same event type.
type Handler func(ctx context.Context, event Event) error