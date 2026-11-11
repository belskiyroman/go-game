package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gogame/internal/rules"
)

func TestHistory(t *testing.T) {
	dir := t.TempDir()
	h, a, b := testHub(t, filepath.Join(dir, "games.json"))

	// game 1: adjourned, then continued and won by resignation — one record
	h.startGame(&Invite{From: a.ID, To: b.ID, Settings: DefaultSettings()})
	g1 := h.playerGame[a.ID]
	playOpening(t, g1, 3)
	h.adjournGame(g1, time.Now())
	if l := h.history.Of("alpha"); len(l) != 1 || l[0].Status != HistAdjourned || l[0].Moves != 3 || l[0].SGF != "" {
		t.Fatalf("adjourned: %+v", l)
	}
	g1.Continue(time.Now())
	h.enter(g1, a.ID, b.ID)
	loser := g1.Turn
	h.handleGame(sessionOf(a, b, g1, loser), inMsg{T: "resign"}, time.Now())
	l := h.history.Of("Alpha")
	if len(l) != 1 || l[0].Status != HistFinished || l[0].Result == nil || l[0].Result.Reason != "resign" {
		t.Fatalf("finished: %+v", l)
	}
	winner := loser.Opponent()
	if want := [2]int{0, 0}; true {
		want[winner-1] = 1 // the game just won is in the snapshot of the head-to-head score
		if l[0].H2H != want {
			t.Fatalf("h2h snapshot %v, want %v", l[0].H2H, want)
		}
	}
	full := h.history.Get(g1.ID, "beta")
	if full == nil || !strings.Contains(full.SGF, ";B[dd];W[pp];B[ee]") || !strings.Contains(full.SGF, "RE["+strings.ToUpper(winner.Letter())+"+R]") {
		t.Fatalf("sgf: %+v", full)
	}
	if h.history.Get(g1.ID, "someone") != nil {
		t.Fatal("a stranger must not get the game")
	}
	h.release(a.ID)
	h.release(b.ID)

	// game 2: closed; it is the latest
	time.Sleep(2 * time.Millisecond)
	h.startGame(&Invite{From: a.ID, To: b.ID, Settings: DefaultSettings()})
	g2 := h.playerGame[a.ID]
	playOpening(t, g2, 2)
	h.closeGame(g2)
	l = h.history.Of("beta")
	if len(l) != 2 || l[0].ID != g2.ID || l[0].Status != HistClosed || l[1].ID != g1.ID {
		t.Fatalf("list: %+v", l)
	}

	// the history survives a restart
	h2 := NewHistoryStore(HistoryPath(filepath.Join(dir, "games.json")))
	if l := h2.Of("alpha"); len(l) != 2 {
		t.Fatalf("after restart: %+v", l)
	}
}

func sessionOf(a, b *Session, g *Game, c rules.Color) *Session {
	if g.ColorOf(a.ID) == c {
		return a
	}
	return b
}
