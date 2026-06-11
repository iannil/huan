// Package pagination implements Hugo-style page pagination.
package pagination

import (
	"fmt"
	"strings"

	"github.com/novel_ttl/huan/internal/content"
)

// Pager represents a single page of paginated results.
type Pager struct {
	PageNumber int
	PageURL    string
	Pages      []*content.Page // pages on THIS pager (slice of the full list)
	TotalPages int
	TotalItems int
	HasPrev    bool
	HasNext    bool
	Prev       *Pager
	Next       *Pager
	First      *Pager
	Last       *Pager
	PagerSize  int
	sectionURL string
}

// URL returns the absolute URL for this pager.
// First pager uses sectionURL, subsequent use sectionURL + "page/N/"
func (p *Pager) URL() string {
	if p.PageNumber == 1 {
		return p.sectionURL
	}
	return strings.TrimRight(p.sectionURL, "/") + fmt.Sprintf("/page/%d/", p.PageNumber)
}

// Paginate splits a list of pages into pagers.
// Returns a slice of pagers; the caller typically uses the first one to start.
func Paginate(pages []*content.Page, pageSize int, sectionURL string) []*Pager {
	if pageSize <= 0 {
		pageSize = 10
	}
	totalPages := (len(pages) + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}

	pagers := make([]*Pager, totalPages)
	for i := 0; i < totalPages; i++ {
		start := i * pageSize
		end := start + pageSize
		if end > len(pages) {
			end = len(pages)
		}
		pagers[i] = &Pager{
			PageNumber: i + 1,
			Pages:      pages[start:end],
			TotalPages: totalPages,
			TotalItems: len(pages),
			PagerSize:  pageSize,
			sectionURL: sectionURL,
		}
	}

	// Link pagers
	for i, p := range pagers {
		if i > 0 {
			p.HasPrev = true
			p.Prev = pagers[i-1]
		}
		if i < len(pagers)-1 {
			p.HasNext = true
			p.Next = pagers[i+1]
		}
		p.First = pagers[0]
		p.Last = pagers[len(pagers)-1]
	}

	return pagers
}

// First returns the first pager (page 1) for use in templates as `.Paginator`.
func First(pagers []*Pager) *Pager {
	if len(pagers) == 0 {
		return nil
	}
	return pagers[0]
}
