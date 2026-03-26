package hashutil

import "testing"

func TestHashText_Deterministic(t *testing.T) {
	a := HashText("hello")
	b := HashText("hello")
	if a != b {
		t.Fatalf("expected deterministic hash, got %s != %s", a, b)
	}
}

func TestHashText_DifferentForDifferentInput(t *testing.T) {
	a := HashText("hello")
	b := HashText("world")
	if a == b {
		t.Fatalf("expected different hashes for different input, got %s", a)
	}
}
