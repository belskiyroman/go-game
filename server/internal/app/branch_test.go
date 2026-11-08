package app

import (
	"path/filepath"
	"testing"
	"time"

	"gogame/internal/rules"
)

// testHub: two signed-in players with an open connection each (messages to them are dropped).
func testHub(t *testing.T, gamesPath string) (*Hub, *Session, *Session) {
	dir := t.TempDir()
	sessions := NewSessionStore(filepath.Join(dir, "sessions.json"))
	results := NewResultStore(filepath.Join(dir, "results.json"))
	a, b := sessions.Login("", "Alpha"), sessions.Login("", "Beta")
	h := NewHub(sessions, results, gamesPath)
	for _, s := range []*Session{a, b} {
		h.conns[s.ID] = map[*Conn]bool{{out: make(chan []byte)}: true}
	}
	return h, a, b
}

// playOpening: black 3-3, white 15-15, black 4-4 and so on.
func playOpening(t *testing.T, g *Game, n int) {
	pts := [][2]int{{3, 3}, {15, 15}, {4, 4}, {14, 14}, {5, 5}, {13, 13}}
	for i := 0; i < n; i++ {
		if _, err := g.Play(g.Turn, pts[i][0], pts[i][1], time.Now()); err != nil {
			t.Fatal(err)
		}
	}
}

func colorOf(h *Hub, g *Game, s *Session) rules.Color { return g.ColorOf(s.ID) }

func TestAlternativeGame(t *testing.T) {
	h, a, b := testHub(t, "")
	h.startGame(&Invite{From: a.ID, To: b.ID, Settings: DefaultSettings()})
	main := h.playerGame[a.ID]
	playOpening(t, main, 5)
	aColor := colorOf(h, main, a)

	// a asks for an alternative, b accepts: both are in the copy, the main game is adjourned
	h.handle(a, inMsg{T: "ask", Kind: ReqAlt})
	if main.Req == nil || main.Req.Kind != ReqAlt {
		t.Fatalf("request not stored: %+v", main.Req)
	}
	h.handle(b, inMsg{T: "askAnswer", Accept: true})
	alt := h.playerGame[a.ID]
	if alt == main || alt != h.playerGame[b.ID] || alt.Phase != PhaseSetup || len(alt.Source) != 5 {
		t.Fatalf("alternative: %+v", alt)
	}
	if main.Phase != PhaseAdjourned || main.Resume != PhasePlay {
		t.Fatalf("main game: phase %s resume %s", main.Phase, main.Resume)
	}

	// b offers move 3 with a color swap; a cannot offer while it waits; a accepts
	h.handle(b, inMsg{T: "propose", Step: 3, Swap: true})
	h.handle(a, inMsg{T: "propose", Step: 1})
	if p := alt.Proposal; p == nil || p.Step != 3 || !p.Swap || p.From != colorOf(h, alt, b) {
		t.Fatalf("proposal: %+v", p)
	}
	h.handle(a, inMsg{T: "proposeAnswer", Accept: true})
	if alt.Phase != PhasePlay || len(alt.Moves) != 3 || alt.From != 3 || alt.Source != nil {
		t.Fatalf("started: phase %s moves %d from %d", alt.Phase, len(alt.Moves), alt.From)
	}
	if alt.Turn != rules.White || alt.Board.Points[alt.Board.Index(4, 4)] != rules.Black || alt.Board.Points[alt.Board.Index(14, 14)] != rules.Empty {
		t.Fatalf("position after 3 moves:\n%s turn %v", alt.Board, alt.Turn)
	}
	if colorOf(h, alt, a) != aColor.Opponent() || alt.Nicks[aColor.Opponent()] != "Alpha" {
		t.Fatalf("colors not swapped: a plays %v", colorOf(h, alt, a))
	}
	if left := time.Until(alt.Deadline); left < 299*time.Second {
		t.Fatalf("the player to move must get the full time, has %v", left)
	}

	// "My games" lists the adjourned main game for both nicknames
	if l := h.myGames("alpha"); len(l) != 1 || l[0] != main {
		t.Fatalf("my games: %v", l)
	}

	// closing the alternative by agreement: not counted, not kept, players are free
	h.handle(b, inMsg{T: "ask", Kind: ReqClose})
	h.handle(a, inMsg{T: "askAnswer", Accept: true})
	if h.playerGame[a.ID] != nil || h.playerGame[b.ID] != nil || h.games[alt.ID] != nil {
		t.Fatal("closed game must be gone")
	}
	if x, y := h.results.Score("Alpha", "Beta"); x+y != 0 {
		t.Fatalf("closed game counted: %d %d", x, y)
	}

	// continue the main game from "My games": an invitation, then the game goes on
	h.handle(a, inMsg{T: "resume", Game: main.ID})
	if inv := h.invites[a.ID]; inv == nil || inv.Game != main.ID || inv.To != b.ID {
		t.Fatalf("continue invitation: %+v", inv)
	}
	h.handle(b, inMsg{T: "acceptInvite", From: a.ID})
	if h.playerGame[a.ID] != main || main.Phase != PhasePlay || len(main.Moves) != 5 {
		t.Fatalf("main not resumed: phase %s", main.Phase)
	}
	if len(h.myGames("alpha")) != 0 {
		t.Fatal("a resumed game is not in My games")
	}
}

