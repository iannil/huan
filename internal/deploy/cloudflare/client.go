package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/iannil/huan/internal/observability"
)

// DefaultBaseURL is the Cloudflare API root.
const DefaultBaseURL = "https://api.cloudflare.com/client/v4"

// jwtRefreshMargin is how long before expiry we proactively refresh the JWT.
// Per wrangler, JWTs are typically valid for 300s; we refresh 30s early to
// avoid window-edge 401s.
const jwtRefreshMargin = 30 * time.Second

// HTTPClient is the subset of *http.Client this package uses, extracted as an
// interface for test substitution.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client wraps Cloudflare API access. It handles:
//   - Bearer auth (apiToken at account scope, project JWT at assets scope)
//   - JSON envelope unwrapping ({result, success, errors, messages})
//   - Retry with exponential backoff (delegated to retry.go)
//
// Client is safe for concurrent use.
type Client struct {
	accountID string
	apiToken  string
	baseURL   string
	http      HTTPClient
	logger    *observability.Logger

	// JWT cache for assets/* endpoints. Keyed by project name.
	jwtMu    sync.Mutex
	jwtCache map[string]*jwtEntry
}

type jwtEntry struct {
	token  string
	expiry time.Time
}

// NewClient returns a Client configured for the Cloudflare API.
func NewClient(accountID, apiToken string, logger *observability.Logger) *Client {
	return &Client{
		accountID: accountID,
		apiToken:  apiToken,
		baseURL:   DefaultBaseURL,
		http:      &http.Client{Timeout: 60 * time.Second},
		logger:    logger,
		jwtCache:  make(map[string]*jwtEntry),
	}
}

// WithHTTPClient substitutes the default http.Client. Used by tests to inject
// httptest mock clients.
func (c *Client) WithHTTPClient(h HTTPClient) *Client {
	c.http = h
	return c
}

// WithBaseURL substitutes the API base URL. Used by tests.
func (c *Client) WithBaseURL(url string) *Client {
	c.baseURL = url
	return c
}

// APITokenAuth returns the Authorization header value for apiToken requests.
// Exposed so callers (pages.go) can pass it explicitly to GetJSON/PostJSON.
func (c *Client) APITokenAuth() string {
	return "Bearer " + c.apiToken
}

// JWTViaProject returns the Authorization header value for JWT-authenticated
// assets/* requests, fetching a fresh JWT via UploadToken first.
func (c *Client) JWTViaProject(ctx context.Context, project string) (string, error) {
	tok, err := c.UploadToken(ctx, project)
	if err != nil {
		return "", err
	}
	return "Bearer " + tok, nil
}

// UploadToken returns a JWT for the given project. Cached per-project; auto-
// refreshed when within jwtRefreshMargin of expiry.
func (c *Client) UploadToken(ctx context.Context, project string) (string, error) {
	c.jwtMu.Lock()
	defer c.jwtMu.Unlock()

	if entry, ok := c.jwtCache[project]; ok {
		if time.Now().Add(jwtRefreshMargin).Before(entry.expiry) {
			return entry.token, nil
		}
	}

	path := fmt.Sprintf("/accounts/%s/pages/projects/%s/upload-token", c.accountID, project)
	var resp uploadTokenResponse
	if err := c.doJSONNoLock(ctx, http.MethodGet, path, c.APITokenAuth(), nil, &resp); err != nil {
		return "", fmt.Errorf("upload-token: %w", err)
	}

	entry := &jwtEntry{
		token:  resp.JWT,
		expiry: resp.ExpiresOn,
	}
	// If CF didn't return expires_on, assume 5min from now (per wrangler default).
	if resp.ExpiresOn.IsZero() {
		entry.expiry = time.Now().Add(5 * time.Minute)
	}
	c.jwtCache[project] = entry
	return entry.token, nil
}

// InvalidateJWT clears the cached JWT for a project. Call after a 401 on an
// assets/* endpoint to force a refresh on the next UploadToken call.
func (c *Client) InvalidateJWT(project string) {
	c.jwtMu.Lock()
	defer c.jwtMu.Unlock()
	delete(c.jwtCache, project)
}

// cfEnvelope is the standard Cloudflare response wrapper.
type cfEnvelope struct {
	Result   json.RawMessage `json:"result"`
	Success  bool            `json:"success"`
	Errors   []cfAPIError    `json:"errors"`
	Messages []any           `json:"messages"`
}

type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e cfAPIError) String() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// uploadTokenResponse models the GET upload-token response.
type uploadTokenResponse struct {
	JWT       string    `json:"jwt"`
	ExpiresOn time.Time `json:"expires_on"`
}

// GetJSON sends a GET request and decodes the envelope result into out.
// auth is the full Authorization header value (use APITokenAuth() or
// JWTViaProject()).
func (c *Client) GetJSON(ctx context.Context, path, auth string, out any) error {
	return c.doJSONNoLock(ctx, http.MethodGet, path, auth, nil, out)
}

