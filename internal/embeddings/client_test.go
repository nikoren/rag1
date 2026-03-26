package embeddings

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientEmbed_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer server.Close()

	c := &Client{
		HTTPClient:   server.Client(),
		URL:          server.URL,
		Model:        "model",
		ExpectedDims: 3,
	}

	vec, err := c.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("len mismatch: got %d", len(vec))
	}
}

func TestClientEmbed_DimMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()

	c := &Client{
		HTTPClient:   server.Client(),
		URL:          server.URL,
		Model:        "model",
		ExpectedDims: 3,
	}

	if _, err := c.Embed(context.Background(), "hello"); err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}
