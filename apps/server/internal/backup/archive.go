package backup

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const ManifestName = "manifest.json"

// Manifest is stored at the root of every backup ZIP.
type Manifest struct {
	FormatVersion     int       `json:"formatVersion"`
	Kind              Kind      `json:"kind"`
	CreatedAt         time.Time `json:"createdAt"`
	SchemaHash        string    `json:"schemaHash"`
	DatabaseVersion   string    `json:"databaseVersion"`
	Tables            []string  `json:"tables"`
	Users             []int64   `json:"users,omitempty"`
	FileCount         int       `json:"fileCount"`
	UncompressedBytes int64     `json:"uncompressedBytes"`
}

// Writer creates a backup ZIP and guards entry paths and counts.
type Writer struct {
	file         *os.File
	zip          *zip.Writer
	maxEntries   int
	entries      int
	uncompressed int64
}

func NewWriter(filename string, maxEntries int) (*Writer, error) {
	if maxEntries <= 0 {
		maxEntries = 200_000
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Writer{file: f, zip: zip.NewWriter(f), maxEntries: maxEntries}, nil
}

// Create creates one entry. Counted bytes feed the manifest's uncompressed size.
func (w *Writer) Create(name string) (io.Writer, error) {
	if !validArchiveName(name) {
		return nil, fmt.Errorf("invalid backup entry %q", name)
	}
	if w.entries >= w.maxEntries {
		return nil, errors.New("backup entry limit exceeded")
	}
	if name == ManifestName {
		return nil, errors.New("manifest is reserved")
	}
	entry, err := w.zip.Create(name)
	if err != nil {
		return nil, err
	}
	w.entries++
	return &countingWriter{w: entry, total: &w.uncompressed}, nil
}

// Close writes the manifest and finishes the ZIP.
func (w *Writer) Close(manifest Manifest) error {
	manifest.FormatVersion = 1
	manifest.FileCount = w.entries
	manifest.UncompressedBytes = w.uncompressed
	entry, err := w.zip.Create(ManifestName)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(entry).Encode(manifest); err != nil {
		return err
	}
	if err := w.zip.Close(); err != nil {
		return err
	}
	return w.file.Close()
}

// Abort closes and removes a partially written file.
func (w *Writer) Abort() {
	_ = w.zip.Close()
	name := w.file.Name()
	_ = w.file.Close()
	_ = os.Remove(name)
}

type countingWriter struct {
	w     io.Writer
	total *int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	*c.total += int64(n)
	return n, err
}

// Reader validates and reads a backup ZIP.
type Reader struct {
	zip      *zip.ReadCloser
	files    map[string]*zip.File
	Manifest Manifest
}

func OpenReader(filename string, maxEntries int, maxUncompressed int64) (*Reader, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	if maxEntries <= 0 {
		maxEntries = 200_000
	}
	if len(zr.File) > maxEntries {
		_ = zr.Close()
		return nil, errors.New("backup has too many entries")
	}
	files := make(map[string]*zip.File, len(zr.File))
	var total uint64
	for _, f := range zr.File {
		if !validArchiveName(f.Name) || strings.HasSuffix(f.Name, "/") {
			_ = zr.Close()
			return nil, fmt.Errorf("invalid backup entry %q", f.Name)
		}
		total += f.UncompressedSize64
		files[f.Name] = f
	}
	if maxUncompressed > 0 && total > uint64(maxUncompressed) {
		_ = zr.Close()
		return nil, errors.New("backup exceeds the uncompressed size limit")
	}
	manifestFile, ok := files[ManifestName]
	if !ok {
		_ = zr.Close()
		return nil, errors.New("backup manifest is missing")
	}
	rc, err := manifestFile.Open()
	if err != nil {
		_ = zr.Close()
		return nil, err
	}
	var manifest Manifest
	decodeErr := json.NewDecoder(rc).Decode(&manifest)
	_ = rc.Close()
	if decodeErr != nil {
		_ = zr.Close()
		return nil, fmt.Errorf("invalid backup manifest: %w", decodeErr)
	}
	if manifest.FormatVersion != 1 {
		_ = zr.Close()
		return nil, errors.New("unsupported backup format version")
	}
	return &Reader{zip: zr, files: files, Manifest: manifest}, nil
}

func (r *Reader) Close() error { return r.zip.Close() }

func (r *Reader) Has(name string) bool {
	_, ok := r.files[name]
	return ok
}

func (r *Reader) Open(name string) (io.ReadCloser, error) {
	if !validArchiveName(name) {
		return nil, fmt.Errorf("invalid backup entry %q", name)
	}
	f, ok := r.files[name]
	if !ok {
		return nil, fmt.Errorf("backup entry %q is missing", name)
	}
	return f.Open()
}

func validArchiveName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00") {
		return false
	}
	cleaned := path.Clean(name)
	return cleaned == name && cleaned != "." && !strings.HasPrefix(cleaned, "../")
}
