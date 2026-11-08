package app

import (
	"errors"
	"strings"
	"time"

	"gogame/internal/rules"
)

// Requests that need the opponent's consent, alternative games and adjourned games.
//
// An alternative game is a copy of a main game that starts from a move the players agree on.
// Until then it is in the setup phase: each player picks a move and may offer it, with or without
// swapping colors; the game starts when the other player accepts. The main game is adjourned
// meanwhile. An alternative cannot have alternatives of its own; instead the players may go back
// to the main game (the alternative is closed) or make the alternative the main game (the old
// main game is closed). A main game has at most MaxAlts alternatives.

// Request kinds.
const (
	ReqAlt     = "alt"     // play an alternative from this game
	ReqAdjourn = "adjourn" // freeze the game, both go to the room
	ReqClose   = "close"   // end the game without counting; it is not counted in the score
	ReqBack    = "back"    // close this alternative and go back to the main game
	ReqPromote = "promote" // make this alternative the main game; the old main game is closed
)

// MaxAlts is how many alternatives a main game may have at once.
const MaxAlts = 5

type Request struct {
	Kind string
	From rules.Color
}

type Proposal struct {
	From rules.Color
	Step int  // start after this many moves of the source game
	Swap bool // the players swap colors
}

var errNotNow = errors.New("not now")

// Ask sends a request to the opponent. One request at a time.
func (g *Game) Ask(c rules.Color, kind string) error {
	if c == rules.Empty || g.Req != nil {
		return errNotNow
	}
	switch kind {
	case ReqAlt:
		if g.Parent != "" || len(g.Moves) == 0 || !(g.Phase == PhasePlay || g.Phase == PhasePaused || g.Phase == PhaseScoring) {
			return errNotNow
		}
	case ReqAdjourn, ReqClose:
		if !g.Active() {
			return errNotNow
		}
	case ReqBack:
		if g.Parent == "" || !g.Active() {
			return errNotNow
		}
	case ReqPromote:
		if g.Parent == "" || !(g.Phase == PhasePlay || g.Phase == PhasePaused || g.Phase == PhaseScoring) {
			return errNotNow
		}
	default:
		return errNotNow
	}
	g.Req = &Request{Kind: kind, From: c}
	g.Seq++
	return nil
}

func (g *Game) CancelAsk(c rules.Color) {
	if g.Req != nil && g.Req.From == c {
		g.Req = nil
		g.Seq++
	}
}

// Branch makes the alternative game: same players and settings, in the setup phase.
func (g *Game) Branch(now time.Time) *Game {
	return &Game{
		ID: randomHex(6), Settings: g.Settings, Players: g.Players, Nicks: g.Nicks, Tags: g.Tags,
		Board: rules.NewBoard(g.Settings.Size), Turn: rules.Black, Last: -1, Phase: PhaseSetup,
		Parent: g.ID, Source: append([]Move(nil), g.Moves...), Created: now,
	}
}

// replay plays the moves on an empty board.
func replay(size int, moves []Move) (b *rules.Board, caps [3]int, last int, err error) {
	b, last = rules.NewBoard(size), -1
	for _, m := range moves {
		if m.Pass {
			b.Pass()
			last = -1
			continue
		}
		captured, err := b.Play(m.Color, m.X, m.Y)
		if err != nil {
			return nil, caps, -1, err
		}
		caps[m.Color] += len(captured)
		last = b.Index(m.X, m.Y)
	}
	return b, caps, last, nil
}

// Propose offers a start move to the opponent. While it waits, the player cannot offer another.
func (g *Game) Propose(c rules.Color, step int, swap bool) error {
	if g.Phase != PhaseSetup || c == rules.Empty || g.Proposal != nil || step < 0 || step > len(g.Source) {
		return errNotNow
	}
	g.Proposal = &Proposal{From: c, Step: step, Swap: swap}
	g.Seq++
	return nil
}

// DropProposal: the player who offered withdraws it, or the other one declines it.
func (g *Game) DropProposal() {
	if g.Proposal != nil {
		g.Proposal = nil
		g.Seq++
	}
}

