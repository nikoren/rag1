// Command indexer ingests a source file, searches indexed chunks, or runs RAG (--ask: retrieve + LLM answer).
//
// Examples: go run ./cmd/indexer --source ~/books/a.pdf | --search "dashboard" | --ask "What is a transaction?"
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"rag1/internal/chunking"
	"rag1/internal/dto"
	"rag1/internal/embeddings"
	"rag1/internal/generation"
	"rag1/internal/orchestration"
	"rag1/internal/pdfsource"
	"rag1/internal/retrieval"
	"rag1/internal/sourcedetect"
	"rag1/internal/source"
	"rag1/internal/textclean"
	"rag1/internal/vespa"
)

const (
	embeddingURL   = "http://127.0.0.1:1234/v1/embeddings"              // OpenAI-compatible embeddings endpoint (e.g. LM Studio).
	embeddingModel = "text-embedding-all-minilm-l6-v2-embedding"        // Model id; must match server and tensor dims.
	embeddingDims  = 384                                                 // Vector length; must match knowledge_chunk schema.

	vespaHost      = "http://localhost:8080" // Vespa container HTTP API base (document + search).
	vespaNamespace = "slack"                 // Document API namespace segment (from services.xml / schema).

	chatBaseURL = "http://127.0.0.1:1234/v1" // OpenAI-compatible base for chat completions (LM Studio); override with RAG_CHAT_BASE_URL.
	// Default chat model id (LM Studio loaded model). Override with RAG_CHAT_MODEL.
	defaultChatModel = "qwen2.5-coder-14b-instruct"
)