func TestAdjournKeepsClock(t *testing.T) {
	h, a, b := testHub(t, "")
	h.startGame(&Invite{From: a.ID, To: b.ID, Settings: DefaultSettings()})
	g := h.playerGame[a.ID]
	playOpening(t, g, 2)
	g.Deadline = time.Now().Add(42 * time.Second)

	h.handle(a, inMsg{T: "ask", Kind: ReqAdjourn})
	h.handle(a, inMsg{T: "askAnswer", Accept: true}) // one's own request cannot be answered
	if g.Phase != PhasePlay {
		t.Fatal("adjourned by the requester alone")
	}
	h.handle(b, inMsg{T: "askAnswer", Accept: true})
	if g.Phase != PhaseAdjourned || h.playerGame[a.ID] != nil || h.busy(b.ID) {
		t.Fatalf("not adjourned: %s", g.Phase)
	}
	time.Sleep(30 * time.Millisecond)
	g.Continue(time.Now())
	if left := time.Until(g.Deadline); left < 41*time.Second || left > 42*time.Second {
		t.Fatalf("clock after continue: %v, want about 42s", left)
	}
}

func TestOpponentAway(t *testing.T) {
	h, a, b := testHub(t, "")
	h.startGame(&Invite{From: a.ID, To: b.ID, Settings: DefaultSettings()})
	g := h.playerGame[a.ID]
	playOpening(t, g, 2)
	// the player to move leaves and runs out of time
	gone := g.Players[g.Turn]
	stay := a
	if gone == a.ID {
		stay = b
	}
	delete(h.conns, gone)
	g.Deadline = time.Now().Add(-time.Second)
	h.armTimer(g)
	time.Sleep(150 * time.Millisecond)
	h.mu.Lock()
	away := g.Away
	h.mu.Unlock()
	if away != g.Turn || len(g.Moves) != 2 {
		t.Fatalf("expected the opponent to decide, away=%v moves=%d", away, len(g.Moves))
	}
	h.handle(stay, inMsg{T: "awayScore"})
	if g.Phase != PhaseResult || g.Result == nil || g.Result.Reason != "score" {
		t.Fatalf("not counted: %s", g.Phase)
	}
	if x, y := h.results.Score("Alpha", "Beta"); x+y != 1 {
		t.Fatalf("counted game must be in the score: %d %d", x, y)
	}
}

func TestAdjournedGamesSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "games.json")
	h, a, b := testHub(t, path)
	h.startGame(&Invite{From: a.ID, To: b.ID, Settings: DefaultSettings()})
	g := h.playerGame[a.ID]
	playOpening(t, g, 3)
	h.handle(a, inMsg{T: "ask", Kind: ReqAlt})
	h.handle(b, inMsg{T: "askAnswer", Accept: true})
	h.handle(a, inMsg{T: "ask", Kind: ReqAdjourn})
	h.handle(b, inMsg{T: "askAnswer", Accept: true}) // the alternative in setup is adjourned too
	h.SaveGames()

	h2 := NewHub(h.sessions, h.results, path)
	l := h2.myGames("Beta")
	if len(l) != 2 {
		t.Fatalf("want 2 adjourned games after restart, got %d", len(l))
	}
	for _, x := range l {
		if x.Parent != "" && (x.Resume != PhaseSetup || len(x.Source) != 3) {
			t.Fatalf("alternative in setup lost its state: %+v", x)
		}
	}
}
