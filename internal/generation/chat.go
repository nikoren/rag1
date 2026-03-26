// Package generation calls OpenAI-compatible chat completion APIs (e.g. LM Studio POST /v1/chat/completions).
package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"rag1/internal/dto"
)

const defaultSystemPrompt = `You are a helpful assistant answering questions using only the provided context excerpts from indexed documents.
If the context does not contain enough information, say so clearly. When you use facts, cite the chunk id or source when present in the excerpt headers.`

// OpenAIChatClient implements Generator-style usage via POST {BaseURL}/chat/completions.
type OpenAIChatClient struct {
	HTTPClient   *http.Client
	BaseURL      string // e.g. http://127.0.0.1:1234/v1 (no trailing slash)
	Model        string
	Temperature  float64
	MaxTokens    int
	Logger       *slog.Logger
}

type chatRequest struct {
	Model       string              `json:"model"`
	Messages    []chatMessage       `json:"messages"`
	Temperature float64             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Generate builds system + user messages from in and POSTs to the chat completions endpoint.
func (c *OpenAIChatClient) Generate(ctx context.Context, in dto.GenerateInput) (dto.GenerateOutput, error) {
	if strings.TrimSpace(in.Question) == "" {
		return dto.GenerateOutput{}, fmt.Errorf("generation: empty question")
	}
	sys := in.SystemPrompt
	if strings.TrimSpace(sys) == "" {
		sys = defaultSystemPrompt
	}
	user := buildUserContent(in.Question, in.Chunks)

	body, err := json.Marshal(chatRequest{
		Model:       c.Model,
		Messages:    []chatMessage{{Role: "system", Content: sys}, {Role: "user", Content: user}},
		Temperature: c.Temperature,
		MaxTokens:   c.MaxTokens,
	})
	if err != nil {
		return dto.GenerateOutput{}, err
	}

	url := strings.TrimSuffix(c.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return dto.GenerateOutput{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return dto.GenerateOutput{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return dto.GenerateOutput{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return dto.GenerateOutput{}, fmt.Errorf("chat completion failed: %s: %s", resp.Status, string(respBody))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return dto.GenerateOutput{}, fmt.Errorf("decode chat response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return dto.GenerateOutput{}, fmt.Errorf("chat completion: no choices in response")
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if c.Logger != nil {
		c.Logger.Debug("chat completion", "chars", len(text))
	}
	out := dto.GenerateOutput{Text: text, Model: parsed.Model}
	if out.Model == "" {
		out.Model = c.Model
	}
	return out, nil
}

func buildUserContent(question string, chunks []dto.ContextChunk) string {
	var b strings.Builder
	b.WriteString("Context excerpts:\n\n")
	for i, ch := range chunks {
		b.WriteString(fmt.Sprintf("--- Excerpt %d", i+1))
		if ch.ID != "" {
			b.WriteString(fmt.Sprintf(" [chunk_id=%s]", ch.ID))
		}
		if ch.SourceURL != "" {
			b.WriteString(fmt.Sprintf(" [source=%s]", ch.SourceURL))
		}
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(ch.Text))
		b.WriteString("\n\n")
	}
	b.WriteString("Question: ")
	b.WriteString(question)
	return b.String()
}
