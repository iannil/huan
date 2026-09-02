package plugin

import (
	"context"
	"testing"
)

// base is a minimal stub satisfying Plugin, embedded by capability stubs.
type base struct{ name string }

func (b base) Name() string { return b.name }

// stubExporter verifies the Exporter contract is satisfiable and that
// plugin.Find discovers it by capability.
type stubExporter struct{ base }

func (stubExporter) Export(ctx context.Context, req ExportRequest) (ExportResult, error) {
	return ExportResult{Succeeded: []ExportItem{{Path: "out.epub"}}}, nil
}

func TestExporterInterfaceSatisfied(t *testing.T) {
	var _ Exporter = stubExporter{}
}

func TestFindExporter(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stubExporter{base{"ebook_exporter"}}); err != nil {
		t.Fatal(err)
	}
	got := Find[Exporter](r)
	if len(got) != 1 {
		t.Fatalf("want 1 exporter, got %d", len(got))
	}
	res, err := got[0].Export(context.Background(), ExportRequest{Type: "books"})
	if err != nil || len(res.Succeeded) != 1 {
		t.Fatalf("unexpected: %v %+v", err, res)
	}
}
