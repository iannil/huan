package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/iannil/huan/internal/config"
)

// State enumerates the desktop site runtime states.
type State int32

const (
	StateIdle     State = iota // no server running
	StateServing               // dev server running
	StateBuilding              // production build in progress
	StateError                 // last operation failed; recover via idle
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateServing:
		return "serving"
	case StateBuilding:
		return "building"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// Machine is a tiny CAS-based state machine; concurrent transitions are
// idempotent — exactly one caller wins.
type Machine struct {
	cur atomic.Int32
}

// NewMachine returns a machine in StateIdle.
func NewMachine() *Machine {
	m := &Machine{}
	m.cur.Store(int32(StateIdle))
	return m
}

// Current reports the current state.
func (m *Machine) Current() State { return State(m.cur.Load()) }

// Transition moves from → to atomically; reports whether it won.
func (m *Machine) Transition(from, to State) bool {
	return m.cur.CompareAndSwap(int32(from), int32(to))
}

// ValidateSite checks that sourceDir is a usable huan site: huan.yaml exists
// and config loads. The explicit file check gives a clearer message than a
// bare config error when the user picked a non-site directory.
func ValidateSite(sourceDir string) error {
	if _, err := os.Stat(filepath.Join(sourceDir, "huan.yaml")); err != nil {
		return fmt.Errorf("%s is not a huan site: huan.yaml not found", sourceDir)
	}
	if _, err := config.Load(sourceDir); err != nil {
		return fmt.Errorf("load site config: %w", err)
	}
	return nil
}
