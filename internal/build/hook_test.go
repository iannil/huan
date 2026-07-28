package build

import (
	"context"
	"testing"

	"github.com/iannil/huan/internal/content"
	iplugin "github.com/iannil/huan/internal/plugin"
	pkgplugin "github.com/iannil/huan/pkg/plugin"
)

// postBuildStub satisfies pkgplugin.PostBuildHook (the .so-facing hook) but NOT
// internal/build.Hook — mirroring the shipped seo/sitemap/html .so plugins.
type postBuildStub struct{ called bool }

func (s *postBuildStub) Name() string { return "post_build_stub" }
func (s *postBuildStub) OnOutputWritten(_ context.Context, _ string) error {
	s.called = true
	return nil
}

var _ pkgplugin.PostBuildHook = (*postBuildStub)(nil)

func TestRunOnOutputWritten_InvokesPostBuildHook(t *testing.T) {
	stub := &postBuildStub{}
	reg := iplugin.NewRegistry()
	if err := reg.Register(stub); err != nil {
		t.Fatalf("register: %v", err)
	}
	p := newPipeline(Options{OutputDir: t.TempDir(), PluginRegistry: reg, Logf: func(string, ...any) {}})
	p.runOnOutputWritten()
	if !stub.called {
		t.Fatal("PostBuildHook.OnOutputWritten was not invoked by the pipeline")
	}
}

// pageHookStub satisfies internal/build.Hook (compiled-in rich hook). Guards
// against the PostBuildHook bridge regressing the existing build.Hook path.
type pageHookStub struct{ outCalled bool }

func (s *pageHookStub) Name() string { return "page_hook_stub" }
func (s *pageHookStub) OnContentLoaded(_ context.Context, _ []*content.Page) ([]*content.Page, error) {
	return nil, nil
}
func (s *pageHookStub) OnPageRendered(_ context.Context, _ *content.Page) error { return nil }
func (s *pageHookStub) OnOutputWritten(_ context.Context, _ string) error {
	s.outCalled = true
	return nil
}

var _ Hook = (*pageHookStub)(nil)

func TestRunOnOutputWritten_InvokesBuildHook(t *testing.T) {
	stub := &pageHookStub{}
	reg := iplugin.NewRegistry()
	if err := reg.Register(stub); err != nil {
		t.Fatalf("register: %v", err)
	}
	p := newPipeline(Options{OutputDir: t.TempDir(), PluginRegistry: reg, Logf: func(string, ...any) {}})
	p.runOnOutputWritten()
	if !stub.outCalled {
		t.Fatal("build.Hook.OnOutputWritten was not invoked")
	}
}
