// Package textclean strips or normalizes runes so text is safe for indexing and embedding APIs.
//
// Example: pass DefaultSanitizer into the orchestrator to remove Vespa-invalid control characters from PDF text.
package textclean

import "unicode"

// Sanitizer transforms raw chunk text before hashing and embedding.
type Sanitizer interface {
	Clean(text string) string
}

// DefaultSanitizer removes Unicode control/format characters except tab, LF, CR; keeps normal text and spaces.
type DefaultSanitizer struct{}

// Clean returns a copy of text with disallowed runes removed (idempotent for already-clean input).
//
// Example: Clean("a\u0000b") → "ab"; newlines and tabs are preserved for chunk boundaries.
func (s DefaultSanitizer) Clean(text string) string {
	out := make([]rune, 0, len(text))
	for _, r := range text {
		if isAllowedRune(r) {
			out = append(out, r)
		}
	}
	return string(out)
}

// isAllowedRune implements DefaultSanitizer policy: allow common whitespace controls, drop other controls/Cf.
func isAllowedRune(r rune) bool {
	// Keep common whitespace controls.
	if r == '\n' || r == '\r' || r == '\t' {
		return true
	}
	// Drop all other control and formatting code points.
	if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
		return false
	}
	return true
}
