package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	// embeddings
	ollamaBackgroundServicePort          = 11434
	ollamaBackgroundServiceVectorGenPath = "/api/embeddings"

	// vespa storage
	VespaHost      = "http://localhost:8080"
	VespaNamespace = "slack"
	VespaDocType   = "slack_message" // matches the deployed schema
)

// this app builds vespa ingestion pipeline
// Text → Local ML Model (Ollama) → Vector → Vector Database (Vespa).
func main() {
	slackText := "Can someone share the Zoom link for the daily sync?"

	fmt.Println("Generating vector from local Ollama...")

	realVector, err := getRealEmbedding(slackText)
	if err != nil {
		fmt.Println("Failed to get embedding (Is Ollama running?):", err)
		return
	}

	slog.Info("Success! Generated a %d-dimension vector.\n", "len", len(realVector))
	slog.Info("Sending document to Vespa...")

	// 4. Construct the document for Vespa
	vDoc := VespaDocument{}
	vDoc.Fields.ChannelID = "C123_engineering"
	vDoc.Fields.Timestamp = time.Now().Unix()
	vDoc.Fields.Content = slackText
	vDoc.Fields.Embedding = realVector

	// 5. Convert to JSON
	vDocJson, err := json.Marshal(vDoc)
	if err != nil {
		slog.Error("Failed to marshal Vespa document:", err)
		return
	}

	// 6. Send to Vespa's Document API

	// This would change dynamically for each message you process
	uniqueDocumentID := fmt.Sprintf("msg_%d", time.Now().UnixNano())

	// Construct the URL
	vespaURL := fmt.Sprintf("%s/document/v1/%s/%s/docid/%s",
		VespaHost,
		VespaNamespace,
		VespaDocType,
		uniqueDocumentID,
	)

	req, err := http.NewRequest("POST", vespaURL, bytes.NewBuffer(vDocJson))
	if err != nil {
		panic("Failed to create a new request")
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error communicating with Vespa:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Vespa Ingestion Status:", resp.Status) // Should print "200 OK"
}

// 1. The payload format Vespa expects for ingestion
type VespaDocument struct {
	Fields struct {
		ChannelID string    `json:"channel_id"`
		Timestamp int64     `json:"timestamp"`
		Content   string    `json:"content"`
		Embedding []float32 `json:"embedding"`
	} `json:"fields"`
}

// 2. The response structure from Ollama
type EmbeddingResult struct {
	Embedding []float32 `json:"embedding"`
}

// 3. Get a real vector from local Ollama
func getRealEmbedding(text string) ([]float32, error) {
	embedGenUrl := fmt.Sprintf("http://localhost:%d%s", ollamaBackgroundServicePort, ollamaBackgroundServiceVectorGenPath)

	payload := map[string]string{
		"model":  "all-minilm", // The local model you pulled via 'ollama pull all-minilm'
		"prompt": text,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		slog.Error("Failed to marshal embedding payload from text", "err", err)
		return nil, err
	}

	req, err := http.NewRequest("POST", embedGenUrl, bytes.NewBuffer(payloadBytes))
	if err != nil {
		slog.Error("Failed to create a new request", "err", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	ollamaHttpResp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer ollamaHttpResp.Body.Close()

	var ollamaResult EmbeddingResult
	httpBody, err := io.ReadAll(ollamaHttpResp.Body)
	if err != nil {
		slog.Error("Failed to read embedding  response body", "err", err)
		return nil, err
	}
	json.Unmarshal(httpBody, &ollamaResult)

	return ollamaResult.Embedding, nil
}

/*
http://localhost:8080/document/v1/slack/slack_message/docid/msg_001
|___________________| |_________| |___| |___________| |___| |_____|
       Host               API   VespaNamespace  Doc Type  Keyword Unique ID


*/
