package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/iannil/huan/internal/plugin"
)

// callPluginAdminAPI sends a request to the daemon's admin API.
// Returns nil on success, error on failure (daemon not running, etc.).
func callPluginAdminAPI(method, endpoint string, body interface{}) error {
	token := os.Getenv("HUAN_ADMIN_TOKEN")
	if token == "" {
		return fmt.Errorf("HUAN_ADMIN_TOKEN not set")
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	url := fmt.Sprintf("http://127.0.0.1:8080%s", endpoint)
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Huan-Admin-Token", token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("admin API request failed (is daemon running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("admin API: %s", string(respBody))
	}
	return nil
}

// listRuntimePlugins queries the daemon for runtime-loaded plugins and prints them.
func listRuntimePlugins() error {
	token := os.Getenv("HUAN_ADMIN_TOKEN")
	if token == "" {
		return fmt.Errorf("HUAN_ADMIN_TOKEN not set")
	}

	url := "http://127.0.0.1:8080/admin/api/plugins"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-Huan-Admin-Token", token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("admin API request failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	plugins, ok := result["plugins"].([]interface{})
	if !ok || len(plugins) == 0 {
		fmt.Println("\nNo runtime-loaded plugins.")
		return nil
	}

	fmt.Println("\nRuntime plugins:")
	for _, p := range plugins {
		if info, ok := p.(map[string]interface{}); ok {
			fmt.Printf("  - %s (source: %s, status: %s)\n",
				info["name"], info["source"], info["status"])
		}
	}
	return nil
}

// newLocalPluginLoader creates a Loader pointed at the plugin directory
// configured in huan.yaml, or a fallback default.
func newLocalPluginLoader() *plugin.Loader {
	dir := os.Getenv("HUAN_PLUGIN_DIR")
	if dir == "" {
		dir = "plugins"
	}
	return plugin.NewLoader(dir)
}