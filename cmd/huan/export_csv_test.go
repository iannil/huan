package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func csvTestSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	previous := sourceDir
	sourceDir = dir
	t.Cleanup(func() { sourceDir = previous })
	posts := filepath.Join(dir, "content", "posts")
	if err := os.MkdirAll(posts, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"older.md": "---\ndate: '2020-01-01'\n---\nFirst\n\n> *last*, quoted\n",
		"newer.md": "---\ndate: '2021-01-01'\n---\nNew <!--more-->post\n",
	} {
		if err := os.WriteFile(filepath.Join(posts, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestExportCSVDefaults(t *testing.T) {
	for _, args := range [][]string{nil, {"csv"}} {
		t.Run(strings.Join(args, " ")+"default", func(t *testing.T) {
			dir := csvTestSource(t)
			cmd := newExportCmd()
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "developer", "posts-"+time.Now().Format("20060102")+".csv")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := "\xef\xbb\xbf2021-01-01,New post\n2020-01-01,\"Firstlast, quoted\"\n"
			if string(data) != want {
				t.Fatalf("CSV = %q, want %q", data, want)
			}
		})
	}
}

func TestExportCSVCustomDestinationAndLiteralCleanup(t *testing.T) {
	for _, sub := range [][]string{nil, {"csv"}} {
		for _, absolute := range []bool{false, true} {
			t.Run(strings.Join(sub, " ")+map[bool]string{false: "relative", true: "absolute"}[absolute], func(t *testing.T) {
				dir := csvTestSource(t)
				outputArg := "archives[1]"
				output := filepath.Join(dir, outputArg)
				if absolute {
					output = t.TempDir()
					outputArg = output
				}
				if err := os.MkdirAll(output, 0o755); err != nil {
					t.Fatal(err)
				}
				prefix := "notes[*]?"
				old := prefix + "20000101.csv"
				keep := []string{"notesX20000101.csv", "other-20000101.csv", prefix + "notes.csv", prefix + "20000101.txt"}
				for _, name := range append(append([]string{}, keep...), old) {
					if err := os.WriteFile(filepath.Join(output, name), []byte("keep"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				directory := filepath.Join(output, prefix+"20000102.csv")
				if err := os.Mkdir(directory, 0o755); err != nil {
					t.Fatal(err)
				}
				link := filepath.Join(output, prefix+"20000103.csv")
				if err := os.Symlink(filepath.Join(output, keep[0]), link); err != nil {
					t.Fatal(err)
				}
				cmd := newExportCmd()
				cmd.SetArgs(append(append([]string{}, sub...), "--output-dir", outputArg, "--filename-prefix", prefix))
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(output, prefix+time.Now().Format("20060102")+".csv")); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(output, old)); !os.IsNotExist(err) {
					t.Fatalf("old archive remains: %v", err)
				}
				for _, name := range keep {
					if data, err := os.ReadFile(filepath.Join(output, name)); err != nil || string(data) != "keep" {
						t.Fatalf("unrelated file %q changed: %q, %v", name, data, err)
					}
				}
				for _, path := range []string{directory, link} {
					if _, err := os.Lstat(path); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestExportCSVRejectsUnsafePrefix(t *testing.T) {
	for _, prefix := range []string{"", "../outside-", "nested/name", `nested\name`} {
		t.Run(prefix, func(t *testing.T) {
			dir := csvTestSource(t)
			cmd := newExportCmd()
			cmd.SetArgs([]string{"--filename-prefix", prefix})
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "filename-prefix") {
				t.Fatalf("want prefix validation error, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "developer")); !os.IsNotExist(err) {
				t.Fatalf("invalid prefix created output directory: %v", err)
			}
		})
	}
}

func TestExportCSVFlagsNotAcceptedByEbook(t *testing.T) {
	for _, flag := range []string{"--output-dir", "--filename-prefix"} {
		cmd := newExportCmd()
		cmd.SetArgs([]string{"ebook", flag, "unused"})
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("ebook accepted CSV flag %s: %v", flag, err)
		}
	}
}
