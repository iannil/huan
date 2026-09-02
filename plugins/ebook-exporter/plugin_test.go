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

func TestManifestKeyKindScoped(t *testing.T) {
	// books and practices units sharing the same baseName (e.g. volume-1)
	// must map to distinct manifest keys — otherwise alternate exports of
	// the two kinds thrash (or false-skip via hash coincidence).
	booksKey := manifestKey("books", "volumes", "volume-1", "zh", "epub")
	practicesKey := manifestKey("practices", "volumes", "volume-1", "zh", "epub")
	if booksKey == practicesKey {
		t.Fatalf("cross-kind key collision: %s", booksKey)
	}
	// Level scoping within one kind too.
	if manifestKey("books", "volumes", "volume-1", "zh", "epub") == manifestKey("books", "individual", "volume-1", "zh", "epub") {
		t.Fatal("cross-level key collision")
	}
	// Lang scoping.
	if manifestKey("books", "individual", "rc", "zh", "epub") == manifestKey("books", "individual", "rc", "en", "epub") {
		t.Fatal("cross-lang key collision")
	}
	// Format scoping — a format must never collide with another format of
	// the same unit (the cross-format false-skip regression).
	if manifestKey("books", "individual", "rc", "zh", "epub") == manifestKey("books", "individual", "rc", "zh", "pdf") {
		t.Fatal("cross-format key collision")
	}
}
