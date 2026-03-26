package vespa

import (
	"context"

	"rag1/internal/source"
)

// ChunkRecord is one knowledge_chunk document: parent linkage, stable chunk id, text, unix IndexedAt, and embedding slice.
//
// Example: UpsertChunk(ctx, ChunkRecord{ParentRef: ref, ChunkID: id+"#chunk_0", Text: t, Embedding: vec}).
type ChunkRecord struct {
	ParentID      string    // Namespace-local parent key (same as source metadata doc id string).
	ParentRef     string    // Vespa reference id:... form stored on the chunk (links to source_metadata).
	SequenceIndex int       // Chunk order within the source; part of default child doc id encoding.
	ChunkID       string    // Stable logical id (e.g. parent#chunk_0) used for delete and diff maps.
	ChunkHash     string    // SHA-256 hex of sanitized text for incremental skip.
	Text          string    // Sanitized chunk body stored in text_content for BM25.
	IndexedAt     int64     // Unix seconds when this chunk was last written.
	Embedding     []float32 // Dense vector; length must match schema tensor (e.g. 384).
}

// Repository is the persistence surface for orchestration (implemented by Client; mock in tests).
type Repository interface {
	// UpsertSource writes source_metadata and returns (parentID, parentRef, err).
	UpsertSource(ctx context.Context, md source.SourceMetadata) (string, string, error)
	// GetSourceState loads source_hash for skip-if-unchanged when the source is bounded.
	GetSourceState(ctx context.Context, parentID string) (source.SourceState, error)
	// GetChunkState returns chunk_id → chunk_hash for all chunks under parentRef (paginated internally).
	GetChunkState(ctx context.Context, parentRef string) (map[string]string, error)
	// UpsertChunk writes or replaces one knowledge_chunk document.
	UpsertChunk(ctx context.Context, chunk ChunkRecord) error
	// DeleteChunk removes a chunk document by id (stale entries after re-ingest).
	DeleteChunk(ctx context.Context, chunkID string) error
}
