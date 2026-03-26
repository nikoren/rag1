package vespa

const (
	// defaultChunkStatePageSize is used when Client.PageSize is zero (Vespa search hits per request).
	defaultChunkStatePageSize = 200
	// maxChunkStatePageSize caps PageSize to stay under typical Vespa max hits limits per query.
	maxChunkStatePageSize = 400
)

// normalizePageSize returns a positive page size in [defaultChunkStatePageSize, maxChunkStatePageSize].
func normalizePageSize(size int) int {
	if size <= 0 {
		return defaultChunkStatePageSize
	}
	if size > maxChunkStatePageSize {
		return maxChunkStatePageSize
	}
	return size
}
