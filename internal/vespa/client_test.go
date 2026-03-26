package vespa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rag1/internal/source"
)

func TestClient_UpsertSourceAndChunkAndDelete(t *testing.T) {
	var parentBody map[string]any
	var chunkBody map[string]any
	var deletedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			defer r.Body.Close()
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if strings.Contains(r.URL.Path, "source_metadata") {
				parentBody = body
			} else {
				chunkBody = body
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if strings.HasPrefix(r.URL.Path, "/search/") {
				_, _ = w.Write([]byte(`{"root":{"fields":{"totalCount":0},"children":[]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"fields":{"source_hash":"hsrc"}}`))
		case http.MethodDelete:
			deletedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	c := &Client{HTTPClient: server.Client(), Host: server.URL, Namespace: "slack"}
	parentID, parentRef, err := c.UpsertSource(context.Background(), source.SourceMetadata{
		ID: "msg_123", Type: "pdf", SourceHash: "hsrc",
	})
	if err != nil {
		t.Fatalf("UpsertSource: %v", err)
	}
	if parentID != "msg_123" || parentRef == "" {
		t.Fatalf("unexpected source result: %s %s", parentID, parentRef)
	}
	if err := c.UpsertChunk(context.Background(), ChunkRecord{
		ParentID: parentID, ParentRef: parentRef, SequenceIndex: 0,
		ChunkID: "msg_123#chunk_0", ChunkHash: "h0", Text: "hello", IndexedAt: 1,
		Embedding: []float32{1, 2},
	}); err != nil {
		t.Fatalf("UpsertChunk: %v", err)
	}
	if _, err := c.GetSourceState(context.Background(), parentID); err != nil {
		t.Fatalf("GetSourceState: %v", err)
	}
	if err := c.DeleteChunk(context.Background(), "msg_123#chunk_0"); err != nil {
		t.Fatalf("DeleteChunk: %v", err)
	}

	if parentBody["fields"].(map[string]any)["source_hash"] != "hsrc" {
		t.Fatalf("missing source_hash in source payload")
	}
	if chunkBody["fields"].(map[string]any)["chunk_hash"] != "h0" {
		t.Fatalf("missing chunk_hash in chunk payload")
	}
	if deletedPath == "" {
		t.Fatalf("expected delete call")
	}
}

func TestClient_GetChunkState_Paginates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/search/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		offset := q.Get("offset")
		switch offset {
		case "0":
			_, _ = w.Write([]byte(`{"root":{"fields":{"totalCount":3},"children":[{"fields":{"chunk_id":"c1","chunk_hash":"h1"}},{"fields":{"chunk_id":"c2","chunk_hash":"h2"}}]}}`))
		case "2":
			_, _ = w.Write([]byte(`{"root":{"fields":{"totalCount":3},"children":[{"fields":{"chunk_id":"c3","chunk_hash":"h3"}}]}}`))
		default:
			t.Fatalf("unexpected offset: %s", offset)
		}
	}))
	defer server.Close()

	c := &Client{
		HTTPClient: server.Client(),
		Host:       server.URL,
		Namespace:  "slack",
		PageSize:   2,
	}
	state, err := c.GetChunkState(context.Background(), "id:slack:source_metadata::msg_1")
	if err != nil {
		t.Fatalf("GetChunkState: %v", err)
	}
	if len(state) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(state))
	}
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("c%d", i)
		if state[id] == "" {
			t.Fatalf("missing state for %s", id)
		}
	}
}
