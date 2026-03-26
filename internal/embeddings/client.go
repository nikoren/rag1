// Package embeddings calls OpenAI-compatible /v1/embeddings HTTP APIs (e.g. LM Studio) for dense vectors.
//
// Example: Client with URL http://127.0.0.1:1234/v1/embeddings and ExpectedDims 384 matches Vespa tensor shape.
package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// Embedder turns a text string into a float32 vector for the configured model (same interface as orchestration).
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Client is a concrete Embedder using JSON POST requests; fields must match your server and Vespa embedding dimension.
type Client struct {
	HTTPClient   *http.Client // Shared client for timeouts and tracing; required for production use.
	URL          string       // Full embeddings endpoint URL (e.g. .../v1/embeddings).
	Model        string       // Model id sent in the JSON body; must match indexer and query-time embedding.
	ExpectedDims int          // If >0, Embed errors when the response length differs (guards schema tensor size).
	Logger       *slog.Logger // Optional; Debug logs when a vector is produced.
}

// requestPayload is the JSON body sent to the embeddings API (model + single input string).
type requestPayload struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// responsePayload mirrors a typical OpenAI-style embeddings response (first data element’s vector).
type responsePayload struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// Embed POSTs text to c.URL, parses the first embedding, converts to float32, and validates ExpectedDims if set.
//
// Example: vec, err := client.Embed(ctx, "query text") then pass vec to Vespa as query(user_vector).
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	payloadBytes, err := json.Marshal(requestPayload{
		Model: c.Model,
		Input: text,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding request failed: %s: %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed responsePayload
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %w", err)
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding response has no vectors")
	}

	vector := make([]float32, len(parsed.Data[0].Embedding))
	for i, v := range parsed.Data[0].Embedding {
		vector[i] = float32(v)
	}

	if c.ExpectedDims > 0 && len(vector) != c.ExpectedDims {
		return nil, fmt.Errorf("embedding dimension mismatch: got %d, want %d", len(vector), c.ExpectedDims)
	}

	if c.Logger != nil {
		c.Logger.Debug("embedding generated", "dims", len(vector))
	}

	return vector, nil
}
