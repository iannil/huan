package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// imageExtensions are file extensions treated as gallery images.
var imageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".avif": true,
	".bmp":  true,
	".svg":  true,
}

func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Scaffold content from existing assets",
	}
	cmd.AddCommand(newSyncGalleryCmd())
	return cmd
}

func newSyncGalleryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gallery",
		Short: "Generate content/gallery/*.md for new images in static/images/gallery/",
		Long: `Scan static/images/gallery/ for image files. For each image that does
not yet have a corresponding content/gallery/<name>.md, scaffold one with
gallery frontmatter. Existing markdown files are left untouched.

R2 upload is separate — run "huan deploy cloudflare r2" after this to push
the images themselves.`,
		Args: cobra.NoArgs,
		RunE: runSyncGallery,
	}
}

func runSyncGallery(cmd *cobra.Command, args []string) error {
	galleryDir := filepath.Join(sourceDir, "static", "images", "gallery")
	contentDir := filepath.Join(sourceDir, "content", "gallery")

	if _, err := os.Stat(galleryDir); os.IsNotExist(err) {
		return fmt.Errorf("gallery directory does not exist: %s", galleryDir)
	}

	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		return fmt.Errorf("create content dir: %w", err)
	}

	total, generated, skipped, err := syncGalleryDir(galleryDir, contentDir)
	if err != nil {
		return err
	}

	fmt.Printf("Gallery sync: %d images, %d new markdown, %d existing\n", total, generated, skipped)
	return nil
}

// syncGalleryDir walks galleryDir for image files and ensures each has a
// matching markdown file in contentDir. Returns counts.
func syncGalleryDir(galleryDir, contentDir string) (total, generated, skipped int, err error) {
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		return 0, 0, 0, fmt.Errorf("create content dir: %w", err)
	}

	walkErr := filepath.Walk(galleryDir, func(imgPath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		name := info.Name()
		if strings.HasPrefix(name, ".") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		if !imageExtensions[ext] {
			return nil
		}

		total++

		stem := strings.TrimSuffix(name, filepath.Ext(name))
		relImg, err := filepath.Rel(galleryDir, imgPath)
		if err != nil {
			return err
		}
		relImg = filepath.ToSlash(relImg)
		mdPath := filepath.Join(contentDir, stem+".md")

		if _, err := os.Stat(mdPath); err == nil {
			skipped++
			return nil
		}

		if err := writeGalleryMarkdown(mdPath, name, relImg); err != nil {
			return fmt.Errorf("write %s: %w", mdPath, err)
		}
		fmt.Printf("  ✓ %s.md\n", stem)
		generated++
		return nil
	})
	return total, generated, skipped, walkErr
}

func writeGalleryMarkdown(mdPath, imgName, relImg string) error {
	stem := strings.TrimSuffix(imgName, filepath.Ext(imgName))
	now := time.Now().Format(time.RFC3339)
	featured := "/images/gallery/" + relImg

	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "title: %q\n", stem)
	fmt.Fprintf(&sb, "date: %s\n", now)
	sb.WriteString("draft: false\n")
	sb.WriteString("type: gallery\n")
	fmt.Fprintf(&sb, "featured_image: %q\n", featured)
	sb.WriteString(`description: ""` + "\n")
	sb.WriteString("tags: []\n")
	sb.WriteString("---\n")

	return os.WriteFile(mdPath, []byte(sb.String()), 0o644)
}
