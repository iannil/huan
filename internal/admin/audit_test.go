package admin

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAuditLog_Create_RecordsAfterSHAOnly verifies the L4 contract for
// content.create: the entry has _new_ as before-SHA and the file's actual
// SHA256 as after-SHA.
func TestAuditLog_Create_RecordsAfterSHAOnly(t *testing.T) {
	tmp := t.TempDir()
	logger := NewAuditLogger(tmp)

	if err := logger.Log(AuditRecord{
		Action:    ActionContentCreate,
		Path:      "posts/new.md",
		AfterSHA:  "abc123",
		OccurredAt: time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "2026-06-30.md"))
	if err != nil {
		t.Fatalf("read daily note: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "content.create") {
		t.Errorf("daily note missing action: %s", s)
	}
	if !strings.Contains(s, "posts/new.md") {
		t.Errorf("daily note missing path: %s", s)
	}
	if !strings.Contains(s, "_new_") {
		t.Errorf("daily note missing _new_ marker for create: %s", s)
	}
	if !strings.Contains(s, "abc123") {
		t.Errorf("daily note missing after-SHA: %s", s)
	}
}

// TestAuditLog_Update_RecordsBeforeAndAfterSHA verifies the update case
// shows before→after transition.
func TestAuditLog_Update_RecordsBeforeAndAfterSHA(t *testing.T) {
	tmp := t.TempDir()
	logger := NewAuditLogger(tmp)

	if err := logger.Log(AuditRecord{
		Action:    ActionContentUpdate,
		Path:      "posts/foo.md",
		BeforeSHA: "before-hash",
		AfterSHA:  "after-hash",
	}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmp, time.Now().Format("2006-01-02")+".md"))
	s := string(data)
	if !strings.Contains(s, "before-hash") || !strings.Contains(s, "after-hash") {
		t.Errorf("daily note missing before/after SHA: %s", s)
	}
	if !strings.Contains(s, "`before-hash` → `after-hash`") {
		t.Errorf("daily note missing arrow transition: %s", s)
	}
}

// TestAuditLog_Delete_RecordsBeforeSHAOnly verifies the delete case shows
// before-SHA → _deleted_.
func TestAuditLog_Delete_RecordsBeforeSHAOnly(t *testing.T) {
	tmp := t.TempDir()
	logger := NewAuditLogger(tmp)

	if err := logger.Log(AuditRecord{
		Action:    ActionContentDelete,
		Path:      "posts/gone.md",
		BeforeSHA: "was-here",
	}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmp, time.Now().Format("2006-01-02")+".md"))
	s := string(data)
	if !strings.Contains(s, "_deleted_") {
		t.Errorf("daily note missing _deleted_ marker: %s", s)
	}
	if !strings.Contains(s, "was-here") {
		t.Errorf("daily note missing before-SHA: %s", s)
	}
}

// TestAuditLog_ConcurrentWrites_ThreadSafe runs many goroutines writing
// audit entries simultaneously, then verifies the daily note has all of
// them. CI race gate (go test -race) catches data races; this test also
// catches logical corruption from interleaved writes.
func TestAuditLog_ConcurrentWrites_ThreadSafe(t *testing.T) {
	tmp := t.TempDir()
	logger := NewAuditLogger(tmp)

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			_ = logger.Log(AuditRecord{
				Action: ActionContentUpdate,
				Path:   "posts/concurrent.md",
			})
		}(i)
	}
	wg.Wait()

	data, _ := os.ReadFile(filepath.Join(tmp, time.Now().Format("2006-01-02")+".md"))
	// Each entry writes exactly one "content.update" string. Count occurrences.
	count := strings.Count(string(data), "content.update")
	if count != N {
		t.Errorf("wrote %d entries, daily note has %d", N, count)
	}
}

// TestAuditLog_NilLogger_NoOp verifies the nil-safe contract: callers can
// pass a nil logger (e.g., in tests without memoryDir) and Log returns
// nil without panic.
func TestAuditLog_NilLogger_NoOp(t *testing.T) {
	var nilLogger *AuditLogger
	if err := nilLogger.Log(AuditRecord{Action: ActionContentCreate}); err != nil {
		t.Errorf("nil logger Log = %v, want nil", err)
	}
}

// TestAuditLog_EmptyMemoryDir_NoOp verifies the same nil-safe contract
// when memoryDir is empty (disabled audit).
func TestAuditLog_EmptyMemoryDir_NoOp(t *testing.T) {
	logger := NewAuditLogger("")
	if err := logger.Log(AuditRecord{Action: ActionContentCreate}); err != nil {
		t.Errorf("empty memoryDir Log = %v, want nil", err)
	}
}

// TestComputeSHA256 verifies the SHA matches a known value for stable
// input. This pins the algorithm — switching away from SHA256 would
// silently invalidate all audit log entries.
func TestComputeSHA256(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// echo -n "hello" | sha256sum
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	got, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("ComputeSHA256: %v", err)
	}
	if got != want {
		t.Errorf("ComputeSHA256(hello) = %q, want %q", got, want)
	}
}

// TestComputeSHA256_MissingFile verifies error behavior for the create
// before-SHA case (file doesn't exist yet).
func TestComputeSHA256_MissingFile(t *testing.T) {
	_, err := ComputeSHA256("/nonexistent/path/file.txt")
	if err == nil {
		t.Errorf("ComputeSHA256 on missing file = nil, want error")
	}
}
