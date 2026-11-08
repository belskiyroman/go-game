package app

import (
	"path/filepath"
	"testing"
	"time"

	"gogame/internal/rules"
)

func TestGamesSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	sessions := NewSessionStore(filepath.Join(dir, "sessions.json"))
	results := NewResultStore(filepath.Join(dir, "results.json"))
	gamesPath := filepath.Join(dir, "games.json")
	a, b := sessions.Login("", "Alpha"), sessions.Login("", "Beta")

	h := NewHub(sessions, results, gamesPath)
	h.mu.Lock()
	h.startGame(&Invite{From: a.ID, To: b.ID, Settings: DefaultSettings()})
	g := h.playerGame[a.ID]
	now := time.Now()
	black := g.Turn
	if _, err := g.Play(black, 3, 3, now); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Play(black.Opponent(), 15, 15, now); err != nil {
		t.Fatal(err)
	}
	g.Deadline = now.Add(42 * time.Second) // black has 42 s left
	h.mu.Unlock()
	h.SaveGames()

	time.Sleep(50 * time.Millisecond) // "downtime": the clock must not run
	h2 := NewHub(sessions, results, gamesPath)
	r := h2.playerGame[b.ID]
	if r == nil || r != h2.playerGame[a.ID] || r.ID != g.ID {
		t.Fatalf("game not restored for both players: %+v", r)
	}
	if r.Board.String() != g.Board.String() || r.Turn != rules.Black || len(r.Moves) != 2 {
		t.Fatalf("board/turn/moves differ:\n%s\nturn %v moves %d", r.Board, r.Turn, len(r.Moves))
	}
	left := time.Until(r.Deadline)
	if left < 41*time.Second || left > 42*time.Second {
		t.Fatalf("clock left %v, want about 42s", left)
	}
	// the restored game plays on, with the rules state (ko point) intact
	if _, err := r.Play(rules.Black, 4, 4, time.Now()); err != nil {
		t.Fatal(err)
	}
}
