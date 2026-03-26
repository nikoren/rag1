package pdfsource

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rag1/internal/source"
)

func writeTinyPDF(t *testing.T, dir string, text string) string {
	t.Helper()

	var buf bytes.Buffer
	write := func(s string) {
		_, _ = buf.WriteString(s)
	}

	write("%PDF-1.4\n")

	type obj struct {
		num int
		s   string
	}

	objects := []obj{
		{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>"},
		{5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"},
	}

	// Content stream (object 4) depends on text length.
	stream := fmt.Sprintf("BT\n/F1 24 Tf\n72 72 Td\n(%s) Tj\nET\n", text)
	objects = append(objects[:3], append([]obj{
		{4, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream)},
	}, objects[3:]...)...)

	offsets := make([]int, 6) // 0..5
	for _, o := range objects {
		offsets[o.num] = buf.Len()
		write(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", o.num, o.s))
	}

	xrefOffset := buf.Len()
	write("xref\n")
	write("0 6\n")
	write("0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		write(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	write("trailer\n")
	write("<< /Size 6 /Root 1 0 R >>\n")
	write("startxref\n")
	write(fmt.Sprintf("%d\n", xrefOffset))
	write("%%EOF\n")

	p := filepath.Join(dir, "hello.pdf")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	return p
}

func TestNew_ValidatesPath(t *testing.T) {
	if _, err := New("", source.SourceMetadata{}); err == nil {
		t.Fatal("expected error for empty path")
	}
	if _, err := New("/path/does/not/exist.pdf", source.SourceMetadata{}); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestNew_DefaultsMetadata(t *testing.T) {
	dir := t.TempDir()
	p := writeTinyPDF(t, dir, "Hello PDF")
	s, err := New(p, source.SourceMetadata{})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	md := s.GetMetadata()
	if md.Type != "pdf" {
		t.Fatalf("expected Type=pdf, got %q", md.Type)
	}
	if md.URL == "" {
		t.Fatal("expected URL to be set")
	}
	if md.Timestamp == 0 {
		t.Fatal("expected Timestamp to be set")
	}
	if md.ID == "" {
		t.Fatal("expected ID to be set")
	}
}

func TestGetReader_ReturnsText(t *testing.T) {
	dir := t.TempDir()
	p := writeTinyPDF(t, dir, "Hello PDF")
	s, err := New(p, source.SourceMetadata{})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	b, err := io.ReadAll(s.GetReader())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		t.Fatal("expected non-empty extracted text")
	}
}

