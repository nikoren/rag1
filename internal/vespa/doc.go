// Package vespa defines chunk records, Repository, and Client for Vespa document/v1 and search HTTP APIs.
//
// Example: Client{Host: "http://localhost:8080", Namespace: "slack"} as orchestration.Repository for indexing,
// or Client.Search with an embedded query for hybrid retrieval over knowledge_chunk.
package vespa
