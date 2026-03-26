package orchestration

import (
	"context"
	"errors"
	"testing"

	"rag1/internal/dto"
)

type fakeRetriever struct {
	chunks []dto.ContextChunk
	err    error
}

func (f *fakeRetriever) Retrieve(ctx context.Context, in dto.RetrievalInput) ([]dto.ContextChunk, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.chunks, nil
}

type fakeGenerator struct {
	out dto.GenerateOutput
	err error
	got *dto.GenerateInput
}

func (f *fakeGenerator) Generate(ctx context.Context, in dto.GenerateInput) (dto.GenerateOutput, error) {
	f.got = &in
	if f.err != nil {
		return dto.GenerateOutput{}, f.err
	}
	return f.out, nil
}

func TestAnswerOrchestrator_Answer_HappyPath(t *testing.T) {
	fr := &fakeRetriever{chunks: []dto.ContextChunk{{ID: "a", Text: "ctx"}}}
	fg := &fakeGenerator{out: dto.GenerateOutput{Text: "final answer", Model: "m"}}
	o := &AnswerOrchestrator{Retriever: fr, Generator: fg}
	out, err := o.Answer(context.Background(), dto.RetrievalInput{Query: "q?", Hits: 3})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "final answer" {
		t.Fatalf("out %+v", out)
	}
	if fg.got == nil || fg.got.Question != "q?" || len(fg.got.Chunks) != 1 || fg.got.Chunks[0].ID != "a" {
		t.Fatalf("generator input %+v", fg.got)
	}
}

func TestAnswerOrchestrator_Answer_EmptyChunksStillGenerates(t *testing.T) {
	fr := &fakeRetriever{chunks: nil}
	fg := &fakeGenerator{out: dto.GenerateOutput{Text: "no data"}}
	o := &AnswerOrchestrator{Retriever: fr, Generator: fg}
	out, err := o.Answer(context.Background(), dto.RetrievalInput{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "no data" {
		t.Fatalf("out %+v", out)
	}
	if fg.got == nil || len(fg.got.Chunks) != 0 {
		t.Fatalf("expected empty chunks passed to generator")
	}
}

func TestAnswerOrchestrator_Answer_RetrieveError(t *testing.T) {
	fr := &fakeRetriever{err: errors.New("boom")}
	fg := &fakeGenerator{}
	o := &AnswerOrchestrator{Retriever: fr, Generator: fg}
	_, err := o.Answer(context.Background(), dto.RetrievalInput{Query: "q"})
	if err == nil {
		t.Fatal("expected error")
	}
	if fg.got != nil {
		t.Fatal("generator should not run")
	}
}

func TestAnswerOrchestrator_Answer_NilDeps(t *testing.T) {
	o := &AnswerOrchestrator{}
	if _, err := o.Answer(context.Background(), dto.RetrievalInput{Query: "q"}); err == nil {
		t.Fatal("expected error")
	}
}
