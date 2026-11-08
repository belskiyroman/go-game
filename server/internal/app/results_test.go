package app

import (
	"path/filepath"
	"testing"
)

func TestHeadToHead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	rs := NewResultStore(path)
	rs.Add("belskiyroman", "Romanos")
	rs.Add("Belskiyroman", "romanos") // nicknames are case-insensitive
	rs.Add("Romanos", "belskiyroman")
	if a, b := rs.Score("BELSKIYROMAN", "Romanos"); a != 2 || b != 1 {
		t.Fatalf("score %d:%d, want 2:1", a, b)
	}
	// kept between restarts
	if a, b := NewResultStore(path).Score("Romanos", "belskiyroman"); a != 1 || b != 2 {
		t.Fatalf("after reload %d:%d, want 1:2", a, b)
	}
	if got := ResultsPath("./data/test-sessions.json"); got != "data/test-results.json" {
		t.Fatalf("results path %q", got)
	}
}
