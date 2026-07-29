package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// KrokiClient renders diagram source to SVG via a Kroki HTTP endpoint.
type KrokiClient struct {
	baseURL string
	httpc   *http.Client
}

// NewKrokiClient returns a client for baseURL with the given per-request timeout.
func NewKrokiClient(baseURL string, timeout time.Duration) *KrokiClient {
	return &KrokiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpc:   &http.Client{Timeout: timeout},
	}
}

// Render POSTs source to {baseURL}/{lang}/svg and returns sanitized inline SVG.
func (k *KrokiClient) Render(ctx context.Context, lang, source string) (string, error) {
	url := k.baseURL + "/" + lang + "/svg"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(source))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Accept", "image/svg+xml")

	resp, err := k.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("kroki %s: status %d: %s", lang, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return sanitizeSVG(string(body)), nil
}

var (
	xmlPrologRe = regexp.MustCompile(`(?is)^\s*(<\?xml[^>]*\?>\s*)?(<!DOCTYPE[^>]*>\s*)?`)
	svgOpenRe   = regexp.MustCompile(`(?is)^<svg\b`)
)

// sanitizeSVG strips any XML prolog / DOCTYPE so the SVG can be inlined, and
// ensures the root <svg> element carries class="kroki".
func sanitizeSVG(svg string) string {
	svg = xmlPrologRe.ReplaceAllString(svg, "")
	svg = strings.TrimSpace(svg)
	if svgOpenRe.MatchString(svg) && !strings.Contains(svg[:min(len(svg), 200)], `class="kroki"`) {
		svg = svgOpenRe.ReplaceAllString(svg, `<svg class="kroki"`)
	}
	return svg
}
