package app

import (
	"fmt"
	"math"
	"strings"
	"time"

	"gogame/internal/rules"
)

// Animation time the clients need after a move, before the next move is accepted and the
// per-move clock starts. Mirrors the client's placement and capture animations: the game is
// meant to be slow, and the time spent watching must not eat into a player's clock.
const (
	placeAnim      = 3300 * time.Millisecond
	captureStart   = 2000 * time.Millisecond // capture chain starts when the new stone separates
	captureLayer   = 300 * time.Millisecond
	captureStone   = 2400 * time.Millisecond
	inputTolerance = 300 * time.Millisecond
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
	PhasePlay    = "play"
	PhasePaused  = "paused"
	PhaseScoring = "scoring"
	PhaseResult  = "result"
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

	Deadline    time.Time     // when the current player's per-move time runs out
	Remaining   time.Duration // frozen time while paused
	InputOpenAt time.Time     // moves before this are refused: animations are still running

	Dead  map[int]bool
	Agree [3]bool

	Result *Result
	Seq    int // increases with every change; clients use it to order events

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
	g.InputOpenAt = now
	g.Deadline = now.Add(g.moveTime())
	return g
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

func (g *Game) Active() bool { return g.Phase != PhaseResult }

// captureLayers: how many links the capture chain has (BFS distance from the capturing move).
func captureLayers(b *rules.Board, move int, captured []int) int {
	if len(captured) == 0 {
		return 0
	}
	in := map[int]bool{}
	for _, c := range captured {
		in[c] = true
	}
	depth := map[int]int{}
	queue := []int{}
	for _, q := range b.Neighbors(move) {
		if in[q] {
			depth[q] = 0
			queue = append(queue, q)
		}
	}
	maxD := 0
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, q := range b.Neighbors(p) {
			if in[q] {
				if _, ok := depth[q]; !ok {
					depth[q] = depth[p] + 1
					if depth[q] > maxD {
						maxD = depth[q]
					}
					queue = append(queue, q)
				}
			}
		}
	}
	return maxD + 1
}

// Play places a stone. Returns the captured points.
func (g *Game) Play(c rules.Color, x, y int, now time.Time) ([]int, error) {
	if g.Phase != PhasePlay {
		return nil, fmt.Errorf("not playing")
	}
	if c != g.Turn {
		return nil, fmt.Errorf("not your turn")
	}
	if now.Add(inputTolerance).Before(g.InputOpenAt) {
		return nil, fmt.Errorf("animation in progress")
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
	anim := placeAnim
	if layers := captureLayers(g.Board, g.Last, captured); layers > 0 {
		chain := captureStart + time.Duration(layers-1)*captureLayer + captureStone
		if chain > anim {
			anim = chain
		}
	}
	g.InputOpenAt = now.Add(anim)
	g.Deadline = g.InputOpenAt.Add(g.moveTime())
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
	g.InputOpenAt = now
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
	g.InputOpenAt = now
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
}

func (g *Game) Resign(c rules.Color) {
	if g.Phase == PhaseResult {
		return
	}
	g.Result = &Result{Winner: c.Opponent().Letter(), Reason: "resign"}
	g.Phase = PhaseResult
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
		if wait := g.InputOpenAt.Sub(now); wait > 0 {
			// a pause during an animation: keep the full move time for after the pause
			g.Remaining = g.moveTime()
		}
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
		g.InputOpenAt = now
		g.Deadline = now.Add(g.Remaining)
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
