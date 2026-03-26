// Package retrieval implements embedding + Vespa search and maps hits to dto.ContextChunk.
package retrieval

import (
	"context"
	"fmt"

	"rag1/internal/dto"
	"rag1/internal/embeddings"
	"rag1/internal/vespa"
)

// VespaRetriever runs hybrid search using an Embedder and Vespa Client.
type VespaRetriever struct {
	Embedder embeddings.Embedder
	Client   *vespa.Client
}

// Retrieve embeds in.Query, searches Vespa, and maps results to dto.ContextChunk.
func (r *VespaRetriever) Retrieve(ctx context.Context, in dto.RetrievalInput) ([]dto.ContextChunk, error) {
	if r == nil || r.Embedder == nil || r.Client == nil {
		return nil, fmt.Errorf("retrieval: nil retriever or dependencies")
	}
	q := in.Query
	if q == "" {
		return nil, fmt.Errorf("retrieval: empty query")
	}
	vec, err := r.Embedder.Embed(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	hits := in.Hits
	if hits <= 0 {
		hits = 10
	}
	raw, err := r.Client.Search(ctx, vespa.SearchRequest{
		QueryText:  q,
		UserVector: vec,
		Hits:       hits,
		Offset:     0,
		FilterURL:  in.FilterURL,
	})
	if err != nil {
		return nil, err
	}
	out := make([]dto.ContextChunk, 0, len(raw))
	for _, h := range raw {
		out = append(out, dto.ContextChunk{
			ID:            h.ChunkID,
			Text:          h.TextContent,
			SourceURL:     h.URL,
			SourceType:    h.SourceType,
			Relevance:     h.Relevance,
			SequenceIndex: h.SequenceIndex,
		})
	}
	return out, nil
}
