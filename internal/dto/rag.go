// Package dto holds shared RAG data types (no HTTP or storage dependencies).
package dto

// ContextChunk is one retrieved passage for prompting and citations.
type ContextChunk struct {
	ID            string
	Text          string
	SourceURL     string
	SourceType    string
	Relevance     float64
	SequenceIndex int
}

// RetrievalInput selects how many hits to fetch and optional book filter (url substring).
type RetrievalInput struct {
	Query     string
	Hits      int
	FilterURL string
}

// GenerateInput is the LLM prompt payload: user question plus retrieved context.
type GenerateInput struct {
	Question     string
	Chunks       []ContextChunk
	SystemPrompt string // If empty, the generator uses a default RAG system prompt.
}

// GenerateOutput is the assistant reply from the chat model.
type GenerateOutput struct {
	Text  string
	Model string // Echo of model id when returned by the API.
}
