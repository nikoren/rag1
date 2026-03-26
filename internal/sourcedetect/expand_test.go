package sourcedetect

import (
	"path/filepath"
	"testing"
)

func TestExpandPathTilde(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	got, err := ExpandPath("~/books/design_data_apps_2.pdf")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/home/testuser", "books", "design_data_apps_2.pdf")
	if got != want {
		t.Fatalf("ExpandPath: got %q, want %q", got, want)
	}
}

func TestExpandPathTildeOnly(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	got, err := ExpandPath("~")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/home/testuser" {
		t.Fatalf("got %q, want /home/testuser", got)
	}
}

func TestExpandPathNoTilde(t *testing.T) {
	got, err := ExpandPath("/abs/foo.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/abs/foo.pdf" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandPathEmpty(t *testing.T) {
	if _, err := ExpandPath(""); err == nil {
		t.Fatal("expected error")
	}
}
