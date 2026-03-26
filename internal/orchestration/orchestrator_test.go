package orchestration

import (
	"context"
	"io"
	"strings"
	"testing"

	"rag1/internal/source"
	"rag1/internal/vespa"
)

type testSource struct{}

func (testSource) GetMetadata() source.SourceMetadata {
	return source.SourceMetadata{ID: "msg_1", Type: "slack", Mode: source.SourceModeBounded}
}

func (testSource) GetReader() io.Reader {
	return strings.NewReader("ignored")
}

func TestOrchestratorIngest_IndexesParentThenChunks(t *testing.T) {
	var events []string

	chunker := &ChunkerMock{
		ChunkFunc: func(_ context.Context, _ io.Reader, emit func(index int, chunk string) error) error {
			events = append(events, "chunk")
			if err := emit(0, "first"); err != nil {
				return err
			}
			return emit(1, "second")
		},
	}
	embedder := &EmbedderMock{
		EmbedFunc: func(_ context.Context, text string) ([]float32, error) {
			events = append(events, "embed:"+text)
			return []float32{1, 2, 3}, nil
		},
	}
	repo := &RepositoryMock{
		UpsertSourceFunc: func(_ context.Context, _ source.SourceMetadata) (string, string, error) {
			events = append(events, "parent")
			return "msg_1", "id:slack:source_metadata::msg_1", nil
		},
		GetSourceStateFunc: func(_ context.Context, _ string) (source.SourceState, error) {
			return source.SourceState{}, nil
		},
		GetChunkStateFunc: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{}, nil
		},
		UpsertChunkFunc: func(_ context.Context, chunk vespa.ChunkRecord) error {
			events = append(events, "index:"+chunk.Text)
			if chunk.SequenceIndex < 0 {
				t.Fatalf("unexpected seq %d", chunk.SequenceIndex)
			}
			if chunk.ChunkID == "" || chunk.ChunkHash == "" {
				t.Fatalf("expected chunk id/hash")
			}
			return nil
		},
		DeleteChunkFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	sanitizer := &SanitizerMock{
		CleanFunc: func(text string) string {
			events = append(events, "sanitize:"+text)
			return text + "_clean"
		},
	}

	o := &Orchestrator{
		Chunker:  chunker,
		Embedder: embedder,
		Repo:     repo,
		Sanitize: sanitizer,
	}

	if err := o.Ingest(context.Background(), testSource{}); err != nil {
		t.Fatalf("Ingest error: %v", err)
	}

	want := []string{
		"parent", "chunk",
		"sanitize:first", "embed:first_clean", "index:first_clean",
		"sanitize:second", "embed:second_clean", "index:second_clean",
	}
	if len(events) != len(want) {
		t.Fatalf("event count mismatch: got %v want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("event[%d] mismatch: got %q want %q", i, events[i], want[i])
		}
	}
}

func TestOrchestratorIngest_SkipsEmptyAfterSanitize(t *testing.T) {
	chunker := &ChunkerMock{
		ChunkFunc: func(_ context.Context, _ io.Reader, emit func(index int, chunk string) error) error {
			return emit(0, "\x02\x03")
		},
	}
	embedCalls := 0
	indexCalls := 0
	embedder := &EmbedderMock{
		EmbedFunc: func(_ context.Context, _ string) ([]float32, error) {
			embedCalls++
			return []float32{1}, nil
		},
	}
	repo := &RepositoryMock{
		UpsertSourceFunc: func(_ context.Context, _ source.SourceMetadata) (string, string, error) {
			return "msg_1", "id:slack:source_metadata::msg_1", nil
		},
		GetSourceStateFunc: func(_ context.Context, _ string) (source.SourceState, error) {
			return source.SourceState{}, nil
		},
		GetChunkStateFunc: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{}, nil
		},
		UpsertChunkFunc: func(_ context.Context, _ vespa.ChunkRecord) error {
			indexCalls++
			return nil
		},
		DeleteChunkFunc: func(_ context.Context, _ string) error { return nil },
	}
	sanitizer := &SanitizerMock{
		CleanFunc: func(_ string) string { return "   " },
	}

	o := &Orchestrator{
		Chunker:  chunker,
		Embedder: embedder,
		Repo:     repo,
		Sanitize: sanitizer,
	}

	if err := o.Ingest(context.Background(), testSource{}); err != nil {
		t.Fatalf("Ingest error: %v", err)
	}
	if embedCalls != 0 {
		t.Fatalf("expected no embed calls, got %d", embedCalls)
	}
	if indexCalls != 0 {
		t.Fatalf("expected no chunk index calls, got %d", indexCalls)
	}
}

func TestOrchestratorIngest_SkipsWhenBoundedSourceHashUnchanged(t *testing.T) {
	called := false
	repo := &RepositoryMock{
		GetSourceStateFunc: func(_ context.Context, _ string) (source.SourceState, error) {
			return source.SourceState{Found: true, SourceHash: "same"}, nil
		},
		UpsertSourceFunc: func(_ context.Context, _ source.SourceMetadata) (string, string, error) {
			called = true
			return "", "", nil
		},
	}
	o := &Orchestrator{
		Chunker: &ChunkerMock{},
		Embedder: &EmbedderMock{},
		Repo: repo,
	}
	ds := testSource{}
	md := ds.GetMetadata()
	md.SourceHash = "same"
	ds2 := customSource{metadata: md, body: "hello"}
	if err := o.Ingest(context.Background(), ds2); err != nil {
		t.Fatalf("Ingest error: %v", err)
	}
	if called {
		t.Fatal("expected source indexing to be skipped")
	}
}

type customSource struct {
	metadata source.SourceMetadata
	body     string
}

func (c customSource) GetMetadata() source.SourceMetadata { return c.metadata }
func (c customSource) GetReader() io.Reader               { return strings.NewReader(c.body) }
