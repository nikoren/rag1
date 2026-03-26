package orchestration

import (
	"context"
	"fmt"
	"log/slog"

	"rag1/internal/dto"
)

// Retriever fetches context chunks for a user query (e.g. Vespa hybrid search after embedding).
type Retriever interface {
	Retrieve(ctx context.Context, in dto.RetrievalInput) ([]dto.ContextChunk, error)
}

// Generator turns retrieved chunks and a question into a natural-language answer (e.g. chat completion).
type Generator interface {
	Generate(ctx context.Context, in dto.GenerateInput) (dto.GenerateOutput, error)
}

// AnswerOrchestrator runs retrieve-then-generate for RAG; independent of Ingest orchestration.
type AnswerOrchestrator struct {
	Retriever Retriever
	Generator Generator
	Logger    *slog.Logger
}

// Answer retrieves chunks for in, then generates an answer using the same query string as the user question.
func (o *AnswerOrchestrator) Answer(ctx context.Context, in dto.RetrievalInput) (dto.GenerateOutput, error) {
	if o == nil || o.Retriever == nil || o.Generator == nil {
		return dto.GenerateOutput{}, fmt.Errorf("orchestration: nil AnswerOrchestrator or dependencies")
	}
	chunks, err := o.Retriever.Retrieve(ctx, in)
	if err != nil {
		return dto.GenerateOutput{}, err
	}
	if o.Logger != nil {
		o.Logger.Info("retrieved chunks for answer", "count", len(chunks), "query", in.Query)
	}
	return o.Generator.Generate(ctx, dto.GenerateInput{
		Question: in.Query,
		Chunks:   chunks,
	})
}
