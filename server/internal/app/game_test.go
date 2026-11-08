package app

import (
	"testing"
	"time"

	"gogame/internal/rules"
)

func TestPlayedTimeSkipsPauses(t *testing.T) {
	g := NewGame(DefaultSettings(), &Session{ID: "aaaa1"}, &Session{ID: "bbbb2"})
	t0 := g.Running

	// 10 s of play, then a 60 s pause, then 5 s of play
	if err := g.RequestPause(rules.Black); err != nil {
		t.Fatal(err)
	}
	g.AnswerPause(rules.White, true, t0.Add(10*time.Second))
	if got := g.PlayedTime(t0.Add(40 * time.Second)); got != 10*time.Second {
		t.Fatalf("during the pause: %v, want 10s", got)
	}
	g.SetReady(rules.Black, true, t0.Add(65*time.Second))
	g.SetReady(rules.White, true, t0.Add(70*time.Second))
	if got := g.PlayedTime(t0.Add(75 * time.Second)); got != 15*time.Second {
		t.Fatalf("after the pause: %v, want 15s", got)
	}
}