// AcceptProposal starts the game from the offered move. The player to move gets the full time.
func (g *Game) AcceptProposal(c rules.Color, now time.Time) error {
	p := g.Proposal
	if g.Phase != PhaseSetup || p == nil || p.From == c || c == rules.Empty {
		return errNotNow
	}
	b, caps, last, err := replay(g.Settings.Size, g.Source[:p.Step])
	if err != nil {
		return err
	}
	g.Board, g.Caps, g.Last = b, caps, last
	g.Moves = append([]Move(nil), g.Source[:p.Step]...)
	g.From, g.Source, g.Proposal = p.Step, nil, nil
	g.Turn = rules.Black
	if p.Step > 0 {
		g.Turn = g.Moves[p.Step-1].Color.Opponent()
	}
	if p.Swap {
		b, w := rules.Black, rules.White
		g.Players[b], g.Players[w] = g.Players[w], g.Players[b]
		g.Nicks[b], g.Nicks[w] = g.Nicks[w], g.Nicks[b]
		g.Tags[b], g.Tags[w] = g.Tags[w], g.Tags[b]
	}
	g.Phase = PhasePlay
	g.Passes = 0
	g.Deadline = now.Add(g.moveTime())
	g.Played, g.Running = 0, now
	g.Seq++
	return nil
}

// Adjourn freezes the game; Continue picks it up where it stopped, clocks included.
func (g *Game) Adjourn(now time.Time) {
	if !g.Active() {
		return
	}
	if g.Phase == PhasePlay || g.Phase == PhaseScoring {
		g.Remaining = g.Deadline.Sub(now)
	}
	if g.Away != rules.Empty || g.Remaining <= 0 {
		g.Remaining = g.moveTime() // the player who ran out of time while away gets a full move
	}
	g.stopClock(now)
	g.Resume, g.Phase = g.Phase, PhaseAdjourned
	g.PauseReq, g.Req, g.Proposal, g.Away = rules.Empty, nil, nil, rules.Empty
	g.Ready, g.Agree = [3]bool{}, [3]bool{}
	g.AdjournedAt = now
	g.Seq++
}

func (g *Game) Continue(now time.Time) {
	if g.Phase != PhaseAdjourned {
		return
	}
	g.Phase, g.Resume = g.Resume, ""
	switch g.Phase {
	case PhasePlay, PhaseScoring:
		g.Deadline = now.Add(g.Remaining)
		g.Running = now
	case PhasePaused, PhaseSetup:
	default:
		g.Phase = PhasePlay
		g.Deadline = now.Add(g.Remaining)
		g.Running = now
	}
	g.Seq++
}

// ScoreAway ends the game by counting the position as it is: the opponent is gone.
func (g *Game) ScoreAway() {
	if g.Away == rules.Empty || g.Phase != PhasePlay {
		return
	}
	g.Away = rules.Empty
	g.Dead = rules.EstimateDead(g.Board)
	g.finishByScore()
	g.Seq++
}

// colorOfNick: games are kept by nickname, since the nickname is the login and outlives sessions.
func (g *Game) colorOfNick(nick string) rules.Color {
	for _, c := range []rules.Color{rules.Black, rules.White} {
		if strings.EqualFold(g.Nicks[c], nick) {
			return c
		}
	}
	return rules.Empty
}

// position is what a game card shows: the current position, or for a game in setup the last
// position of its source.
func (g *Game) position() (board string, last, moves int, turn rules.Color) {
	if len(g.Source) > 0 && (g.Phase == PhaseSetup || g.Resume == PhaseSetup) {
		if b, _, l, err := replay(g.Settings.Size, g.Source); err == nil {
			t := g.Source[len(g.Source)-1].Color.Opponent()
			return b.String(), l, len(g.Source), t
		}
	}
	return g.Board.String(), g.Last, len(g.Moves), g.Turn
}

// encodeMoves sends moves as [color, x, y]; a pass is [color, -1, -1].
func encodeMoves(moves []Move) [][3]int {
	out := make([][3]int, len(moves))
	for i, m := range moves {
		if m.Pass {
			out[i] = [3]int{int(m.Color), -1, -1}
		} else {
			out[i] = [3]int{int(m.Color), m.X, m.Y}
		}
	}
	return out
}
