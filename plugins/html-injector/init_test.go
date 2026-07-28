package main

import "testing"

// TestInitPluginSetsLogf guards the .so entrypoint: InitPlugin must return a
// fully-initialized plugin whose logf is non-nil. The host loads plugins via
// InitPlugin (not New), and OnOutputWritten calls p.logf unconditionally — a
// nil logf panics and crashes the whole build.
func TestInitPluginSetsLogf(t *testing.T) {
	p, err := InitPlugin(nil)
	if err != nil {
		t.Fatalf("InitPlugin: %v", err)
	}
	inj, ok := p.(*HTMLInjector)
	if !ok {
		t.Fatalf("InitPlugin returned %T, want *HTMLInjector", p)
	}
	if inj.logf == nil {
		t.Fatal("InitPlugin left logf nil — OnOutputWritten will panic on the .so path")
	}
}
