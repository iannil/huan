package main

import (
	"context"
	"testing"

	"github.com/iannil/huan/pkg/plugin"
)

func TestInitPluginConfig(t *testing.T) {
	p, err := InitPlugin(map[string]any{
		"output_dir": "developer/export",
		"cover":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ex, ok := p.(plugin.Exporter)
	if !ok {
		t.Fatalf("not an Exporter: %T", p)
	}
	if ex.Name() != "ebook_exporter" {
		t.Fatalf("name: %s", ex.Name())
	}
}

func TestExportBadSourceDir(t *testing.T) {
	p, _ := InitPlugin(nil)
	ex := p.(plugin.Exporter)
	_, err := ex.Export(context.Background(), plugin.ExportRequest{SourceDir: "/nonexistent-xyz"})
	if err == nil {
		t.Fatal("want error for missing source dir")
	}
}
