// Package chunking splits arbitrary text streams into word-oriented segments up to a rune budget.
//
// Example: orchestration passes ds.GetReader() and emits each chunk for sanitize → embed → index.
package chunking

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Chunker streams input and calls emit for each segment; index is 0-based sequence order for stable chunk ids.
type Chunker interface {
	Chunk(ctx context.Context, reader io.Reader, emit func(index int, chunk string) error) error
}

// FixedSizeChunker buffers words until adding the next word would exceed ChunkSize runes (soft boundary at word gaps).
type FixedSizeChunker struct {
	ChunkSize int // Maximum runes per chunk (excluding the soft split behavior between words).
}

// Chunk scans words from reader and invokes emit for each chunk; respects ctx cancellation between words.
//
// Example: ChunkSize 500 yields chunks of roughly ≤500 runes, splitting before long runs when needed.
func (c FixedSizeChunker) Chunk(ctx context.Context, reader io.Reader, emit func(index int, chunk string) error) error {
	if c.ChunkSize <= 0 {
		return fmt.Errorf("chunk size must be positive")
	}

	scanner := bufio.NewScanner(reader)
	// Increase token limit to handle long words without scanner failures.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	scanner.Split(bufio.ScanWords)

	var b strings.Builder
	index := 0
	runeCount := 0

	// The flush function is responsible for outputting (emitting) the accumulated chunk of text when called.
	// - If the buffer is empty, it does nothing.
	// - Otherwise, it calls the user-provided emit function with the current chunk's index and content.
	// - If emit returns an error, flush returns this error.
	// - After emitting, it increments the chunk index, resets the buffer, and zeroes the rune count for the next chunk.
	flush := func() error {
		if b.Len() == 0 {
			return nil
		}
		if err := emit(index, b.String()); err != nil {
			return err
		}
		index++
		b.Reset()
		runeCount = 0
		return nil
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		word := scanner.Text()
		wordRunes := utf8.RuneCountInString(word)

		needed := wordRunes
		if b.Len() > 0 {
			needed++ // one space between words
		}
		// Soft cap: prefer full words over mid-word slicing.
		if b.Len() > 0 && runeCount+needed > c.ChunkSize {
			if err := flush(); err != nil {
				return err
			}
		}

		if b.Len() > 0 {
			b.WriteByte(' ')
			runeCount++
		}
		b.WriteString(word)
		runeCount += wordRunes
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return flush()
}
