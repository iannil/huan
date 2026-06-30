package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AuditLogger writes admin write-operations to the project's daily note
// (memory/daily/{YYYY-MM-DD}.md), implementing ADR 0011 L4. The log is
// appended in markdown section format that fits the existing daily-note
// convention, so audit entries are grep-able, git-tracked, and reviewable
// alongside other daily context.
type AuditLogger struct {
	memoryDir string // typically <sourceDir>/memory/daily
	mu        sync.Mutex
}

// NewAuditLogger returns a logger that writes to memoryDir/{YYYY-MM-DD}.md.
// The directory is created on first write if missing.
func NewAuditLogger(memoryDir string) *AuditLogger {
	return &AuditLogger{memoryDir: memoryDir}
}

// AuditAction is a string-typed enum for the audit log entry kind.
type AuditAction string

const (
	ActionContentCreate AuditAction = "content.create"
	ActionContentUpdate AuditAction = "content.update"
	ActionContentDelete AuditAction = "content.delete"
	ActionSettingsUpdate AuditAction = "settings.update"
	ActionSettingsYAML   AuditAction = "settings.yaml.update"
	ActionMediaUpload    AuditAction = "media.upload"
	ActionMediaDelete    AuditAction = "media.delete"
)

// AuditRecord captures one admin write-operation.
// BeforeSHA is the hex-encoded SHA256 of the file before the operation
// (empty for create). AfterSHA is the hex-encoded SHA256 after (empty for
// delete).
type AuditRecord struct {
	Action    AuditAction
	Path      string // relative path within content/ static/ or "huan.yaml"
	BeforeSHA string
	AfterSHA  string
	OccurredAt time.Time
}

// Log appends one audit record to today's daily note. The format matches
// the daily-note section style so entries grep cleanly alongside manual
// notes. Concurrent calls are serialized by a mutex (file-level locking
// would also work, but mutex is sufficient for a single-process server).
func (a *AuditLogger) Log(rec AuditRecord) error {
	if a == nil || a.memoryDir == "" {
		return nil // audit disabled (e.g., tests with no memoryDir)
	}
	if rec.OccurredAt.IsZero() {
		rec.OccurredAt = time.Now()
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := os.MkdirAll(a.memoryDir, 0o755); err != nil {
		return fmt.Errorf("audit: create memory dir: %w", err)
	}

	dateStr := rec.OccurredAt.Format("2006-01-02")
	timeStr := rec.OccurredAt.Format("15:04:05")
	path := filepath.Join(a.memoryDir, dateStr+".md")

	var sb strings.Builder
	sb.WriteString("\n### admin audit (")
	sb.WriteString(timeStr)
	sb.WriteString(")\n\n")
	sb.WriteString("- **action**: `")
	sb.WriteString(string(rec.Action))
	sb.WriteString("`\n")
	sb.WriteString("- **path**: `")
	sb.WriteString(rec.Path)
	sb.WriteString("`\n")
	switch {
	case rec.BeforeSHA == "" && rec.AfterSHA == "":
		// No SHA info (shouldn't happen for write ops; defensive)
	case rec.BeforeSHA == "":
		sb.WriteString("- **sha256**: _new_ → `")
		sb.WriteString(rec.AfterSHA)
		sb.WriteString("`\n")
	case rec.AfterSHA == "":
		sb.WriteString("- **sha256**: `")
		sb.WriteString(rec.BeforeSHA)
		sb.WriteString("` → _deleted_\n")
	default:
		if rec.BeforeSHA == rec.AfterSHA {
			sb.WriteString("- **sha256**: `")
			sb.WriteString(rec.AfterSHA)
			sb.WriteString("` (unchanged)\n")
		} else {
			sb.WriteString("- **sha256**: `")
			sb.WriteString(rec.BeforeSHA)
			sb.WriteString("` → `")
			sb.WriteString(rec.AfterSHA)
			sb.WriteString("`\n")
		}
	}
	sb.WriteString("\n")

	// O_APPEND is atomic for writes < PIPE_BUF (4KB on Linux); our entries
	// are well under that, but the mutex above guarantees serialization
	// regardless.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("audit: open daily note: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(sb.String()); err != nil {
		return fmt.Errorf("audit: write daily note: %w", err)
	}
	return nil
}

// ComputeSHA256 reads filePath and returns hex-encoded SHA256.
// Returns "" + error if the file cannot be read.
func ComputeSHA256(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
