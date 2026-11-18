package desktop

import (
	"fmt"
	"strings"
	"sync"
)

// DiagLevel classifies a startup diagnostic item.
type DiagLevel string

const (
	LevelWarn  DiagLevel = "warn"
	LevelError DiagLevel = "error"
)

// notifStartupDiag is the fixed notification ID: re-showing replaces the
// previous one (no pile-up), and the click handler routes on it.
const notifStartupDiag = "startup-diag"

// DiagItem is one startup-time problem surfaced in the desktop UI.
// Component is one of "plugin" | "serve" | "build" | "site".
type DiagItem struct {
	Level     DiagLevel
	Component string
	Message   string
}

// Diagnostics collects startup-time problems (plugin load warnings, serve
// start failures, ...). Safe for concurrent use: Add may be called from the
// serve goroutine while Snapshot is read on the main thread.
type Diagnostics struct {
	mu    sync.Mutex
	items []DiagItem
}

// NewDiagnostics returns an empty collector.
func NewDiagnostics() *Diagnostics { return &Diagnostics{} }

// Add records one diagnostic item, skipping an identical (level, component,
// msg) entry already collected — repeated rebuilds re-fire the same plugin
// warnings and would otherwise inflate the badge count and duplicate dialog
// lines. Distinct problems always collect. Structured logging is the caller's job.
func (d *Diagnostics) Add(level DiagLevel, component, msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, it := range d.items {
		if it.Level == level && it.Component == component && it.Message == msg {
			return
		}
	}
	d.items = append(d.items, DiagItem{Level: level, Component: component, Message: msg})
}

// Snapshot returns a copy of the collected items.
func (d *Diagnostics) Snapshot() []DiagItem {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]DiagItem, len(d.items))
	copy(out, d.items)
	return out
}

// HasErrors reports whether any error-level item is present.
func (d *Diagnostics) HasErrors() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, it := range d.items {
		if it.Level == LevelError {
			return true
		}
	}
	return false
}

// Reset clears all items. startAndServe calls it when a new serve session
// begins, so diagnostics always describe the current startup.
func (d *Diagnostics) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.items = nil
}

// diagSummary renders a snapshot as dialog message and detail block. An
// empty snapshot reads as "no issues found".
func diagSummary(items []DiagItem) (message, detail string) {
	if len(items) == 0 {
		return "本次启动未发现问题", ""
	}
	message = fmt.Sprintf("本次启动发现 %d 个问题", len(items))
	var b strings.Builder
	for _, it := range items {
		level := "警告"
		if it.Level == LevelError {
			level = "错误"
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", level, it.Component, it.Message)
	}
	return message, strings.TrimRight(b.String(), "\n")
}

// notifyTitle renders the startup notification title by severity mix.
func notifyTitle(errs, warns int) string {
	if errs > 0 {
		return fmt.Sprintf("huan 启动存在问题（%d 个错误，%d 个警告）", errs, warns)
	}
	return fmt.Sprintf("huan 已启动，有 %d 个警告", warns)
}

// truncMsg caps s at n runes, appending an ellipsis when cut.
func truncMsg(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
