package vespa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"rag1/internal/source"
)

// Client implements Repository via POST/GET/DELETE to /document/v1 and GET /search for chunk state paging.
type Client struct {
	HTTPClient *http.Client // Used for all requests; set timeouts on shared clients.
	Host       string       // Base URL like http://localhost:8080 (no trailing slash).
	Namespace  string       // Vespa document type namespace segment in URLs (e.g. "slack").
	Logger     *slog.Logger // Optional; Info on upserts.
	PageSize   int          // Hits per search page when listing chunks; capped by normalizePageSize (default 200).
}

// sourceDocument is the JSON envelope for POST source_metadata (id = md.ID or generated).
type sourceDocument struct {
	Fields struct {
		SourceID   string `json:"source_id"`
		SourceType string `json:"source_type"`
		Author     string `json:"author"`
		URL        string `json:"url"`
		Timestamp  int64  `json:"timestamp"`
		SourceHash string `json:"source_hash,omitempty"`
	} `json:"fields"`
}

// chunkDocument is the JSON envelope for POST knowledge_chunk (embedding + text_content fields).
type chunkDocument struct {
	Fields struct {
		ParentRef     string    `json:"parent_ref"`
		SequenceIndex int       `json:"sequence_index"`
		ChunkID       string    `json:"chunk_id"`
		ChunkHash     string    `json:"chunk_hash"`
		IndexedAt     int64     `json:"indexed_at"`
		TextContent   string    `json:"text_content"`
		Embedding     []float32 `json:"embedding"`
	} `json:"fields"`
}

