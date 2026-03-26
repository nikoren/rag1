package sourcedetect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath replaces leading "~" or "~/" with os.UserHomeDir(); other paths are returned unchanged (trimmed).
//
// Example: ExpandPath("~/books/a.pdf") → "/Users/me/books/a.pdf" on Unix; required because Go does not expand ~.
func ExpandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
