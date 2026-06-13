package deploy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// EventType enumerates the structured log event types mandated by CLAUDE.md
// (Full-Lifecycle Observability).
type EventType string

const (
	EventFunctionStart EventType = "Function_Start"
	EventFunctionEnd   EventType = "Function_End"
	EventBranch        EventType = "Branch"
	EventError         EventType = "Error"
	EventPoint         EventType = "Point"
)

// Logger emits JSON-structured log lines to stderr (default). Each line has:
//
//	{
//	  "timestamp": "2026-06-13T...",
//	  "trace_id": "<deploy invocation id>",
//	  "span_id": "<step id within this deploy>",
//	  "event_type": "Function_Start|Function_End|Branch|Error|Point",
//	  "payload": { ... arbitrary fields ... }
//	}
//
// The Logger keeps no state about active spans — callers pass an explicit
// spanID per call. This avoids the implicit-global-state footguns common in
// context-based loggers and makes the log calls grep-friendly.
//
// Logger is safe for concurrent use.
type Logger struct {
	traceID string
	out     io.Writer
	enc     *json.Encoder
}

// NewLogger returns a Logger that writes to stderr. traceID is propagated to
// every log line for correlation; if empty, a random one is generated.
func NewLogger(traceID string) *Logger {
	return NewLoggerWithWriter(traceID, os.Stderr)
}

// NewLoggerWithWriter is like NewLogger but lets tests inject a buffer.
// traceID is auto-generated when empty, matching NewLogger's contract.
func NewLoggerWithWriter(traceID string, w io.Writer) *Logger {
	if traceID == "" {
		traceID = newTraceID()
	}
	l := &Logger{
		traceID: traceID,
		out:     w,
	}
	l.enc = json.NewEncoder(l.out)
	return l
}

// TraceID returns the trace id used for all lines from this logger.
func (l *Logger) TraceID() string { return l.traceID }

// Log emits one structured log line. payload may be nil.
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

// LogFunctionStart and LogFunctionEnd record the boundary of a logical step
// (e.g. an HTTP API call). They return the same spanID for chaining.
//
//	start := l.LogFunctionStart("upload-file", map[string]any{"path": p})
//	defer l.LogFunctionEnd(start, time.Since(...), nil)
func (l *Logger) LogFunctionStart(spanID string, payload map[string]any) string {
	l.Log(spanID, EventFunctionStart, payload)
	return spanID
}

func (l *Logger) LogFunctionEnd(spanID string, duration time.Duration, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["duration_ms"] = duration.Milliseconds()
	l.Log(spanID, EventFunctionEnd, payload)
}

// newTraceID generates a random 16-byte hex-encoded id (32 chars).
// Used when callers don't supply one.
func newTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// rand.Read should never fail; fall back to timestamp if it does.
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
