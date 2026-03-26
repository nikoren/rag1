package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rag1/internal/dto"
)

func TestOpenAIChatClient_Generate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		var req chatRequest
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
			t.Fatalf("messages: %+v", req.Messages)
		}
		if !strings.Contains(req.Messages[1].Content, "hello") || !strings.Contains(req.Messages[1].Content, "Excerpt") {
			t.Fatalf("user content: %s", req.Messages[1].Content)
		}
		_, _ = w.Write([]byte(`{"model":"test-model","choices":[{"message":{"content":"  Answer text  "}}]}`))
	}))
	defer server.Close()

	c := &OpenAIChatClient{
		HTTPClient: server.Client(),
		BaseURL:    server.URL + "/v1",
		Model:      "m1",
		Temperature: 0.2,
	}
	out, err := c.Generate(context.Background(), dto.GenerateInput{
		Question: "hello?",
		Chunks: []dto.ContextChunk{
			{ID: "c1", Text: "some context", SourceURL: "/a.pdf"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "Answer text" || out.Model != "test-model" {
		t.Fatalf("out %+v", out)
	}
}

func TestOpenAIChatClient_Generate_EmptyQuestion(t *testing.T) {
	c := &OpenAIChatClient{BaseURL: "http://x", Model: "m"}
	_, err := c.Generate(context.Background(), dto.GenerateInput{Question: "  "})
	if err == nil {
		t.Fatal("expected error")
	}
}
