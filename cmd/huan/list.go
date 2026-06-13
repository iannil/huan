package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/iannil/huan/internal/content"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List content",
	}
	cmd.AddCommand(
		newListSubCmd("drafts", listDrafts),
		newListSubCmd("future", listFuture),
		newListSubCmd("expired", listExpired),
		newListSubCmd("all", listAll),
	)
	return cmd
}

func newListSubCmd(name string, fn func(pages []*content.Page, now time.Time)) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: "list " + name + " content",
		RunE: func(cmd *cobra.Command, args []string) error {
			contentDir := filepath.Join(sourceDir, "content")
			pages, err := content.LoadDir(contentDir)
			if err != nil {
				return fmt.Errorf("load content: %w", err)
			}
			fn(pages, time.Now())
			return nil
		},
	}
}

func printPage(p *content.Page) {
	fmt.Printf("%s\t%s\t%s\n", p.RelPath, p.Date, p.Title)
}

func listDrafts(pages []*content.Page, _ time.Time) {
	var out []*content.Page
	for _, p := range pages {
		if p.Draft {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RelPath < out[j].RelPath
	})
	for _, p := range out {
		printPage(p)
	}
}

func listFuture(pages []*content.Page, now time.Time) {
	var out []*content.Page
	for _, p := range pages {
		if !p.PublishDateParsed.IsZero() && p.PublishDateParsed.After(now) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PublishDateParsed.Before(out[j].PublishDateParsed)
	})
	for _, p := range out {
		printPage(p)
	}
}

func listExpired(pages []*content.Page, now time.Time) {
	var out []*content.Page
	for _, p := range pages {
		if !p.ExpiryDateParsed.IsZero() && p.ExpiryDateParsed.Before(now) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ExpiryDateParsed.After(out[j].ExpiryDateParsed)
	})
	for _, p := range out {
		printPage(p)
	}
}

func listAll(pages []*content.Page, _ time.Time) {
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].RelPath < pages[j].RelPath
	})
	for _, p := range pages {
		printPage(p)
	}
}
