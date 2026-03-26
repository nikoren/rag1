// Package orchestration coordinates chunking, sanitization, embedding, and Vespa upserts for one DataSource.
//
// Example: wire Orchestrator with chunking.FixedSizeChunker, embeddings.Client, vespa.Client; call Ingest(ctx, pdfSource).
package orchestration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"rag1/internal/hashutil"
	"rag1/internal/source"
	"rag1/internal/vespa"
)

//go:generate go run github.com/matryer/moq@latest -pkg orchestration -out mocks_test.go . Chunker Embedder Repository Sanitizer

// Chunker is the streaming splitter used by Ingest (same shape as chunking.Chunker for tests/mocks).
type Chunker interface {
	Chunk(ctx context.Context, reader io.Reader, emit func(index int, chunk string) error) error
}

// Embedder produces one vector per sanitized chunk (same shape as embeddings.Embedder).
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Repository abstracts Vespa parent/child writes and state reads (implemented by vespa.Client).
type Repository interface {
	UpsertSource(ctx context.Context, md source.SourceMetadata) (string, string, error)
	GetSourceState(ctx context.Context, parentID string) (source.SourceState, error)
	GetChunkState(ctx context.Context, parentRef string) (map[string]string, error)
	UpsertChunk(ctx context.Context, chunk vespa.ChunkRecord) error
	DeleteChunk(ctx context.Context, chunkID string) error
}

// Sanitizer cleans chunk text before hash/embed (same shape as textclean.Sanitizer).
type Sanitizer interface {
	Clean(text string) string
}

// Orchestrator holds dependencies; all fields are used by Ingest—inject nil Sanitizer only if cleaning is undesired.
type Orchestrator struct {
	Chunker  Chunker      // Splits ds.GetReader() and calls emit per segment.
	Embedder Embedder     // Embeds each non-empty sanitized chunk.
	Repo     Repository   // Vespa upserts and incremental diff state.
	Sanitize Sanitizer    // Optional; if nil, raw chunk text is hashed and embedded.
	Logger   *slog.Logger // Optional structured logs for progress and summaries.
}

// Ingest upserts source metadata, loads prior chunk hashes, reuses unchanged chunks, embeds/upserts changed ones, deletes stale ids when bounded.
//
// Example: err := orch.Ingest(ctx, pdfSource) after configuring Repo to your Vespa namespace.
func (o *Orchestrator) Ingest(ctx context.Context, ds source.DataSource) error {
	md := ds.GetMetadata()

	mode := md.Mode
	if mode == "" {
		mode = source.SourceModeBounded
	}

	if mode == source.SourceModeBounded && md.SourceHash != "" && md.ID != "" {
		srcState, err := o.Repo.GetSourceState(ctx, md.ID)
		if err != nil {
			return err
		}
		if srcState.Found && srcState.SourceHash == md.SourceHash {
			if o.Logger != nil {
				o.Logger.Info("source unchanged, skipping ingestion", "id", md.ID)
			}
			return nil
		}
	}

	parentID, parentRef, err := o.Repo.UpsertSource(ctx, md)
	if err != nil {
		return err
	}

	oldState, err := o.Repo.GetChunkState(ctx, parentRef)
	if err != nil {
		return err
	}

	if o.Logger != nil {
		o.Logger.Info("source indexed", "id", parentID, "ref", parentRef)
	}

	var chunksTotal, chunksSkipped, chunksUpserted, chunksDeleted int

	if err := o.Chunker.Chunk(
		ctx,
		ds.GetReader(),
		// The emit function is responsible for outputting (emitting) the chunk of text when called.
		// - It increments the total chunk count.
		// - It sanitizes the chunk text if a Sanitizer is provided.
		// - If the sanitized text is empty, it skips the chunk and logs a debug message.
		// - Otherwise, it calculates the chunk ID and hash, and upserts the chunk record.
		// - It increments the upserted chunk count and deletes the chunk from the old state.
		// - If the upsert fails, it returns the error.
		func(index int, chunk string) error {
			chunksTotal++
			sanitized := chunk
			if o.Sanitize != nil {
				sanitized = o.Sanitize.Clean(chunk)
			}
			if strings.TrimSpace(sanitized) == "" {
				chunksSkipped++
				if o.Logger != nil {
					o.Logger.Debug("skipping empty chunk after sanitization", "sequence_index", index)
				}
				return nil
			}

			chunkID := fmt.Sprintf("%s#chunk_%d", parentID, index)
			chunkHash := hashutil.HashText(sanitized)
			if oldHash, exists := oldState[chunkID]; exists && oldHash == chunkHash {
				chunksSkipped++
				delete(oldState, chunkID)
				return nil
			}

			embedding, err := o.Embedder.Embed(ctx, sanitized)
			if err != nil {
				return err
			}

			if err := o.Repo.UpsertChunk(ctx,
				vespa.ChunkRecord{
					ParentID:      parentID,
					ParentRef:     parentRef,
					SequenceIndex: index,
					ChunkID:       chunkID,
					ChunkHash:     chunkHash,
					Text:          sanitized,
					IndexedAt:     time.Now().Unix(),
					Embedding:     embedding,
				}); err != nil {
				return err
			}
			chunksUpserted++
			delete(oldState, chunkID)
			return nil
		}); err != nil {
		return err
	}

	if mode == source.SourceModeBounded {
		for staleChunkID := range oldState {
			if err := o.Repo.DeleteChunk(ctx, staleChunkID); err != nil {
				return err
			}
			chunksDeleted++
		}
	}

	if o.Logger != nil {
		o.Logger.Info(
			"ingestion chunk summary",
			"source_mode", mode,
			"chunks_total", chunksTotal,
			"chunks_skipped_same_or_empty", chunksSkipped,
			"chunks_upserted", chunksUpserted,
			"chunks_deleted_stale", chunksDeleted,
		)
	}
	return nil
}
