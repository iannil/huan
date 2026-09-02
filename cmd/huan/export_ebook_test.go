package main

import (
	"context"
	"testing"

	pkgplugin "github.com/iannil/huan/pkg/plugin"
)

// stubExporterCmd satisfies pkg/plugin.Exporter for CLI wiring tests.
type stubExporterCmd struct{}

func (stubExporterCmd) Name() string { return "stub_exporter" }

func (stubExporterCmd) Export(ctx context.Context, req pkgplugin.ExportRequest) (pkgplugin.ExportResult, error) {
	return pkgplugin.ExportResult{}, nil
}

func TestExportCmdHasSubcommands(t *testing.T) {
	root := newExportCmd()
	subs := root.Commands()
	names := map[string]bool{}
	for _, c := range subs {
		names[c.Name()] = true
	}
	if !names["csv"] || !names["ebook"] {
		t.Fatalf("want csv+ebook subcommands, got %v", names)
	}
	// bare `huan export` keeps CSV behavior: RunE non-nil, no subcommand required
	if root.RunE == nil {
		t.Fatal("bare export must keep RunE (CSV back-compat)")
	}
}

func TestExportEbookCmdFlags(t *testing.T) {
	cmd := newExportEbookCmd()
	for _, name := range []string{"type", "format", "level", "slug", "volume", "force", "jobs"} {
		if err := cmd.Flags().Lookup(name); err == nil {
			t.Fatalf("ebook flag %q missing", name)
		}
	}
	if def, _ := cmd.Flags().GetString("type"); def != "all" {
		t.Fatalf("type default = %q, want all", def)
	}
	if def, _ := cmd.Flags().GetString("format"); def != "all" {
		t.Fatalf("format default = %q, want all", def)
	}
	if def, _ := cmd.Flags().GetString("level"); def != "all" {
		t.Fatalf("level default = %q, want all", def)
	}
	if def, _ := cmd.Flags().GetInt("jobs"); def != 0 {
		t.Fatalf("jobs default = %d, want 0", def)
	}
}

func TestNormalizeExportLevel(t *testing.T) {
	if got := normalizeExportLevel("seasons"); got != "volumes" {
		t.Fatalf("normalizeExportLevel(seasons) = %q, want volumes", got)
	}
	if got := normalizeExportLevel("individual"); got != "individual" {
		t.Fatalf("normalizeExportLevel(individual) = %q", got)
	}
}
