// Package hashutil provides deterministic hashing helpers for chunk and source fingerprints.
//
// Example: orchestration uses HashText(sanitizedChunk) as chunk_hash for Vespa upserts and diffing.
package hashutil

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashText returns the lowercase hex SHA-256 of the UTF-8 bytes of text (stable across processes).
//
// Example: hash := HashText("hello") for comparing chunk content without storing the full string twice.
func HashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
