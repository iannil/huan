package build

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/content"
)

func TestStaleSnapshotMatchesDisk(t *testing.T) {
	dir := t.TempDir()
	source := " \n---\ntitle: Source\n---\nOriginal body. \n"
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
	fixtures := map[string]string{
		"posts/a.md":       source,
		"posts/a.en.md":    "---\nsource_hash: '" + hash + "'\n---\nEnglish",
		"posts/a.fr.md":    "---\nsource_hash: stale\n---\nFrench",
		"posts/a.zh-cn.md": "---\nsource_hash: \"" + strings.ToUpper(hash) + "\"\n---\nChinese",
		"posts/a.de.md":    "---\nsource_hash: 123\n---\nGerman",
		"posts/a.it.md":    "---\nsource_hash: [wrong]\n---\nItalian",
		"posts/a.pt.md":    "---\nsource_hash: ''\n---\nPortuguese",
		"posts/a.es.md":    "No frontmatter",
		"posts/a.ja.md":    "---\r\nsource_hash: stale\r\n---\r\nJapanese",
		"posts/b.md":       "---\ntitle: Section\n---\n \t",
		"posts/b.en.md":    "Section",
		"posts/c.md":       " \n\t ", "posts/c.en.md": "Empty source",
		"posts/missing.en.md": "Orphan",
		"posts/z.md":          "Body", "posts/z.en.md": "Missing hash",
	}
	for path, data := range fixtures {
		writeFile(t, dir, path, data)
	}
	want, err := checkStaleTranslations(dir)
	if err != nil {
		t.Fatal(err)
	}
	expected := &I18nStaleReport{Checked: 4, Stale: 3, Missing: 5,
		StaleFiles:       []string{"posts/a.de.md", "posts/a.fr.md", "posts/a.zh-cn.md"},
		MissingHashFiles: []string{"posts/a.es.md", "posts/a.it.md", "posts/a.ja.md", "posts/a.pt.md", "posts/z.en.md"}}
	if !reflect.DeepEqual(want, expected) {
		t.Fatalf("disk report=%+v, want=%+v", want, expected)
	}
	snapshot := newStaleTranslationSnapshot()
	if _, err := content.LoadDirWithObserver(dir, snapshot.observe); err != nil {
		t.Fatal(err)
	}
	// The report must depend only on captured bytes, even if disk content changes.
	if err := os.WriteFile(filepath.Join(dir, "posts/a.md"), []byte("Changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := snapshot.report(dir); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot=%+v disk=%+v", got, want)
	}
}

func BenchmarkStaleContentLoad(b *testing.B) {
	dir := b.TempDir()
	source := "---\ntitle: Source\n---\n" + strings.Repeat("Source markdown body.\n", 100)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
	sidecar := "---\ntitle: Translation\nsource_hash: '" + hash + "'\n---\n" + strings.Repeat("Translated markdown body.\n", 100)
	for i := 0; i < 300; i++ {
		for _, lang := range []string{"", ".en", ".fr", ".ja"} {
			data := sidecar
			if lang == "" {
				data = source
			}
			path := filepath.Join(dir, fmt.Sprintf("page-%04d%s.md", i, lang))
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.Run("disk_check_then_load", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := checkStaleTranslations(dir); err != nil {
				b.Fatal(err)
			}
			if _, err := content.LoadDir(dir); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("shared_read_snapshot", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			snapshot := newStaleTranslationSnapshot()
			if _, err := content.LoadDirWithObserver(dir, snapshot.observe); err != nil {
				b.Fatal(err)
			}
			if report := snapshot.report(dir); report.Checked != 900 || report.Stale != 0 || report.Missing != 0 {
				b.Fatalf("unexpected report: %+v", report)
			}
		}
	})
}