// UpsertSource PUTs/POSTs parent metadata and returns (parentID, parentRef, err); parentRef is id:namespace:source_metadata::id.
//
// Example: id, ref, err := c.UpsertSource(ctx, md) then pass ref to GetChunkState.
func (c *Client) UpsertSource(ctx context.Context, md source.SourceMetadata) (string, string, error) {
	parentID := md.ID
	if parentID == "" {
		parentID = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	if md.Timestamp == 0 {
		md.Timestamp = time.Now().Unix()
	}

	doc := sourceDocument{}
	doc.Fields.SourceID = parentID
	doc.Fields.SourceType = md.Type
	doc.Fields.Author = md.Author
	doc.Fields.URL = md.URL
	doc.Fields.Timestamp = md.Timestamp
	doc.Fields.SourceHash = md.SourceHash

	payload, err := json.Marshal(doc)
	if err != nil {
		return "", "", err
	}
	if err := c.postJSON(ctx, c.parentDocURL(parentID), payload); err != nil {
		return "", "", err
	}
	parentRef := c.parentRef(parentID)
	if c.Logger != nil {
		c.Logger.Info("upserted source metadata", "parent_id", parentID, "parent_ref", parentRef)
	}
	return parentID, parentRef, nil
}

// GetSourceState GETs one source_metadata document and returns Found plus stored source_hash (404 → Found false).
func (c *Client) GetSourceState(ctx context.Context, parentID string) (source.SourceState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.parentDocURL(parentID), nil)
	if err != nil {
		return source.SourceState{}, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return source.SourceState{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return source.SourceState{Found: false}, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return source.SourceState{}, fmt.Errorf("vespa get source state failed: %s: %s", resp.Status, string(body))
	}
	var payload struct {
		Fields struct {
			SourceHash string `json:"source_hash"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return source.SourceState{}, err
	}
	return source.SourceState{Found: true, SourceHash: payload.Fields.SourceHash}, nil
}

// GetChunkState merges paginated search results into chunk_id → chunk_hash for incremental diff (uses PageSize).
func (c *Client) GetChunkState(ctx context.Context, parentRef string) (map[string]string, error) {
	pageSize := normalizePageSize(c.PageSize)
	offset := 0
	state := map[string]string{}
	for {
		page, totalCount, err := c.getChunkStatePage(ctx, parentRef, pageSize, offset)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		for id, hash := range page {
			state[id] = hash
		}
		offset += len(page)
		if totalCount > 0 && offset >= totalCount {
			break
		}
	}
	return state, nil
}

// getChunkStatePage runs one YQL search page: select chunk_id, chunk_hash from knowledge_chunk where parent_ref contains "...".
func (c *Client) getChunkStatePage(ctx context.Context, parentRef string, hits, offset int) (map[string]string, int, error) {
	yql := fmt.Sprintf("select chunk_id, chunk_hash from knowledge_chunk where parent_ref contains \"%s\";", parentRef)
	searchURL := fmt.Sprintf(
		"%s/search/?yql=%s&hits=%d&offset=%d",
		c.Host,
		url.QueryEscape(yql),
		hits,
		offset,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("vespa get chunk state failed: %s: %s", resp.Status, string(body))
	}
	var payload struct {
		Root struct {
			Fields struct {
				TotalCount int `json:"totalCount"`
			} `json:"fields"`
			Children []struct {
				Fields struct {
					ChunkID   string `json:"chunk_id"`
					ChunkHash string `json:"chunk_hash"`
				} `json:"fields"`
			} `json:"children"`
		} `json:"root"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, 0, err
	}
	page := map[string]string{}
	for _, ch := range payload.Root.Children {
		if ch.Fields.ChunkID != "" {
			page[ch.Fields.ChunkID] = ch.Fields.ChunkHash
		}
	}
	return page, payload.Root.Fields.TotalCount, nil
}

// UpsertChunk POSTs JSON to the knowledge_chunk document id derived from ParentID and SequenceIndex.
func (c *Client) UpsertChunk(ctx context.Context, chunk ChunkRecord) error {
	doc := chunkDocument{}
	doc.Fields.ParentRef = chunk.ParentRef
	doc.Fields.SequenceIndex = chunk.SequenceIndex
	doc.Fields.ChunkID = chunk.ChunkID
	doc.Fields.ChunkHash = chunk.ChunkHash
	doc.Fields.IndexedAt = chunk.IndexedAt
	doc.Fields.TextContent = chunk.Text
	doc.Fields.Embedding = chunk.Embedding
	payload, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if err := c.postJSON(ctx, c.childDocURL(chunk.ParentID, chunk.SequenceIndex), payload); err != nil {
		return err
	}
	return nil
}

// DeleteChunk DELETEs /document/v1/{namespace}/knowledge_chunk/docid/{chunkID}; 404 is treated success.
func (c *Client) DeleteChunk(ctx context.Context, chunkID string) error {
	deleteURL := fmt.Sprintf("%s/document/v1/%s/knowledge_chunk/docid/%s", c.Host, c.Namespace, url.QueryEscape(chunkID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vespa delete chunk failed: %s: %s", resp.Status, string(body))
	}
	return nil
}

// postJSON sends Content-Type application/json POST and errors on non-2xx (used for document upserts).
func (c *Client) postJSON(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vespa request failed: %s: %s", resp.Status, string(respBody))
	}
	return nil
}

// parentDocURL builds GET/POST URL for the source_metadata document with id parentID.
func (c *Client) parentDocURL(parentID string) string {
	return fmt.Sprintf("%s/document/v1/%s/source_metadata/docid/%s", c.Host, c.Namespace, parentID)
}

// childDocURL builds the knowledge_chunk doc URL; doc id encodes parentID#chunk_<sequenceIndex> (URL-escaped #).
func (c *Client) childDocURL(parentID string, sequenceIndex int) string {
	return fmt.Sprintf("%s/document/v1/%s/knowledge_chunk/docid/%s%%23chunk_%d", c.Host, c.Namespace, parentID, sequenceIndex)
}

// parentRef returns the Vespa internal reference string stored on chunks (id:namespace:source_metadata::id).
func (c *Client) parentRef(parentID string) string {
	return fmt.Sprintf("id:%s:source_metadata::%s", c.Namespace, parentID)
}
