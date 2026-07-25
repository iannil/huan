package plugin

import (
	"strings"
	"testing"
)

// stubPlugin is a minimal Plugin implementation for testing.
type stubPlugin struct {
	name string
}

func (s *stubPlugin) Name() string { return s.name }

// capability is a marker interface used to exercise Find[T].
type capability interface {
	CapabilityMarker()
}

type capPlugin struct {
	stubPlugin
}

func (c *capPlugin) CapabilityMarker() {}

func TestNewRegistry_Empty(t *testing.T) {
	r := NewRegistry()
	if got := r.All(); len(got) != 0 {
		t.Errorf("All() = %v, want empty", got)
	}
	if got := r.Names(); len(got) != 0 {
		t.Errorf("Names() = %v, want empty", got)
	}
}

func TestRegister_Success(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubPlugin{name: "alpha"}); err != nil {
		t.Fatalf("Register alpha: %v", err)
	}
	got, ok := r.Get("alpha")
	if !ok {
		t.Fatal("Get(alpha) not found")
	}
	if got.Name() != "alpha" {
		t.Errorf("got.Name() = %q, want alpha", got.Name())
	}
}

func TestRegister_Duplicate(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubPlugin{name: "alpha"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := r.Register(&stubPlugin{name: "alpha"})
	if err == nil {
		t.Fatal("duplicate Register: want error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, want contains 'duplicate'", err.Error())
	}
}

func TestRegister_Nil(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Error("Register(nil): want error, got nil")
	}
}

func TestRegister_EmptyName(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubPlugin{name: ""}); err == nil {
		t.Error("Register(empty name): want error, got nil")
	}
}

func TestAll_PreservesRegistrationOrder(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		if err := r.Register(&stubPlugin{name: name}); err != nil {
			t.Fatalf("Register %s: %v", name, err)
		}
	}
	got := r.Names()
	want := []string{"charlie", "alpha", "bravo"}
	if len(got) != len(want) {
		t.Fatalf("Names() len = %d, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("Names()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestAll_ReturnsCopy(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&stubPlugin{name: "alpha"})
	got := r.All()
	got[0] = nil
	if _, ok := r.Get("alpha"); !ok {
		t.Error("mutating All() slice affected registry")
	}
}

func TestGet_NotFound(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("missing"); ok {
		t.Error("Get(missing) returned ok=true")
	}
}

func TestFind_ReturnsOnlyCapabilityImpls(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&stubPlugin{name: "plain"})
	_ = r.Register(&capPlugin{stubPlugin{name: "cap1"}})
	_ = r.Register(&capPlugin{stubPlugin{name: "cap2"}})

	got := Find[*capPlugin](r)
	if len(got) != 2 {
		t.Fatalf("Find[*capPlugin] len = %d, want 2", len(got))
	}
	if got[0].Name() != "cap1" || got[1].Name() != "cap2" {
		t.Errorf("Find order = %s, %s; want cap1, cap2", got[0].Name(), got[1].Name())
	}
}

func TestFind_NoCapabilityImpls(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&stubPlugin{name: "plain"})
	got := Find[*capPlugin](r)
	if len(got) != 0 {
		t.Errorf("Find len = %d, want 0", len(got))
	}
}

func TestSortedNames_Lexicographic(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		_ = r.Register(&stubPlugin{name: name})
	}
	got := r.SortedNames()
	want := []string{"alpha", "bravo", "charlie"}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("SortedNames()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestUnregister_Success(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubPlugin{name: "alpha"}); err != nil {
		t.Fatalf("Register alpha: %v", err)
	}
	got := r.Unregister("alpha")
	if !got {
		t.Error("Unregister(alpha): want true, got false")
	}
	if _, ok := r.Get("alpha"); ok {
		t.Error("Get(alpha) after Unregister returned ok=true")
	}
}

func TestUnregister_NotFound(t *testing.T) {
	r := NewRegistry()
	got := r.Unregister("nonexistent")
	if got {
		t.Error("Unregister(nonexistent): want false, got true")
	}
}

func TestUnregister_MaintainsOrder(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&stubPlugin{name: "alpha"})
	_ = r.Register(&stubPlugin{name: "bravo"})
	_ = r.Register(&stubPlugin{name: "charlie"})
	r.Unregister("bravo")
	want := []string{"alpha", "charlie"}
	got := r.Names()
	if len(got) != len(want) {
		t.Fatalf("Names() len = %d, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("Names()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

type testMetaPlugin struct {
	stubPlugin
	meta PluginMeta
}

func (p *testMetaPlugin) PluginMetadata() PluginMeta { return p.meta }

var _ MetadataProvider = (*testMetaPlugin)(nil)

func TestMetadataProvider_OptionalInterface(t *testing.T) {
	r := NewRegistry()
	// Plugin without MetadataProvider
	_ = r.Register(&stubPlugin{name: "plain"})
	// Plugin with MetadataProvider
	_ = r.Register(&testMetaPlugin{
		stubPlugin: stubPlugin{name: "metap"},
		meta: PluginMeta{
			Version:    "1.0.0",
			Author:     "test author",
			Tags:       []string{"tag1", "tag2"},
			IsOfficial: true,
		},
	})

	// Test Find[MetadataProvider]
	providers := Find[MetadataProvider](r)
	if len(providers) != 1 {
		t.Fatalf("Find[MetadataProvider] len = %d, want 1", len(providers))
	}
	meta := providers[0].PluginMetadata()
	if meta.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", meta.Version)
	}
	if meta.Author != "test author" {
		t.Errorf("Author = %q", meta.Author)
	}
	if len(meta.Tags) != 2 || meta.Tags[0] != "tag1" {
		t.Errorf("Tags = %v", meta.Tags)
	}
	if !meta.IsOfficial {
		t.Error("IsOfficial should be true")
	}
}

func TestMetadataProvider_NotRequired(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&stubPlugin{name: "plain"})
	providers := Find[MetadataProvider](r)
	if len(providers) != 0 {
		t.Errorf("Find[MetadataProvider] len = %d, want 0", len(providers))
	}
}
