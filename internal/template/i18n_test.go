package template

import (
	"testing"
)

// mockI18nBundle implements I18nBundle for testing.
type mockI18nBundle struct{}

func (m *mockI18nBundle) Translate(key string, args ...interface{}) string {
	return "translated:" + key
}

func TestSetI18nBundle(t *testing.T) {
	// Reset to nil first
	SetI18nBundle(nil)
	b := currentI18nBundle()
	if b != nil {
		t.Error("expected nil bundle after reset")
	}

	// Set mock bundle
	mock := &mockI18nBundle{}
	SetI18nBundle(mock)

	b = currentI18nBundle()
	if b == nil {
		t.Fatal("expected non-nil bundle")
	}

	result := b.Translate("hello")
	if result != "translated:hello" {
		t.Errorf("Translate: got %s, want 'translated:hello'", result)
	}

	// Reset for other tests
	SetI18nBundle(nil)
}

func TestCurrentI18nBundle(t *testing.T) {
	SetI18nBundle(nil)
	defer SetI18nBundle(nil)

	b := currentI18nBundle()
	if b != nil {
		t.Error("expected nil bundle")
	}
}

func TestFuncMapWithI18nBindsBundle(t *testing.T) {
	SetI18nBundle(nil)
	defer SetI18nBundle(nil)

	fm := FuncMapWithI18n("https://example.com/", &mockI18nBundle{})
	fn, ok := fm["i18n"].(func(string, ...interface{}) string)
	if !ok {
		t.Fatal("i18n func missing or wrong type")
	}
	if got := fn("hello"); got != "translated:hello" {
		t.Fatalf("bound i18n: got %q", got)
	}
	// Package-level bundle stays nil — the FuncMap must not depend on it.
	if currentI18nBundle() != nil {
		t.Fatal("FuncMapWithI18n should not require SetI18nBundle")
	}
}
