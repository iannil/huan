package main

import "testing"

// TestInitPluginSetsLogf guards the .so entrypoint: InitPlugin must return a
// fully-initialized plugin whose logf is non-nil. The host loads plugins via
// InitPlugin (not New); a nil logf panics whenever a logf call is reached.
func TestInitPluginSetsLogf(t *testing.T) {
	p, err := InitPlugin(nil)
	if err != nil {
		t.Fatalf("InitPlugin: %v", err)
	}
	inj, ok := p.(*SEOInjector)
	if !ok {
		t.Fatalf("InitPlugin returned %T, want *SEOInjector", p)
	}
	if inj.logf == nil {
		t.Fatal("InitPlugin left logf nil — logf calls will panic on the .so path")
	}
}
