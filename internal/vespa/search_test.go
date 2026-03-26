package vespa

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSearchResponse(t *testing.T) {
	const raw = `{
  "root": {
    "children": [
      {
        "relevance": 0.42,
        "fields": {
          "chunk_id": "pdf_abc#chunk_0",
          "text_content": "hello world",
          "url": "/books/a.pdf",
          "source_type": "pdf",
          "sequence_index": 0
        }
      }
    ]
  }
}`
	hits, err := parseSearchResponse([]byte(raw))
	if err != nil {
		t.Fatalf("parseSearchResponse: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("len %d", len(hits))
	}
	h := hits[0]
	if h.Relevance != 0.42 || h.ChunkID != "pdf_abc#chunk_0" || h.TextContent != "hello world" {
		t.Fatalf("unexpected hit: %+v", h)
	}
	if h.URL != "/books/a.pdf" || h.SourceType != "pdf" || h.SequenceIndex != 0 {
		t.Fatalf("unexpected metadata: %+v", h)
	}
}

func TestBuildHybridYQL(t *testing.T) {
	q := buildHybridYQL(100, "")
	if !strings.Contains(q, "chunk_id") || !strings.Contains(q, "nearestNeighbor(embedding, user_vector)") || !strings.Contains(q, "userQuery()") {
		t.Fatalf("unexpected yql: %s", q)
	}
	q2 := buildHybridYQL(50, `/books/foo bar.pdf`)
	if !strings.Contains(q2, `url contains`) || !strings.Contains(q2, `foo bar`) {
		t.Fatalf("expected url filter in yql: %s", q2)
	}
}

func TestEscapeYQLString(t *testing.T) {
	if got := escapeYQLString(`a"b\c`); got != `a\"b\\c` {
		t.Fatalf("got %q", got)
	}
}

func TestClient_Search(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search/" {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err := json.Unmarshal(b, &gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{
  "root": {
    "children": [
      {
        "relevance": 1.5,
        "fields": {
          "chunk_id": "c1",
          "text_content": "snippet",
          "url": "file:///x.pdf",
          "source_type": "pdf",
          "sequence_index": 2
        }
      }
    ]
  }
}`))
	}))
	defer server.Close()

	c := &Client{HTTPClient: server.Client(), Host: server.URL, Namespace: "slack"}
	vec := make([]float32, 384)
	vec[0] = 0.25
	hits, err := c.Search(context.Background(), SearchRequest{
		QueryText:  "dashboard design",
		UserVector: vec,
		Hits:       5,
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].ChunkID != "c1" || hits[0].Relevance != 1.5 {
		t.Fatalf("unexpected hits: %+v", hits)
	}

	ranking, _ := gotBody["ranking"].(map[string]any)
	if ranking["profile"] != "hybrid_search" {
		t.Fatalf("ranking profile: %v", ranking)
	}
	feat, _ := ranking["features"].(map[string]any)
	qv, ok := feat["query(user_vector)"].([]any)
	if !ok || len(qv) != 384 {
		t.Fatalf("query tensor in request: %v", feat["query(user_vector)"])
	}
	if gotBody["query"] != "dashboard design" {
		t.Fatalf("query text: %v", gotBody["query"])
	}
}

func TestClient_Search_EmptyVector(t *testing.T) {
	c := &Client{HTTPClient: http.DefaultClient, Host: "http://localhost"}
	_, err := c.Search(context.Background(), SearchRequest{QueryText: "x", UserVector: nil})
	if err == nil {
		t.Fatal("expected error")
	}
}
