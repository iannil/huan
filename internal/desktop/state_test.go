package desktop

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestTransition_ValidAndInvalid(t *testing.T) {
	m := NewMachine()
	if m.Current() != StateIdle {
		t.Fatalf("initial state = %v, want idle", m.Current())
	}
	if !m.Transition(StateIdle, StateServing) {
		t.Fatal("idle→serving should succeed")
	}
	if m.Transition(StateIdle, StateBuilding) {
		t.Fatal("serving→building must fail when from=idle")
	}
	if !m.Transition(StateServing, StateBuilding) {
		t.Fatal("serving→building should succeed")
	}
	if !m.Transition(StateBuilding, StateServing) {
		t.Fatal("building→serving should succeed")
	}
	if !m.Transition(StateServing, StateIdle) {
		t.Fatal("serving→idle should succeed")
	}
	if !m.Transition(StateIdle, StateError) {
		t.Fatal("idle→error should succeed")
	}
	if !m.Transition(StateError, StateIdle) {
		t.Fatal("error→idle (reset) should succeed")
	}
}

func TestTransition_ConcurrentOnlyOneWinner(t *testing.T) {
	m := NewMachine()
	var wg sync.WaitGroup
	wins := 0
	var mu sync.Mutex
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if m.Transition(StateIdle, StateServing) {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Errorf("expected exactly 1 winner, got %d", wins)
	}
	if m.Current() != StateServing {
		t.Errorf("state = %v, want serving", m.Current())
	}
}

func TestStateString(t *testing.T) {
	cases := map[State]string{
		StateIdle: "idle", StateServing: "serving",
		StateBuilding: "building", StateError: "error",
		State(99): "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", s, got, want)
		}
	}
}

func TestValidateSite(t *testing.T) {
	if err := ValidateSite(t.TempDir()); err == nil {
		t.Fatal("empty dir should fail validation")
	}
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "huan.yaml"),
		[]byte("baseURL: https://example.com/\ntitle: T\nlanguageCode: en\n"), 0o644)
	if err := ValidateSite(src); err != nil {
		t.Fatalf("valid site rejected: %v", err)
	}
}
