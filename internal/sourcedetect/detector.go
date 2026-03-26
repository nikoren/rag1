// Package sourcedetect detects source file formats for ingestion and expands home-relative paths (~) for flags.
//
// Example: ExpandPath("~/doc.pdf") then Detect(path) to choose pdfsource vs future types.
package sourcedetect

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	// KindPDF is the detector label returned for paths whose extension is .pdf (case-insensitive).
	KindPDF = "pdf"
)

// Detect returns a format kind based on the file extension, or an error if missing/unsupported.
//
// Example: kind, err := Detect("/data/book.PDF") → KindPDF, nil; ".txt" returns unsupported format error.
func Detect(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("source path is required")
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return KindPDF, nil
	default:
		if ext == "" {
			return "", fmt.Errorf("unsupported source file format: no extension")
		}
		return "", fmt.Errorf("unsupported source file format: %s", ext)
	}
}
