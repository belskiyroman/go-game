package app

import (
	"fmt"
	"math"
	"strings"
	"time"

	"gogame/internal/rules"
)

type Settings struct {
	Size     int     `json:"size"`
	Komi     float64 `json:"komi"`
	MoveTime int     `json:"moveTime"` // seconds per move
}

var (
	allowedSizes = map[int]bool{9: true, 13: true, 15: true, 19: true}
	allowedKomi  = map[float64]bool{4.5: true, 5.5: true, 6.5: true, 7.5: true}
	allowedTime  = map[int]bool{30: true, 60: true, 120: true, 180: true, 300: true, 600: true}
)

func DefaultSettings() Settings { return Settings{Size: 19, Komi: 6.5, MoveTime: 300} }

func (s Settings) Valid() bool {
	return allowedSizes[s.Size] && allowedKomi[s.Komi] && allowedTime[s.MoveTime]
}

type Move struct {
	Color rules.Color
	X, Y  int
	Pass  bool
}

type Result struct {
	Winner string       `json:"winner"` // "b" | "w"
	Reason string       `json:"reason"` // "score" | "resign"
	Score  *rules.Score `json:"score,omitempty"`
	Diff   float64      `json:"diff"`
}

// Game phases.
const (
	PhasePlay      = "play"
	PhasePaused    = "paused"
	PhaseScoring   = "scoring"
	PhaseResult    = "result"
	PhaseSetup     = "setup"     // an alternative game before it starts: the players choose the start move
	PhaseAdjourned = "adjourned" // frozen until both players agree to continue it from "My games"
)

type Game struct {
	ID       string
	Settings Settings
	Players  [3]string // indexed by rules.Color: [1] black, [2] white (session ids)
	Nicks    [3]string
	Tags     [3]string
	Board    *rules.Board
	Turn     rules.Color
	Caps     [3]int // stones captured during play, by color
	Moves    []Move
	Last     int // last stone placed, or -1
	Passes   int
	Phase    string

	PauseReq rules.Color // who asked for a pause and waits for an answer, or Empty
	Ready    [3]bool     // during a pause: "ready to continue"

	Deadline  time.Time     // when the current player's per-move time runs out
	Remaining time.Duration // frozen time while paused

	// game clock: total playing time without pauses
	Played  time.Duration // accumulated while the clock was stopped
	Running time.Time     // when the clock last started; zero while stopped

	// ClockLeft is the current player's time left when the game was saved: the clocks
	// stand still while the server restarts
	ClockLeft time.Duration

	Dead  map[int]bool
	Agree [3]bool

	Result   *Result
	Recorded bool // the result was counted in the head-to-head score

	// alternative games: a copy of another game that starts from a chosen move
	Parent   string    // the game this one was copied from ("" for a game started from an invitation)
	From     int       // the move it started from
	Source   []Move    // setup: the moves of the parent game to choose the start move from
	Proposal *Proposal // setup: the start move one player offered

	Req         *Request    // a request waiting for the opponent's answer: alternative, adjourn or close
	Resume      string      // adjourned: the phase to return to
	AdjournedAt time.Time   // when the game was adjourned
	Away        rules.Color // the player to move went offline and ran out of time: the other one decides
	Seq         int         // increases with every change; clients use it to order events

	timer *time.Timer
}

func NewGame(s Settings, black, white *Session) *Game {
	g := &Game{
		ID: randomHex(6), Settings: s,
		Board: rules.NewBoard(s.Size), Turn: rules.Black, Last: -1, Phase: PhasePlay,
	}
	g.Players[rules.Black], g.Players[rules.White] = black.ID, white.ID
	g.Nicks[rules.Black], g.Nicks[rules.White] = black.Nick, white.Nick
	g.Tags[rules.Black], g.Tags[rules.White] = black.Tag(), white.Tag()
	now := time.Now()
	g.Deadline = now.Add(g.moveTime())
	g.Running = now
	return g
}

// PlayedTime is the game time so far, pauses not included.
func (g *Game) PlayedTime(now time.Time) time.Duration {
	if g.Running.IsZero() {
		return g.Played
	}
	return g.Played + now.Sub(g.Running)
}

func (g *Game) stopClock(now time.Time) {
	g.Played = g.PlayedTime(now)
	g.Running = time.Time{}
}

func (g *Game) moveTime() time.Duration { return time.Duration(g.Settings.MoveTime) * time.Second }

// ColorOf returns the color the session plays, or Empty.
func (g *Game) ColorOf(sessionID string) rules.Color {
	if g.Players[rules.Black] == sessionID {
		return rules.Black
	}
	if g.Players[rules.White] == sessionID {
		return rules.White
	}
	return rules.Empty
}

// Active: the game is being played (setup included); finished and adjourned games are not.
func (g *Game) Active() bool { return g.Phase != PhaseResult && g.Phase != PhaseAdjourned }

// Play places a stone. Returns the captured points.
func (g *Game) Play(c rules.Color, x, y int, now time.Time) ([]int, error) {
	if g.Phase != PhasePlay {
		return nil, fmt.Errorf("not playing")
	}
	if c != g.Turn {
		return nil, fmt.Errorf("not your turn")
	}
	captured, err := g.Board.Play(c, x, y)
	if err != nil {
		return nil, err
	}
	g.Caps[c] += len(captured)
	g.Moves = append(g.Moves, Move{Color: c, X: x, Y: y})
	g.Last = g.Board.Index(x, y)
	g.Passes = 0
	g.Turn = c.Opponent()
	g.Deadline = now.Add(g.moveTime())
	g.PauseReq = rules.Empty
	g.Seq++
	return captured, nil
}

