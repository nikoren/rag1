package source

import "testing"

func TestSourceMetadataFields(t *testing.T) {
	md := SourceMetadata{
		ID:        "id1",
		Type:      "slack",
		Author:    "alice",
		URL:       "https://example.com",
		Timestamp: 123,
	}

	if md.ID == "" || md.Type == "" || md.URL == "" {
		t.Fatal("expected metadata fields to be set")
	}
}
