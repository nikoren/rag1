package sourcedetect

import "testing"

func TestDetectPDF(t *testing.T) {
	kind, err := Detect("/tmp/book.PDF")
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if kind != KindPDF {
		t.Fatalf("expected %q, got %q", KindPDF, kind)
	}
}

func TestDetectUnsupported(t *testing.T) {
	if _, err := Detect("/tmp/file.txt"); err == nil {
		t.Fatal("expected unsupported format error")
	}
}

func TestDetectEmptyPath(t *testing.T) {
	if _, err := Detect(""); err == nil {
		t.Fatal("expected empty path error")
	}
}
