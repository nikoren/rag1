package vespa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SearchRequest is a hybrid retrieval query: natural language for BM25 (userQuery) plus dense vector for nearestNeighbor.
//
// Example: embed the same QueryText with the indexer model, set UserVector to the result, Hits to 10, FilterURL to "" for all books.
type SearchRequest struct {
	QueryText  string    // Passed to Vespa as `query` for bm25(text_content) via userQuery() in YQL.
	UserVector []float32 // Must match schema tensor(x[384]); used as query(user_vector) for nearestNeighbor/closeness.
	Hits       int       // Max hits to return (default 10 in CLI if unset).
	Offset     int       // Pagination offset.
	FilterURL  string    // Optional: if non-empty, restricts to chunks whose imported url contains this substring.
}

// SearchHit is one ranked knowledge_chunk match from a search response.
type SearchHit struct {
	Relevance     float64
	ChunkID       string
	TextContent   string
	URL           string
	SourceType    string
	SequenceIndex int
}

// Search runs hybrid search (nearestNeighbor + userQuery) with rank profile hybrid_search.
func (c *Client) Search(ctx context.Context, req SearchRequest) ([]SearchHit, error) {
	if len(req.UserVector) == 0 {
		return nil, fmt.Errorf("search: UserVector is required")
	}
	hits := req.Hits
	if hits <= 0 {
		hits = 10
	}
	targetHits := hits + req.Offset
	if targetHits < 100 {
		targetHits = 100
	}

	// Project only fields we need so Vespa fetches smaller summaries (avoids 504 on large corpora).
	yql := buildHybridYQL(targetHits, req.FilterURL)

	vec := make([]float64, len(req.UserVector))
	for i, v := range req.UserVector {
		vec[i] = float64(v)
	}

	body, err := json.Marshal(map[string]any{
		"yql":     yql,
		"query":   req.QueryText,
		"hits":    hits,
		"offset":  req.Offset,
		"timeout": "60s",
		"ranking": map[string]any{
			"profile": "hybrid_search",
			"features": map[string]any{
				// Indexed dense tensor: Vespa accepts array form for query(myTensor) inputs.
				"query(user_vector)": vec,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	searchURL := strings.TrimSuffix(c.Host, "/") + "/search/"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, searchURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("vespa search failed: %s: %s", resp.Status, string(respBody))
	}

	return parseSearchResponse(respBody)
}

func buildHybridYQL(targetHits int, filterURL string) string {
	proj := "chunk_id, text_content, url, source_type, sequence_index"
	base := fmt.Sprintf(
		`select %s from knowledge_chunk where ({targetHits:%d}nearestNeighbor(embedding, user_vector)) or userQuery();`,
		proj, targetHits,
	)
	if strings.TrimSpace(filterURL) == "" {
		return base
	}
	esc := escapeYQLString(filterURL)
	return fmt.Sprintf(
		`select %s from knowledge_chunk where (({targetHits:%d}nearestNeighbor(embedding, user_vector)) or userQuery()) and url contains "%s";`,
		proj, targetHits, esc,
	)
}

func escapeYQLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func parseSearchResponse(body []byte) ([]SearchHit, error) {
	var payload struct {
		Root struct {
			Children []struct {
				Relevance float64 `json:"relevance"`
				Fields    struct {
					ChunkID       string `json:"chunk_id"`
					TextContent   string `json:"text_content"`
					URL           string `json:"url"`
					SourceType    string `json:"source_type"`
					SequenceIndex int    `json:"sequence_index"`
				} `json:"fields"`
			} `json:"children"`
		} `json:"root"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	out := make([]SearchHit, 0, len(payload.Root.Children))
	for _, ch := range payload.Root.Children {
		out = append(out, SearchHit{
			Relevance:     ch.Relevance,
			ChunkID:       ch.Fields.ChunkID,
			TextContent:   ch.Fields.TextContent,
			URL:           ch.Fields.URL,
			SourceType:    ch.Fields.SourceType,
			SequenceIndex: ch.Fields.SequenceIndex,
		})
	}
	return out, nil
}
