package admin

// ContentItem is the API response for a single content file.
type ContentItem struct {
	Title       string   `json:"title"`
	RelPath     string   `json:"relPath"`
	Section     string   `json:"section"`
	Kind        string   `json:"kind"`
	Draft       bool     `json:"draft"`
	Hidden      bool     `json:"hidden"`
	Date        string   `json:"date"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
	Slug        string   `json:"slug"`
	Language    string   `json:"language"`
	URL         string   `json:"url"`
}

// ContentDetail is the API response for reading a single file (full detail).
type ContentDetail struct {
	ContentItem
	RawContent   string                 `json:"rawContent"`
	Frontmatter  map[string]interface{} `json:"frontmatter"`
}

// ContentListResponse wraps the content listing.
type ContentListResponse struct {
	Sections map[string][]ContentItem `json:"sections"`
	Total    int                      `json:"total"`
}

// StatusResponse holds site overview stats for the dashboard.
type StatusResponse struct {
	Title            string            `json:"title"`
	BaseURL          string            `json:"baseURL"`
	ServeURL         string            `json:"serveURL"`
	Total            int               `json:"total"`
	Drafts           int               `json:"drafts"`
	Sections         int               `json:"sections"`
	Languages        []string          `json:"languages"`
	SectionBreakdown map[string]int    `json:"sectionBreakdown"`
}

// CreateContentRequest is the API body for creating a new content file.
type CreateContentRequest struct {
	Section    string `json:"section"`
	Filename   string `json:"filename"` // e.g. "my-post" (without .md)
	Title      string `json:"title"`
	Draft      bool   `json:"draft"`
}

// UpdateContentRequest is the API body for updating a content file.
type UpdateContentRequest struct {
	Frontmatter map[string]interface{} `json:"frontmatter"`
	RawContent  string                 `json:"rawContent"`
}

// APIError represents a JSON error response.
type APIError struct {
	Error string `json:"error"`
}