// Pass: two passes in a row end the play and start counting.
func (g *Game) Pass(c rules.Color, now time.Time) error {
	if g.Phase != PhasePlay || c != g.Turn {
		return fmt.Errorf("cannot pass now")
	}
	g.Board.Pass()
	g.Moves = append(g.Moves, Move{Color: c, Pass: true})
	g.Passes++
	g.Turn = c.Opponent()
	g.PauseReq = rules.Empty
	g.Deadline = now.Add(g.moveTime())
	if g.Passes >= 2 {
		g.startScoring(now)
	}
	g.Seq++
	return nil
}

func (g *Game) startScoring(now time.Time) {
	g.Phase = PhaseScoring
	g.Dead = rules.EstimateDead(g.Board)
	g.Agree = [3]bool{}
	g.Deadline = now.Add(g.moveTime())
}

// Agree with the server's marking of dead stones; when both agree the game ends.
func (g *Game) AgreeScore(c rules.Color) {
	if g.Phase != PhaseScoring {
		return
	}
	g.Agree[c] = true
	if g.Agree[rules.Black] && g.Agree[rules.White] {
		g.finishByScore()
	}
	g.Seq++
}

// PlayOn: the game continues from where it stopped.
func (g *Game) PlayOn(now time.Time) {
	if g.Phase != PhaseScoring {
		return
	}
	g.Phase = PhasePlay
	g.Passes = 0
	g.Dead = nil
	g.Agree = [3]bool{}
	g.Deadline = now.Add(g.moveTime())
	g.Seq++
}

func (g *Game) finishByScore() {
	s := rules.Count(g.Board, g.Dead, g.Caps[rules.Black], g.Caps[rules.White], g.Settings.Komi)
	w := "b"
	if s.White > s.Black {
		w = "w"
	}
	g.Result = &Result{Winner: w, Reason: "score", Score: &s, Diff: math.Abs(s.White - s.Black)}
	g.Phase = PhaseResult
	g.Req, g.Away = nil, rules.Empty
	g.stopClock(time.Now())
}

func (g *Game) Resign(c rules.Color) {
	if g.Phase != PhasePlay && g.Phase != PhasePaused && g.Phase != PhaseScoring {
		return
	}
	g.Result = &Result{Winner: c.Opponent().Letter(), Reason: "resign"}
	g.Phase = PhaseResult
	g.Req, g.Away = nil, rules.Empty
	g.stopClock(time.Now())
	g.Seq++
}

// --- pause ---

func (g *Game) RequestPause(c rules.Color) error {
	if g.Phase != PhasePlay || g.PauseReq != rules.Empty {
		return fmt.Errorf("cannot request a pause now")
	}
	g.PauseReq = c
	g.Seq++
	return nil
}

func (g *Game) CancelPause(c rules.Color) {
	if g.PauseReq == c {
		g.PauseReq = rules.Empty
		g.Seq++
	}
}

func (g *Game) AnswerPause(c rules.Color, accept bool, now time.Time) {
	if g.Phase != PhasePlay || g.PauseReq == rules.Empty || g.PauseReq == c {
		return
	}
	g.PauseReq = rules.Empty
	if accept {
		g.Phase = PhasePaused
		g.Ready = [3]bool{}
		g.Remaining = g.Deadline.Sub(now)
		g.stopClock(now)
	}
	g.Seq++
}

func (g *Game) SetReady(c rules.Color, ready bool, now time.Time) {
	if g.Phase != PhasePaused {
		return
	}
	g.Ready[c] = ready
	if g.Ready[rules.Black] && g.Ready[rules.White] {
		g.Phase = PhasePlay
		g.Deadline = now.Add(g.Remaining)
		g.Running = now
	}
	g.Seq++
}

// Timeout handles the per-move clock running out: an automatic pass, or "agree" while counting.
func (g *Game) Timeout(now time.Time) bool {
	if now.Before(g.Deadline) {
		return false
	}
	switch g.Phase {
	case PhasePlay:
		_ = g.Pass(g.Turn, now)
		return true
	case PhaseScoring:
		changed := false
		for _, c := range []rules.Color{rules.Black, rules.White} {
			if !g.Agree[c] {
				g.Agree[c] = true
				changed = true
			}
		}
		if changed {
			g.finishByScore()
			g.Seq++
		}
		return changed
	}
	return false
}

// SGF exports the game record.
func (g *Game) SGF() string {
	var sb strings.Builder
	sb.WriteString("(;GM[1]FF[4]CA[UTF-8]")
	fmt.Fprintf(&sb, "SZ[%d]KM[%.1f]PB[%s]PW[%s]", g.Settings.Size, g.Settings.Komi, sgfEscape(g.Nicks[rules.Black]), sgfEscape(g.Nicks[rules.White]))
	if g.Result != nil {
		if g.Result.Reason == "resign" {
			fmt.Fprintf(&sb, "RE[%s+R]", strings.ToUpper(g.Result.Winner))
		} else {
			fmt.Fprintf(&sb, "RE[%s+%.1f]", strings.ToUpper(g.Result.Winner), g.Result.Diff)
		}
	}
	for _, m := range g.Moves {
		col := "B"
		if m.Color == rules.White {
			col = "W"
		}
		if m.Pass {
			fmt.Fprintf(&sb, ";%s[]", col)
		} else {
			fmt.Fprintf(&sb, ";%s[%c%c]", col, 'a'+m.X, 'a'+m.Y)
		}
	}
	sb.WriteString(")")
	return sb.String()
}

func sgfEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `]`, `\]`).Replace(s)
}
