package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestCallPluginAdminAPI_TokenNotSet(t *testing.T) {
	orig := os.Getenv("HUAN_ADMIN_TOKEN")
	os.Unsetenv("HUAN_ADMIN_TOKEN")
	defer func() {
		if orig != "" {
			os.Setenv("HUAN_ADMIN_TOKEN", orig)
		}
	}()

	err := callPluginAdminAPI("GET", "/admin/api/plugins", nil)
	if err == nil {
		t.Fatal("expected error when HUAN_ADMIN_TOKEN not set")
	}
	if !strings.Contains(err.Error(), "HUAN_ADMIN_TOKEN not set") {
		t.Errorf("err = %q, want contains 'HUAN_ADMIN_TOKEN not set'", err.Error())
	}
}

func TestListRuntimePlugins_TokenNotSet(t *testing.T) {
	orig := os.Getenv("HUAN_ADMIN_TOKEN")
	os.Unsetenv("HUAN_ADMIN_TOKEN")
	defer func() {
		if orig != "" {
			os.Setenv("HUAN_ADMIN_TOKEN", orig)
		}
	}()

	err := listRuntimePlugins()
	if err == nil {
		t.Fatal("expected error when HUAN_ADMIN_TOKEN not set")
	}
	if !strings.Contains(err.Error(), "HUAN_ADMIN_TOKEN not set") {
		t.Errorf("err = %q, want contains 'HUAN_ADMIN_TOKEN not set'", err.Error())
	}
}

func TestNewLocalPluginLoader_DefaultDir(t *testing.T) {
	orig := os.Getenv("HUAN_PLUGIN_DIR")
	os.Unsetenv("HUAN_PLUGIN_DIR")
	defer func() {
		if orig != "" {
			os.Setenv("HUAN_PLUGIN_DIR", orig)
		}
	}()

	loader := newLocalPluginLoader()
	if loader == nil {
		t.Fatal("expected loader, got nil")
	}
}

func TestNewLocalPluginLoader_CustomDir(t *testing.T) {
	orig := os.Getenv("HUAN_PLUGIN_DIR")
	os.Setenv("HUAN_PLUGIN_DIR", "/custom/plugins")
	defer func() {
		if orig != "" {
			os.Setenv("HUAN_PLUGIN_DIR", orig)
		} else {
			os.Unsetenv("HUAN_PLUGIN_DIR")
		}
	}()

	loader := newLocalPluginLoader()
	if loader == nil {
		t.Fatal("expected loader, got nil")
	}
}

func TestNewPluginLoadCmd_Args(t *testing.T) {
	cmd := newPluginLoadCmd()
	if cmd == nil {
		t.Fatal("expected command, got nil")
	}
	if cmd.Use != "load <path>" {
		t.Errorf("Use = %q, want 'load <path>'", cmd.Use)
	}
	if !strings.Contains(cmd.Short, "Load") {
		t.Errorf("Short = %q, want contains 'Load'", cmd.Short)
	}
}

func TestNewPluginUnloadCmd_Args(t *testing.T) {
	cmd := newPluginUnloadCmd()
	if cmd == nil {
		t.Fatal("expected command, got nil")
	}
	if cmd.Use != "unload <name>" {
		t.Errorf("Use = %q, want 'unload <name>'", cmd.Use)
	}
	if !strings.Contains(cmd.Short, "Unload") {
		t.Errorf("Short = %q, want contains 'Unload'", cmd.Short)
	}
}

func TestNewPluginReloadCmd_Args(t *testing.T) {
	cmd := newPluginReloadCmd()
	if cmd == nil {
		t.Fatal("expected command, got nil")
	}
	if cmd.Use != "reload <name> <path>" {
		t.Errorf("Use = %q, want 'reload <name> <path>'", cmd.Use)
	}
	if !strings.Contains(cmd.Short, "Hot-reload") {
		t.Errorf("Short = %q, want contains 'Hot-reload'", cmd.Short)
	}
}

func TestNewPluginListCmd_AllFlag(t *testing.T) {
	cmd := newPluginListCmd()
	if cmd == nil {
		t.Fatal("expected command, got nil")
	}

	// Check that --all flag exists
	flag := cmd.Flags().Lookup("all")
	if flag == nil {
		t.Fatal("expected --all flag, got nil")
	}
	if flag.Shorthand != "a" {
		t.Errorf("flag shorthand = %q, want 'a'", flag.Shorthand)
	}
}

func TestNewPluginCmd_HasSubcommands(t *testing.T) {
	cmd := newPluginCmd()
	if cmd == nil {
		t.Fatal("expected command, got nil")
	}

	subcmds := cmd.Commands()
	if len(subcmds) < 5 {
		t.Errorf("expected at least 5 subcommands, got %d", len(subcmds))
	}

	names := make(map[string]bool)
	for _, c := range subcmds {
		// Extract first word from Use (e.g., "list" from "list")
		name := strings.Split(c.Use, " ")[0]
		names[name] = true
	}

	expected := []string{"list", "info", "load", "unload", "reload"}
	for _, e := range expected {
		if !names[e] {
			t.Errorf("missing subcommand: %s", e)
		}
	}
}

// captureOutput redirects stdout to capture command output.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string)
	go func() {
		buf := new(bytes.Buffer)
		_, _ = io.Copy(buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}