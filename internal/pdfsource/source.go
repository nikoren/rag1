// Package pdfsource implements source.DataSource for local PDF files: extract text once, stream via strings.Reader.
//
// Example: s, err := pdfsource.New("/path/book.pdf", source.SourceMetadata{Type: "pdf"}); then orch.Ingest(ctx, s).
package pdfsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"

	"rag1/internal/hashutil"
	"rag1/internal/source"
)

// Source holds absolute path, filled SourceMetadata, and full extracted plain text for GetReader.
type Source struct {
	path     string                     // Absolute filesystem path after New (for logging and hashing).
	metadata source.SourceMetadata      // ID, hashes, mode, URL — populated by New when fields are empty.
	text     string                     // Full PDF text extracted at construction time.
}

// New validates path, fills metadata defaults (bounded mode, id, source hash), and extracts text via ledongthuc/pdf.
//
// Example: New("book.pdf", source.SourceMetadata{Author: "me"}) — path should be absolute or cwd-relative after expand.
func New(path string, md source.SourceMetadata) (*Source, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("pdf path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("pdf path is a directory: %s", absPath)
	}

	if md.Type == "" {
		md.Type = "pdf"
	}
	if md.Mode == "" {
		md.Mode = source.SourceModeBounded
	}
	if md.URL == "" {
		md.URL = absPath
	}
	if md.Timestamp == 0 {
		md.Timestamp = fi.ModTime().Unix()
		if md.Timestamp == 0 {
			md.Timestamp = time.Now().Unix()
		}
	}
	if md.ID == "" {
		sum := sha256.Sum256([]byte(absPath + "|" + fmt.Sprintf("%d", md.Timestamp)))
		md.ID = "pdf_" + hex.EncodeToString(sum[:8])
	}

	text, err := extractText(context.Background(), absPath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("extracted empty text from pdf: %s", absPath)
	}
	if md.SourceHash == "" {
		md.SourceHash = hashutil.HashText(text)
	}

	return &Source{
		path:     absPath,
		metadata: md,
		text:     text,
	}, nil
}

// GetMetadata returns the struct filled by New (id, source_hash, url, timestamp) for Vespa parent docs and skip logic.
func (s *Source) GetMetadata() source.SourceMetadata {
	return s.metadata
}

// GetReader exposes full extracted text as a stream for chunking (e.g. FixedSizeChunker over one logical document).
func (s *Source) GetReader() io.Reader {
	return strings.NewReader(s.text)
}

// extractText uses ledongthuc/pdf to read plain text, honors ctx after ReadAll, appends newline for page separation.
func extractText(ctx context.Context, path string) (string, error) {
	// ledongthuc/pdf returns an *os.File that must be closed by caller.
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	plain, err := r.GetPlainText()
	if err != nil {
		return "", err
	}

	// Keep this extraction simple and deterministic: read all extracted text once.
	// Downstream chunking remains streaming, and this can be optimized later if needed.
	b, err := io.ReadAll(plain)
	if err != nil {
		return "", err
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// Add an extra newline to help separate page boundaries in some PDFs.
	// (Some PDFs already contain line breaks; this is a light normalization.)
	return string(b) + "\n", nil
}
