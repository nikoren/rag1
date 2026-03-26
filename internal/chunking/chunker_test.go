package chunking

import (
	"context"
	"strings"
	"testing"
)

func TestFixedSizeChunker_ChunksInOrder(t *testing.T) {
	ch := FixedSizeChunker{ChunkSize: 5}
	input := "alpha beta gamma delta"
	var got []string

	err := ch.Chunk(context.Background(), strings.NewReader(input), func(_ int, chunk string) error {
		got = append(got, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("Chunk returned error: %v", err)
	}

	want := []string{"alpha", "beta", "gamma", "delta"}
	if len(got) != len(want) {
		t.Fatalf("chunk count mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk[%d] mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestFixedSizeChunker_PacksWordsUpToSoftLimit(t *testing.T) {
	ch := FixedSizeChunker{ChunkSize: 12}
	input := "one two three four"
	var got []string

	err := ch.Chunk(context.Background(), strings.NewReader(input), func(_ int, chunk string) error {
		got = append(got, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("Chunk returned error: %v", err)
	}

	want := []string{"one two", "three four"}
	if len(got) != len(want) {
		t.Fatalf("chunk count mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk[%d] mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestFixedSizeChunker_EmptyInput(t *testing.T) {
	ch := FixedSizeChunker{ChunkSize: 5}
	calls := 0

	err := ch.Chunk(context.Background(), strings.NewReader(""), func(_ int, _ string) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("Chunk returned error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no chunks, got %d", calls)
	}
}
