package deploy

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewLogger_GeneratesTraceIDWhenEmpty(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoggerWithWriter("", &buf)
	if l.TraceID() == "" {
		t.Fatal("TraceID empty")
	}
	if len(l.TraceID()) != 32 {
		t.Errorf("TraceID len = %d, want 32 (16 bytes hex)", len(l.TraceID()))
	}
}

func TestNewLogger_PreservesSuppliedTraceID(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoggerWithWriter("my-trace-123", &buf)
	if got := l.TraceID(); got != "my-trace-123" {
		t.Errorf("TraceID = %q", got)
	}
}

func TestLog_EmitsValidJSONWithAllFields(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoggerWithWriter("trace-1", &buf)

	l.Log("span-1", EventFunctionStart, map[string]any{
		"endpoint": "/foo",
		"method":   "GET",
	})

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}

	required := []string{"timestamp", "trace_id", "span_id", "event_type", "payload"}
	for _, k := range required {
		if _, ok := parsed[k]; !ok {
			t.Errorf("missing field %q in %s", k, buf.String())
		}
	}
	if parsed["trace_id"] != "trace-1" {
		t.Errorf("trace_id = %v", parsed["trace_id"])
	}
	if parsed["span_id"] != "span-1" {
		t.Errorf("span_id = %v", parsed["span_id"])
	}
	if parsed["event_type"] != "Function_Start" {
		t.Errorf("event_type = %v", parsed["event_type"])
	}
}

func TestLog_NilPayload_OmittedFromJSON(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoggerWithWriter("trace-1", &buf)
	l.Log("span-1", EventPoint, nil)

	var parsed map[string]any
	_ = json.Unmarshal(buf.Bytes(), &parsed)
	// payload field is omitempty when nil; check it's absent or null.
	if v, ok := parsed["payload"]; ok && v != nil {
		t.Errorf("payload = %v, want nil or absent", v)
	}
}

func TestLogError_AddsErrorFieldToPayload(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoggerWithWriter("trace-1", &buf)
	l.LogError("span-1", errFoo("boom"), map[string]any{"stage": "upload"})

	var parsed map[string]any
	_ = json.Unmarshal(buf.Bytes(), &parsed)
	payload, _ := parsed["payload"].(map[string]any)
	if payload["error"] != "boom" {
		t.Errorf("payload.error = %v", payload["error"])
	}
	if payload["stage"] != "upload" {
		t.Errorf("payload.stage = %v", payload["stage"])
	}
	if parsed["event_type"] != "Error" {
		t.Errorf("event_type = %v", parsed["event_type"])
	}
}

func TestLogError_NilPayload_StillGetsErrorField(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoggerWithWriter("trace-1", &buf)
	l.LogError("span-1", errFoo("oops"), nil)

	if !strings.Contains(buf.String(), `"error":"oops"`) {
		t.Errorf("output missing error field: %s", buf.String())
	}
}

func TestLogFunctionStartEnd_ProducesPairedEvents(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoggerWithWriter("trace-1", &buf)

	span := l.LogFunctionStart("upload", map[string]any{"file": "/x"})
	time.Sleep(2 * time.Millisecond)
	l.LogFunctionEnd(span, 5*time.Millisecond, nil)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	var start, end map[string]any
	_ = json.Unmarshal(lines[0], &start)
	_ = json.Unmarshal(lines[1], &end)
	if start["event_type"] != "Function_Start" {
		t.Errorf("line 1 event_type = %v", start["event_type"])
	}
	if end["event_type"] != "Function_End" {
		t.Errorf("line 2 event_type = %v", end["event_type"])
	}
	endPayload, _ := end["payload"].(map[string]any)
	if endPayload["duration_ms"] == nil {
		t.Errorf("end event missing duration_ms: %v", end)
	}
}

type errFoo string

func (e errFoo) Error() string { return string(e) }
