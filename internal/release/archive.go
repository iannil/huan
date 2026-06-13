package release

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"time"
)

// ArchiveMember is a single file to pack into the archive.
type ArchiveMember struct {
	Name string // path inside the archive (e.g. "huan", "LICENSE")
	Path string // source path on disk
	Mode os.FileMode
}

// zeroTime is the Unix epoch, used for all archive headers to guarantee
// deterministic output: two runs of CreateTarGZ/CreateZip on the same
// inputs produce byte-identical archives.
var zeroTime = time.Unix(0, 0).UTC()

// CreateTarGZ writes members as a flat (no wrapping dir) gzipped tar to
// outFile. The output is deterministic: no directory entries, no mtime in
// tar headers (zero time used so two runs produce identical bytes).
//
// Per ADR 0004 §6: archive contents are flat — binary, LICENSE, READMEs at
// the tarball root. This matches Hugo/Caddy convention so users can
// `tar xzf huan_*.tar.gz -C ~/bin` without an intermediate dir.
func CreateTarGZ(outFile string, members []ArchiveMember) error {
	f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", outFile, err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	for _, m := range members {
		if err := appendFileToTar(tw, m); err != nil {
			return err
		}
	}
	return nil
}

func appendFileToTar(tw *tar.Writer, m ArchiveMember) error {
	info, err := os.Stat(m.Path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", m.Path, err)
	}
	header := &tar.Header{
		Name:     m.Name,
		Mode:     int64(m.Mode.Perm()),
		Size:     info.Size(),
		ModTime:  zeroTime,
		Typeflag: tar.TypeReg,
		Format:   tar.FormatGNU,
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("tar header %s: %w", m.Name, err)
	}
	src, err := os.Open(m.Path)
	if err != nil {
		return fmt.Errorf("open %s: %w", m.Path, err)
	}
	defer src.Close()
	if _, err := io.Copy(tw, src); err != nil {
		return fmt.Errorf("tar copy %s: %w", m.Name, err)
	}
	return nil
}

// CreateZip writes members as a flat zip archive. Zip format is used for
// Windows targets (no permission info preserved; Windows doesn't need
// execute bit on .exe files).
//
// Like CreateTarGZ, the output is deterministic: modification time is zeroed
// so two runs produce identical bytes.
func CreateZip(outFile string, members []ArchiveMember) error {
	f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", outFile, err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

	for _, m := range members {
		if err := appendFileToZip(zw, m); err != nil {
			return err
		}
	}
	return nil
}

func appendFileToZip(zw *zip.Writer, m ArchiveMember) error {
	src, err := os.Open(m.Path)
	if err != nil {
		return fmt.Errorf("open %s: %w", m.Path, err)
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", m.Path, err)
	}
	h, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("zip header %s: %w", m.Name, err)
	}
	h.Name = m.Name
	h.Method = zip.Deflate
	h.Modified = zeroTime
	w, err := zw.CreateHeader(h)
	if err != nil {
		return fmt.Errorf("zip create %s: %w", m.Name, err)
	}
	if _, err := io.Copy(w, src); err != nil {
		return fmt.Errorf("zip copy %s: %w", m.Name, err)
	}
	return nil
}

// CreateArchive dispatches to CreateTarGZ or CreateZip based on target OS.
func CreateArchive(outFile string, target Target, members []ArchiveMember) error {
	if target.OS == "windows" {
		return CreateZip(outFile, members)
	}
	return CreateTarGZ(outFile, members)
}
