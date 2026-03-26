package textclean

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDefaultSanitizer_RemovesControlChars(t *testing.T) {
	s := DefaultSanitizer{}
	got := s.Clean("a\x00b\x02c")
	if got != "abc" {
		t.Fatalf("unexpected cleaned text: %q", got)
	}
}

func TestDefaultSanitizer_PreservesWhitespaceControls(t *testing.T) {
	s := DefaultSanitizer{}
	input := "line1\nline2\tcol2\r\nline3"
	got := s.Clean(input)
	if got != input {
		t.Fatalf("expected whitespace controls to survive, got %q", got)
	}
}

func TestDefaultSanitizer_DropsInvalidAndFormatting(t *testing.T) {
	s := DefaultSanitizer{}
	invalid := string([]byte{0xff, 0xfe, 'a'})
	got := s.Clean(invalid + "\u200b")

	if !utf8.ValidString(got) {
		t.Fatalf("expected valid utf-8 output, got %q", got)
	}
	if strings.Contains(got, "\u200b") {
		t.Fatalf("expected zero-width space removed, got %q", got)
	}
	if !strings.Contains(got, "a") {
		t.Fatalf("expected readable text retained, got %q", got)
	}
}
