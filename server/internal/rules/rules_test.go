package rules

import "testing"

func play(t *testing.T, b *Board, c Color, x, y int) []int {
	t.Helper()
	caps, err := b.Play(c, x, y)
	if err != nil {
		t.Fatalf("play %v at %d,%d: %v", c, x, y, err)
	}
	return caps
}

func TestCaptureSingleStone(t *testing.T) {
	b := NewBoard(9)
	play(t, b, White, 4, 4)
	play(t, b, Black, 3, 4)
	play(t, b, Black, 5, 4)
	play(t, b, Black, 4, 3)
	caps := play(t, b, Black, 4, 5)
	if len(caps) != 1 || caps[0] != b.Index(4, 4) {
		t.Fatalf("expected white at 4,4 to be captured, got %v", caps)
	}
	if b.Points[b.Index(4, 4)] != Empty {
		t.Fatal("captured point is not empty")
	}
}

func TestCaptureGroupInCorner(t *testing.T) {
	b := NewBoard(9)
	play(t, b, White, 0, 0)
	play(t, b, White, 1, 0)
	play(t, b, Black, 2, 0)
	play(t, b, Black, 0, 1)
	caps := play(t, b, Black, 1, 1)
	if len(caps) != 2 {
		t.Fatalf("expected 2 captured stones, got %v", caps)
	}
}

func TestSuicideIsIllegal(t *testing.T) {
	b := NewBoard(9)
	play(t, b, Black, 1, 0)
	play(t, b, Black, 0, 1)
	if _, err := b.Play(White, 0, 0); err != ErrSuicide {
		t.Fatalf("expected suicide error, got %v", err)
	}
	if b.Points[0] != Empty {
		t.Fatal("suicide stone left on board")
	}
}

func TestCaptureBeatsSuicide(t *testing.T) {
	// a move with no liberties is legal when it captures
	b := NewBoard(9)
	play(t, b, Black, 1, 0)
	play(t, b, White, 2, 0)
	play(t, b, White, 1, 1)
	play(t, b, Black, 0, 1)
	caps := play(t, b, White, 0, 0) // white takes black at 1,0's last liberty? no — captures 1,0
	if len(caps) != 1 || caps[0] != b.Index(1, 0) {
		t.Fatalf("expected black 1,0 captured, got %v", caps)
	}
}

func TestKo(t *testing.T) {
	b := NewBoard(9)
	// classic ko shape
	play(t, b, Black, 1, 0)
	play(t, b, Black, 0, 1)
	play(t, b, Black, 1, 2)
	play(t, b, White, 2, 0)
	play(t, b, White, 3, 1)
	play(t, b, White, 2, 2)
	play(t, b, White, 1, 1)
	caps := play(t, b, Black, 2, 1) // black captures white 1,1
	if len(caps) != 1 {
		t.Fatalf("expected ko capture, got %v", caps)
	}
	if _, err := b.Play(White, 1, 1); err != ErrKo {
		t.Fatalf("expected ko error on immediate recapture, got %v", err)
	}
	b.Pass()
	if _, err := b.Play(White, 1, 1); err != nil {
		t.Fatalf("recapture after a pass should be legal: %v", err)
	}
}

func TestCountTerritoryAndDead(t *testing.T) {
	// 9x9: black wall on column 3, white on column 5 (top) and 4 (bottom)
	b := NewBoard(9)
	for y := 0; y < 9; y++ {
		play(t, b, Black, 3, y)
	}
	for y := 0; y < 5; y++ {
		play(t, b, White, 5, y)
	}
	for y := 5; y < 9; y++ {
		play(t, b, White, 4, y)
	}
	play(t, b, White, 1, 3) // dead white stone in black's area
	play(t, b, Black, 7, 6) // dead black stone in white's area
	dead := map[int]bool{b.Index(1, 3): true, b.Index(7, 6): true}
	s := Count(b, dead, 3, 2, 6.5)
	if s.BlackTerritory != 27 || s.WhiteTerritory != 31 {
		t.Fatalf("territory b=%d w=%d", s.BlackTerritory, s.WhiteTerritory)
	}
	if s.Black != 31 || s.White != 40.5 {
		t.Fatalf("score b=%v w=%v", s.Black, s.White)
	}
}

func TestEstimateDeadFindsLoneStones(t *testing.T) {
	b := NewBoard(9)
	for y := 0; y < 9; y++ {
		play(t, b, Black, 3, y)
	}
	for y := 0; y < 9; y++ {
		play(t, b, White, 5, y)
	}
	play(t, b, White, 1, 4) // lone white stone deep in black's area
	dead := EstimateDead(b)
	if !dead[b.Index(1, 4)] {
		t.Fatal("lone white stone should be estimated dead")
	}
	if dead[b.Index(3, 4)] || dead[b.Index(5, 4)] {
		t.Fatal("walls should be alive")
	}
}