func main() {
	sourcePath := flag.String("source", "", "Path to source file to ingest")
	searchQuery := flag.String("search", "", "Natural language query to search indexed chunks (hybrid vector + text)")
	askQuery := flag.String("ask", "", "RAG: retrieve context then answer with the chat model (LM Studio)")
	searchHits := flag.Int("hits", 10, "Maximum hits for --search or context chunks for --ask")
	filterURL := flag.String("filter-url", "", "Optional substring: restrict --search/--ask to chunks whose book url contains this text")
	showContext := flag.Bool("show-context", false, "When used with --ask, print retrieved context snippets before the answer")
	flag.Parse()

	nModes := 0
	if *sourcePath != "" {
		nModes++
	}
	if *searchQuery != "" {
		nModes++
	}
	if *askQuery != "" {
		nModes++
	}
	if nModes != 1 {
		fmt.Fprintln(os.Stderr, "provide exactly one of: --source <path> | --search <query> | --ask <question>")
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	httpTimeout := 60 * time.Second
	if *searchQuery != "" || *askQuery != "" {
		httpTimeout = 120 * time.Second // Vespa search (60s body) + LLM for --ask
	}
	httpClient := &http.Client{Timeout: httpTimeout}

	if *searchQuery != "" {
		if err := runSearch(context.Background(), logger, httpClient, *searchQuery, *searchHits, *filterURL); err != nil {
			logger.Error("search failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if *askQuery != "" {
		if err := runAsk(context.Background(), logger, httpClient, *askQuery, *searchHits, *filterURL, *showContext); err != nil {
			logger.Error("ask failed", "err", err)
			os.Exit(1)
		}
		return
	}

	sourcePathExpanded, err := sourcedetect.ExpandPath(*sourcePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid source path: %v\n", err)
		os.Exit(2)
	}

	orch := &orchestration.Orchestrator{
		Chunker: chunking.FixedSizeChunker{
			ChunkSize: 500,
		},
		Embedder: &embeddings.Client{
			HTTPClient:   httpClient,
			URL:          embeddingURL,
			Model:        embeddingModel,
			ExpectedDims: embeddingDims,
			Logger:       logger,
		},
		Repo: &vespa.Client{
			HTTPClient: httpClient,
			Host:       vespaHost,
			Namespace:  vespaNamespace,
			Logger:     logger,
			PageSize:   200,
		},
		Sanitize: textclean.DefaultSanitizer{},
		Logger:   logger,
	}

	kind, err := sourcedetect.Detect(sourcePathExpanded)
	if err != nil {
		logger.Error("failed to detect source type", "err", err, "source_path", sourcePathExpanded)
		os.Exit(1)
	}

	var ds source.DataSource
	switch kind {
	case sourcedetect.KindPDF:
		ds, err = pdfsource.New(sourcePathExpanded, source.SourceMetadata{
			Type:   "pdf",
			Author: os.Getenv("PDF_AUTHOR"),
		})
		if err != nil {
			logger.Error("failed to create pdf datasource", "err", err, "source_path", sourcePathExpanded)
			os.Exit(1)
		}
	default:
		logger.Error("unsupported source file format", "kind", kind, "source_path", sourcePathExpanded)
		os.Exit(1)
	}

	if err := orch.Ingest(context.Background(), ds); err != nil {
		logger.Error("ingestion failed", "err", err)
		os.Exit(1)
	}

	logger.Info("ingestion complete")
}

const maxSnippetRunes = 500

func runSearch(ctx context.Context, logger *slog.Logger, httpClient *http.Client, query string, hits int, filterURL string) error {
	emb := &embeddings.Client{
		HTTPClient:   httpClient,
		URL:          embeddingURL,
		Model:        embeddingModel,
		ExpectedDims: embeddingDims,
		Logger:       logger,
	}
	vec, err := emb.Embed(ctx, query)
	if err != nil {
		return fmt.Errorf("embed query: %w", err)
	}

	vc := &vespa.Client{
		HTTPClient: httpClient,
		Host:       vespaHost,
		Namespace:  vespaNamespace,
		Logger:     logger,
		PageSize:   200,
	}

	results, err := vc.Search(ctx, vespa.SearchRequest{
		QueryText:  query,
		UserVector: vec,
		Hits:       hits,
		Offset:     0,
		FilterURL:  filterURL,
	})
	if err != nil {
		return err
	}

	for i, h := range results {
		snippet := h.TextContent
		if len([]rune(snippet)) > maxSnippetRunes {
			r := []rune(snippet)
			snippet = string(r[:maxSnippetRunes]) + "…"
		}
		fmt.Printf("--- #%d relevance=%.4f chunk_id=%s url=%s type=%s seq=%d\n%s\n",
			i+1, h.Relevance, h.ChunkID, h.URL, h.SourceType, h.SequenceIndex, strings.TrimSpace(snippet))
	}
	if len(results) == 0 {
		logger.Info("no matching chunks")
	}
	return nil
}

func chatModelID() string {
	if m := os.Getenv("RAG_CHAT_MODEL"); m != "" {
		return m
	}
	return defaultChatModel
}

func chatBase() string {
	if b := os.Getenv("RAG_CHAT_BASE_URL"); b != "" {
		return strings.TrimSuffix(b, "/")
	}
	return chatBaseURL
}

func runAsk(ctx context.Context, logger *slog.Logger, httpClient *http.Client, query string, hits int, filterURL string, showContext bool) error {
	emb := &embeddings.Client{
		HTTPClient:   httpClient,
		URL:          embeddingURL,
		Model:        embeddingModel,
		ExpectedDims: embeddingDims,
		Logger:       logger,
	}
	vc := &vespa.Client{
		HTTPClient: httpClient,
		Host:       vespaHost,
		Namespace:  vespaNamespace,
		Logger:     logger,
		PageSize:   200,
	}
	ret := &retrieval.VespaRetriever{Embedder: emb, Client: vc}
	gen := &generation.OpenAIChatClient{
		HTTPClient:  httpClient,
		BaseURL:     chatBase(),
		Model:       chatModelID(),
		Temperature: 0.3,
		Logger:      logger,
	}
	if !showContext {
		orch := &orchestration.AnswerOrchestrator{
			Retriever: ret,
			Generator: gen,
			Logger:    logger,
		}
		out, err := orch.Answer(ctx, dto.RetrievalInput{
			Query:     query,
			Hits:      hits,
			FilterURL: filterURL,
		})
		if err != nil {
			return err
		}
		fmt.Println(strings.TrimSpace(out.Text))
		return nil
	}

	chunks, err := ret.Retrieve(ctx, dto.RetrievalInput{
		Query:     query,
		Hits:      hits,
		FilterURL: filterURL,
	})
	if err != nil {
		return err
	}
	fmt.Println("=== Retrieved Context ===")
	for i, ch := range chunks {
		snippet := strings.TrimSpace(ch.Text)
		if len([]rune(snippet)) > maxSnippetRunes {
			r := []rune(snippet)
			snippet = string(r[:maxSnippetRunes]) + "…"
		}
		fmt.Printf("[%d] relevance=%.4f chunk_id=%s source=%s\n%s\n\n", i+1, ch.Relevance, ch.ID, ch.SourceURL, snippet)
	}
	fmt.Println("=== Answer ===")
	out, err := gen.Generate(ctx, dto.GenerateInput{
		Question: query,
		Chunks:   chunks,
	})
	if err != nil {
		return err
	}
	fmt.Println(strings.TrimSpace(out.Text))
	return nil
}
