// Package observability provides cross-cutting structured logging.
// This is a self-contained copy of internal/observability/logging.go for plugin isolation.
package observability

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// EventType enumerates the structured log event types.
type EventType string

const (
	EventFunctionStart EventType = "Function_Start"
	EventFunctionEnd   EventType = "Function_End"
	EventBranch        EventType = "Branch"
	EventError         EventType = "Error"
	EventPoint         EventType = "Point"
)

// Logger emits JSON-structured log lines to stderr (default).
type Logger struct {
	traceID string
	out     io.Writer
	enc     *json.Encoder
}

// NewLogger returns a Logger that writes to stderr.
func NewLogger(traceID string) *Logger {
	return NewLoggerWithWriter(traceID, os.Stderr)
}

// NewLoggerWithWriter is like NewLogger but lets tests inject a buffer.
func NewLoggerWithWriter(traceID string, w io.Writer) *Logger {
	if traceID == "" {
		traceID = NewTraceID()
	}
	l := &Logger{
		traceID: traceID,
		out:     w,
	}
	l.enc = json.NewEncoder(l.out)
	return l
}

// NewTraceID generates a random 16-byte hex-encoded id (32 chars).
func NewTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// TraceID returns the trace id used for all lines from this logger.
func (l *Logger) TraceID() string { return l.traceID }

// Log emits one structured log line.
func (l *Logger) Log(spanID string, event EventType, payload map[string]any) {
	entry := struct {
		Timestamp string         `json:"timestamp"`
		TraceID   string         `json:"trace_id"`
		SpanID    string         `json:"span_id"`
		EventType EventType      `json:"event_type"`
		Payload   map[string]any `json:"payload,omitempty"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:   l.traceID,
		SpanID:    spanID,
		EventType: event,
		Payload:   payload,
	}
	_ = l.enc.Encode(entry)
}

// LogError is a convenience wrapper for error events.
func (l *Logger) LogError(spanID string, err error, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["error"] = err.Error()
	l.Log(spanID, EventError, payload)
}

// LogFunctionStart records the start of a logical step.
func (l *Logger) LogFunctionStart(spanID string, payload map[string]any) string {
	l.Log(spanID, EventFunctionStart, payload)
	return spanID
}

// LogFunctionEnd records the end of a logical step.
func (l *Logger) LogFunctionEnd(spanID string, duration time.Duration, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["duration_ms"] = duration.Milliseconds()
	l.Log(spanID, EventFunctionEnd, payload)
}