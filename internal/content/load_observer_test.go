package content

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadDirObserverRawBytesAndParseFailure(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{"a.en.md": " \n---\ntitle: English\n---\n Body \n", "b.md": "---\ntitle: [\n---", "c.md": "Later", "a.txt": "Ignored"}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var observed []string
	pages, err := LoadDirWithObserver(dir, func(path string, data []byte) {
		name := filepath.Base(path)
		observed = append(observed, name)
		if string(data) != files[name] {
			t.Errorf("observer lost raw bytes for %s", name)
		}
	})
	if err == nil || len(pages) != 1 || pages[0].Language != "en" || pages[0].RawContent != "Body" {
		t.Fatalf("pages=%+v err=%v", pages, err)
	}
	if !reflect.DeepEqual(observed, []string{"a.en.md", "b.md"}) {
		t.Fatalf("observed=%v", observed)
	}
}
