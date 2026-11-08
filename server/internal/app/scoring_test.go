package app

import (
	"testing"
	"time"

	"gogame/internal/rules"
)

// countingGame: 9×9, a black wall on column 4, a white wall on column 5, and a lone white stone
// at 1,1 inside black's side. The players are counting; nothing is marked dead yet.
func countingGame(t *testing.T) *Game {
	g := NewGame(Settings{Size: 9, Komi: 6.5, MoveTime: 60},
		&Session{ID: "aaaa1111", Nick: "A"}, &Session{ID: "bbbb2222", Nick: "B"})
	for y := 0; y < 9; y++ {
		g.Board.Points[g.Board.Index(4, y)] = rules.Black
		g.Board.Points[g.Board.Index(5, y)] = rules.White
	}
	g.Board.Points[g.Board.Index(1, 1)] = rules.White
	g.Phase, g.Dead = PhaseScoring, map[int]bool{}
	return g
}

func territory(g *Game) (int, int) {
	b, w := rules.Territory(g.Board, g.Dead, g.Neutral)
	return len(b), len(w)
}

func TestToggleDeadRecountsTerritory(t *testing.T) {
	g := countingGame(t)
	now := time.Now()
	if b, w := territory(g); b != 0 || w != 27 {
		t.Fatalf("before: black %d white %d, want 0 and 27 (the white stone spoils black's side)", b, w)
	}
	g.Agree[rules.Black] = true
	if err := g.ToggleDead(1, 1, now); err != nil {
		t.Fatal(err)
	}
	if b, _ := territory(g); b != 36 {
		t.Fatalf("white stone dead: black territory %d, want 36", b)
	}
	if g.Agree[rules.Black] {
		t.Fatal("a change must ask both players to agree again")
	}
	if err := g.ToggleDead(1, 1, now); err != nil || g.Dead[g.Board.Index(1, 1)] {
		t.Fatal("a second click must bring the group back to life")
	}
	if err := g.ToggleDead(0, 0, now); err == nil {
		t.Fatal("an empty point is not a group")
	}
}

func TestToggleNeutralDoesNotRecount(t *testing.T) {
	g := countingGame(t)
	now := time.Now()
	if err := g.ToggleNeutral(0, 0, now); err == nil {
		t.Fatal("a region bordered by both colors is already neutral")
	}
	_ = g.ToggleDead(1, 1, now)
	// black's side, with the dead stone's point, becomes neutral as a whole and comes back
	if err := g.ToggleNeutral(0, 0, now); err != nil {
		t.Fatal(err)
	}
	if b, w := territory(g); b != 0 || w != 27 || !g.Neutral[g.Board.Index(1, 1)] {
		t.Fatalf("neutral: black %d white %d", b, w)
	}
	// marking a group alive or dead keeps the players' neutral marks
	_ = g.ToggleDead(5, 0, now) // white wall dead: the whole board is black's
	if b, _ := territory(g); b != 9*9-9-36 {
		t.Fatalf("after the white wall died: black %d, want %d (the neutral side stays neutral)", b, 9*9-9-36)
	}
	_ = g.ToggleDead(5, 0, now)
	if err := g.ToggleNeutral(2, 2, now); err != nil {
		t.Fatal(err)
	}
	if b, _ := territory(g); b != 36 || len(g.Neutral) != 0 {
		t.Fatalf("back as counted: black %d, neutral %v", b, g.Neutral)
	}

	// the final score uses the players' marks
	_ = g.ToggleNeutral(2, 2, now)
	g.AgreeScore(rules.Black)
	g.AgreeScore(rules.White)
	if g.Result == nil || g.Result.Score.BlackTerritory != 0 || g.Result.Score.WhiteTerritory != 27 || g.Result.Score.BlackPrisoners != 1 {
		t.Fatalf("result: %+v", g.Result.Score)
	}
}