// PostJSON sends a POST request with JSON body and decodes the envelope result.
func (c *Client) PostJSON(ctx context.Context, path, auth string, body, out any) error {
	return c.doJSONNoLock(ctx, http.MethodPost, path, auth, body, out)
}

// PostForm sends multipart/form-data. fields is name -> {contentType, value}.
// Used for POST deployment (manifest + branch + commit_*).
func (c *Client) PostForm(ctx context.Context, path, auth string, fields map[string]formField, out any) error {
	return c.doForm(ctx, http.MethodPost, path, auth, fields, out)
}

// PutForm sends multipart/form-data via PUT. Used for Workers script upload
// (PUT /accounts/{id}/workers/scripts/{name}).
func (c *Client) PutForm(ctx context.Context, path, auth string, fields map[string]formField, out any) error {
	return c.doForm(ctx, http.MethodPut, path, auth, fields, out)
}

// doJSONNoLock is the JSON request implementation. Retry is applied per
// ClassifyError. Separated from a public wrapper so UploadToken can call it
// while holding jwtMu.
func (c *Client) doJSONNoLock(ctx context.Context, method, path, auth string, body, out any) error {
	var lastErr error
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			wait := Backoff(attempt - 1)
			if wait == 0 {
				break
			}
			c.logger.Log("cf-retry", observability.EventPoint, map[string]any{
				"path":    path,
				"attempt": attempt,
				"wait_ms": wait.Milliseconds(),
			})
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		var bodyReader io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				return fmt.Errorf("marshal body: %w", err)
			}
			bodyReader = bytes.NewReader(data)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", auth)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		decision := ClassifyError(resp, err)
		if resp != nil {
			defer resp.Body.Close()
		}

		if !decision.Retryable {
			if err != nil {
				return err
			}
			respBody, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return fmt.Errorf("read response: %w", readErr)
			}
			return decodeEnvelope(respBody, resp.StatusCode, out)
		}

		lastErr = fmt.Errorf("attempt %d: %s", attempt+1, decision.Reason)
		c.logger.Log("cf-retry", observability.EventError, map[string]any{
			"path":    path,
			"attempt": attempt + 1,
			"reason":  decision.Reason,
		})
	}
	return fmt.Errorf("exhausted %d retries: %w", MaxRetries, lastErr)
}

// doForm sends multipart/form-data. No retry — multipart requests are
// deployment-creation which is non-idempotent at the CF side.
func (c *Client) doForm(ctx context.Context, method, path, auth string, fields map[string]formField, out any) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for name, f := range fields {
		header := textproto.MIMEHeader{
			"Content-Type": {f.contentType},
		}
		if f.filename != "" {
			header.Set("Content-Disposition",
				fmt.Sprintf(`form-data; name=%q; filename=%q`, name, f.filename))
		} else {
			header.Set("Content-Disposition",
				fmt.Sprintf(`form-data; name=%q`, name))
		}
		w, err := writer.CreatePart(header)
		if err != nil {
			return fmt.Errorf("create form part %q: %w", name, err)
		}
		if _, err := w.Write(f.value); err != nil {
			return fmt.Errorf("write form part %q: %w", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close form writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &buf)
	if err != nil {
		return fmt.Errorf("build form request: %w", err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("form request: %w", err)
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("read form response: %w", readErr)
	}
	return decodeEnvelope(respBody, resp.StatusCode, out)
}

// formField is one part of a multipart/form-data body. filename is optional;
// when set, the part's Content-Disposition includes filename (required by
// CF Workers script upload).
type formField struct {
	contentType string
	filename    string
	value       []byte
}

// decodeEnvelope parses the CF envelope, checks success, and unmarshals result
// into out. Non-nil error is returned when success=false or status >= 400.
func decodeEnvelope(body []byte, status int, out any) error {
	if status >= 400 && len(body) == 0 {
		return fmt.Errorf("http %d with empty body", status)
	}
	var env cfEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("decode envelope (status %d): %w; body=%s", status, err, truncate(string(body), 200))
	}
	if !env.Success || status >= 400 {
		errMsgs := make([]string, 0, len(env.Errors))
		for _, e := range env.Errors {
			errMsgs = append(errMsgs, e.String())
		}
		return &cfError{
			Status: status,
			Errors: errMsgs,
		}
	}
	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
	}
	return nil
}

// cfError is the structured error returned when CF envelope reports failure.
type cfError struct {
	Status int
	Errors []string
}

func (e *cfError) Error() string {
	return fmt.Sprintf("cloudflare api status %d: %s", e.Status, strings.Join(e.Errors, "; "))
}

// isJWTExpired returns true if err is a cfError whose error messages mention
// "jwt" and ("expir" or "invalid"). Used by pages.go to trigger a JWT refresh
// + single retry.
func isJWTExpired(err error) bool {
	var cfe *cfError
	if !errors.As(err, &cfe) {
		return false
	}
	for _, msg := range cfe.Errors {
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "jwt") && (strings.Contains(lower, "expir") || strings.Contains(lower, "invalid")) {
			return true
		}
	}
	return false
}

// truncate returns s truncated to n runes with ellipsis.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
