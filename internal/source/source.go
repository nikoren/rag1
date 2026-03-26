// Package source defines source metadata and the DataSource abstraction for ingestion.
//
// Example: a PDF adapter implements DataSource; the orchestrator calls GetMetadata then streams GetReader for chunking.
package source

import "io"

// SourceMode controls incremental behavior: bounded sources can delete stale chunks; unbounded sources do not.
type SourceMode string

const (
	// SourceModeBounded means the source has a complete snapshot (e.g. a file); ingestion may remove chunks no longer present.
	SourceModeBounded SourceMode = "bounded"
	// SourceModeUnbounded means an open-ended stream (e.g. live feed); stale chunk deletion is skipped.
	SourceModeUnbounded SourceMode = "unbounded"
)

// SourceMetadata describes a document for Vespa parent docs and idempotency (hash, mode).
//
// Example: set SourceHash from file content hash and Mode to SourceModeBounded so re-runs skip unchanged sources.
type SourceMetadata struct {
	ID         string     // Stable id for this source (e.g. pdf_<hash>); used as Vespa document id base.
	Type       string     // Logical type label (e.g. "pdf") for filtering and display.
	Author     string     // Optional provenance string (e.g. from env or catalog).
	URL        string     // Canonical location or file path string stored on the parent document.
	Timestamp  int64      // Unix seconds for versioning; defaults may be filled by adapters.
	SourceHash string     // Hash of full source text or bytes; unchanged hash skips re-ingestion when bounded.
	Mode       SourceMode // SourceModeBounded or SourceModeUnbounded; empty is treated as bounded in orchestration.
}

// SourceState is the result of looking up an existing parent document by ID (for skip-if-unchanged).
//
// Example: Found true and SourceHash matching current metadata means ingestion can return early.
type SourceState struct {
	Found      bool   // Whether a parent document exists in Vespa.
	SourceHash string // Stored hash from the last indexed version; compared to incoming metadata.
}

// DataSource is anything that can provide metadata and a text stream for chunking (PDF, future formats).
//
// Example: pdfsource.Source returns metadata from the file and a Reader over extracted plain text.
type DataSource interface {
	GetMetadata() SourceMetadata
	GetReader() io.Reader
}
