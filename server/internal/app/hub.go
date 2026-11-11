package app

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"
	"gogame/internal/rules"
)

// Conn is one browser tab connected over WebSocket.
type Conn struct {
	session *Session
	ws      *websocket.Conn
	out     chan []byte
}

// Invite: one outgoing invitation per player. The inviter's settings apply. An invitation with
// Game set offers to continue that adjourned game.
type Invite struct {
	From, To string
	Settings Settings
	At       time.Time
	Game     string
}

// Hub holds the room (lobby), invitations and games. One mutex keeps it simple: the load is tiny.
type Hub struct {
	mu         sync.Mutex
	sessions   *SessionStore
	results    *ResultStore
	conns      map[string]map[*Conn]bool
	settings   map[string]Settings
	invites    map[string]*Invite // by inviter id
	games      map[string]*Game
	playerGame map[string]*Game // the game a player is in (also a finished one until they leave)

	gamesPath  string // where games are saved between restarts ("" = not saved)
	gamesDirty bool
}

func NewHub(sessions *SessionStore, results *ResultStore, gamesPath string) *Hub {
	h := &Hub{
		sessions: sessions, results: results, conns: map[string]map[*Conn]bool{}, settings: map[string]Settings{},
		invites: map[string]*Invite{}, games: map[string]*Game{}, playerGame: map[string]*Game{},
		gamesPath: gamesPath,
	}
	if gamesPath != "" {
		h.loadGames()
		go h.saveLoop()
	}
	return h
}

type inMsg struct {
	T        string   `json:"t"`
	To       string   `json:"to,omitempty"`
	From     string   `json:"from,omitempty"`
	X        int      `json:"x,omitempty"`
	Y        int      `json:"y,omitempty"`
	Ready    bool     `json:"ready,omitempty"`
	Accept   bool     `json:"accept,omitempty"`
	Settings Settings `json:"settings,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Step     int      `json:"step,omitempty"`
	Swap     bool     `json:"swap,omitempty"`
	Game     string   `json:"game,omitempty"`
}

// Serve runs one WebSocket connection until it closes.
func (h *Hub) Serve(ctx context.Context, s *Session, ws *websocket.Conn) {
	c := &Conn{session: s, ws: ws, out: make(chan []byte, 64)}
	h.mu.Lock()
	if h.conns[s.ID] == nil {
		h.conns[s.ID] = map[*Conn]bool{}
	}
	h.conns[s.ID][c] = true
	h.sendTo(s.ID, map[string]any{"t": "me", "id": s.ID, "nick": s.Nick, "tag": s.Tag()})
	if g := h.playerGame[s.ID]; g != nil {
		if g.Away != rules.Empty && g.Away == g.ColorOf(s.ID) {
			// back before the opponent decided: the game goes on, with a full move
			g.Away = rules.Empty
			g.Deadline = time.Now().Add(g.moveTime())
			g.Running = time.Now()
			g.Seq++
			h.armTimer(g)
			h.broadcastGame(g, nil)
		} else {
			h.sendGame(g, s.ID)
		}
	}
	h.broadcastLobby()
	h.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		ping := time.NewTicker(20 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-c.out:
				wctx, wc := context.WithTimeout(ctx, 10*time.Second)
				err := ws.Write(wctx, websocket.MessageText, msg)
				wc()
				if err != nil {
					cancel()
					return
				}
			case <-ping.C:
				pctx, pc := context.WithTimeout(ctx, 10*time.Second)
				err := ws.Ping(pctx)
				pc()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			break
		}
		var m inMsg
		if json.Unmarshal(data, &m) == nil {
			h.handle(s, m)
		}
	}

	h.mu.Lock()
	delete(h.conns[s.ID], c)
	if len(h.conns[s.ID]) == 0 {
		delete(h.conns, s.ID)
		// a player who left the room cannot be waiting for answers
		if inv := h.invites[s.ID]; inv != nil {
			delete(h.invites, s.ID)
		}
		for from, inv := range h.invites {
			if inv.To == s.ID {
				delete(h.invites, from)
			}
		}
	}
	h.broadcastLobby()
	h.mu.Unlock()
	ws.Close(websocket.StatusNormalClosure, "")
}

func (h *Hub) sendTo(sessionID string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	for c := range h.conns[sessionID] {
		select {
		case c.out <- data:
		default: // a stuck tab: drop the message rather than block everyone
		}
	}
}

func (h *Hub) settingsOf(id string) Settings {
	if s, ok := h.settings[id]; ok {
		return s
	}
	return DefaultSettings()
}

func (h *Hub) busy(id string) bool {
	g := h.playerGame[id]
	return g != nil && g.Active()
}

// Drop closes the connections of one browser's session (on logout); the player's other
// browsers stay connected.
func (h *Hub) Drop(s *Session) {
	h.mu.Lock()
	var mine []*Conn
	for c := range h.conns[s.ID] {
		if c.session == s {
			mine = append(mine, c)
		}
	}
	h.mu.Unlock()
	for _, c := range mine {
		_ = c.ws.Close(websocket.StatusNormalClosure, "logout")
	}
}

func (h *Hub) online(id string) bool { return len(h.conns[id]) > 0 }

// release takes the player out of their (finished) game.
func (h *Hub) release(id string) {
	if g := h.playerGame[id]; g != nil {
		delete(h.playerGame, id)
		h.forget(g)
	}
}

// forget drops a finished game once no player looks at it any more.
func (h *Hub) forget(g *Game) {
	if g.Active() || g.Phase == PhaseAdjourned {
		return
	}
	for _, pg := range h.playerGame {
		if pg == g {
			return
		}
	}
	delete(h.games, g.ID)
	h.gamesDirty = true
}

// gamePlayers are the players' current IDs that are in the game.
func (h *Hub) gamePlayers(g *Game) []string {
	ids := []string{}
	for _, c := range []rules.Color{rules.Black, rules.White} {
		if id := g.Players[c]; h.playerGame[id] == g {
			ids = append(ids, id)
		}
	}
	return ids
}

func (h *Hub) handle(s *Session, m inMsg) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	switch m.T {
	case "settings":
		if m.Settings.Valid() && h.invites[s.ID] == nil {
			h.settings[s.ID] = m.Settings
		}
		h.broadcastLobby()

	case "invite":
		if h.invites[s.ID] != nil || m.To == s.ID || h.busy(s.ID) || h.busy(m.To) || len(h.conns[m.To]) == 0 {
			h.broadcastLobby()
			return
		}
		h.invites[s.ID] = &Invite{From: s.ID, To: m.To, Settings: h.settingsOf(s.ID), At: now}
		h.broadcastLobby()

	case "cancelInvite":
		delete(h.invites, s.ID)
		h.broadcastLobby()

	case "declineInvite":
		if inv := h.invites[m.From]; inv != nil && inv.To == s.ID {
			delete(h.invites, m.From)
		}
		h.broadcastLobby()

	case "acceptInvite":
		inv := h.invites[m.From]
		if inv == nil || inv.To != s.ID || h.busy(s.ID) || h.busy(m.From) {
			h.broadcastLobby()
			return
		}
		if inv.Game != "" {
			h.continueGame(inv, now)
		} else {
			h.startGame(inv)
		}

	case "resume": // offer the opponent to continue an adjourned game
		g := h.games[m.Game]
		if g == nil || g.Phase != PhaseAdjourned || h.invites[s.ID] != nil || h.busy(s.ID) {
			h.broadcastLobby()
			return
		}
		c := g.colorOfNick(s.Nick)
		if c == rules.Empty {
			return
		}
		opp := h.sessions.ByNick(g.Nicks[c.Opponent()])
		if opp == nil || opp.ID == s.ID || !h.online(opp.ID) || h.busy(opp.ID) {
			h.broadcastLobby()
			return
		}
		h.invites[s.ID] = &Invite{From: s.ID, To: opp.ID, Settings: g.Settings, At: now, Game: g.ID}
		h.broadcastLobby()

	case "rematch":
		g := h.playerGame[s.ID]
		if g == nil || g.Active() {
			return
		}
		opp := g.Players[g.ColorOf(s.ID).Opponent()]
		if h.invites[s.ID] == nil && !h.busy(opp) && len(h.conns[opp]) > 0 {
			h.invites[s.ID] = &Invite{From: s.ID, To: opp, Settings: g.Settings, At: now}
		}
		h.broadcastLobby()

	case "refresh": // e.g. after a nickname change
		h.sendTo(s.ID, map[string]any{"t": "me", "id": s.ID, "nick": s.Nick, "tag": s.Tag()})
		h.broadcastLobby()

	case "leave":
		if g := h.playerGame[s.ID]; g != nil && !g.Active() {
			h.release(s.ID)
			h.gamesDirty = true
			h.sendTo(s.ID, map[string]any{"t": "noGame"})
		}
		h.broadcastLobby()

	default:
		h.handleGame(s, m, now)
	}
}

func (h *Hub) startGame(inv *Invite) {
	a, b := h.sessions.ByID(inv.From), h.sessions.ByID(inv.To)
	if a == nil || b == nil {
		return
	}
	black, white := a, b
	if rand.IntN(2) == 1 { // colors are random
		black, white = b, a
	}
	g := NewGame(inv.Settings, black, white)
	h.games[g.ID] = g
	h.enter(g, a.ID, b.ID)
	h.armTimer(g)
	h.sendGame(g, a.ID)
	h.sendGame(g, b.ID)
	h.broadcastLobby()
}

// enter puts both players into the game: they are busy now, so every invitation that involves
// them is dropped.
func (h *Hub) enter(g *Game, a, b string) {
	for from, i := range h.invites {
		if from == a || from == b || i.To == a || i.To == b {
			delete(h.invites, from)
		}
	}
	h.release(a)
	h.release(b)
	h.playerGame[a] = g
	h.playerGame[b] = g
	h.gamesDirty = true
}

// continueGame resumes an adjourned game. The players may have new IDs (the game is kept by
// nickname), so they are refreshed.
func (h *Hub) continueGame(inv *Invite, now time.Time) {
	g := h.games[inv.Game]
	a, b := h.sessions.ByID(inv.From), h.sessions.ByID(inv.To)
	if g == nil || g.Phase != PhaseAdjourned || a == nil || b == nil {
		delete(h.invites, inv.From)
		h.broadcastLobby()
		return
	}
	for _, s := range []*Session{a, b} {
		c := g.colorOfNick(s.Nick)
		if c == rules.Empty {
			delete(h.invites, inv.From)
			h.broadcastLobby()
			return
		}
		g.Players[c], g.Nicks[c], g.Tags[c] = s.ID, s.Nick, s.Tag()
	}
	g.Continue(now)
	h.enter(g, a.ID, b.ID)
	h.armTimer(g)
	h.sendGame(g, a.ID)
	h.sendGame(g, b.ID)
	h.broadcastLobby()
}

// adjournGame freezes the game and sends both players to the room.
func (h *Hub) adjournGame(g *Game, now time.Time) {
	ids := h.gamePlayers(g)
	g.Adjourn(now)
	h.armTimer(g)
	for _, id := range ids {
		delete(h.playerGame, id)
		h.sendTo(id, map[string]any{"t": "noGame"})
	}
	h.gamesDirty = true
	h.broadcastLobby()
}

// closeGame ends the game without counting; it is not counted in the score and is not kept.
func (h *Hub) closeGame(g *Game) {
	if g.timer != nil {
		g.timer.Stop()
	}
	for _, id := range h.gamePlayers(g) {
		delete(h.playerGame, id)
		h.sendTo(id, map[string]any{"t": "noGame"})
	}
	delete(h.games, g.ID)
	h.detachAlts(g)
	h.gamesDirty = true
	h.broadcastLobby()
}

// altsOf lists the alternatives of a main game, the oldest first.
func (h *Hub) altsOf(mainID string) []*Game {
	list := []*Game{}
	for _, x := range h.games {
		if x.Parent == mainID && mainID != "" {
			list = append(list, x)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created.Before(list[j].Created) })
	return list
}

// detachAlts: when a main game ends or is closed, its alternatives become main games of their own.
func (h *Hub) detachAlts(main *Game) {
	for _, x := range h.altsOf(main.ID) {
		x.Parent = ""
		x.Seq++
		h.broadcastGame(x, nil)
	}
}

// openMain is the adjourned main game of an alternative, which the players may go back to.
func (h *Hub) openMain(alt *Game) *Game {
	if m := h.games[alt.Parent]; m != nil && alt.Parent != "" && m.Phase == PhaseAdjourned {
		return m
	}
	return nil
}

// backToMain closes the alternative (not counted) and continues its main game.
func (h *Hub) backToMain(alt *Game, now time.Time) {
	main := h.openMain(alt)
	ids := h.gamePlayers(alt)
	if main == nil || len(ids) != 2 {
		return
	}
	if alt.timer != nil {
		alt.timer.Stop()
	}
	delete(h.games, alt.ID)
	for _, id := range ids {
		delete(h.playerGame, id)
		if s := h.sessions.ByID(id); s != nil { // the players' IDs may be newer than the main game's
			if c := main.colorOfNick(s.Nick); c != rules.Empty {
				main.Players[c], main.Nicks[c], main.Tags[c] = s.ID, s.Nick, s.Tag()
			}
		}
	}
	main.Continue(now)
	h.enter(main, ids[0], ids[1])
	h.armTimer(main)
	for _, id := range ids {
		h.sendGame(main, id)
	}
	h.broadcastLobby()
}

// promote makes the alternative the main game: the old main game is closed without counting,
// its game time is added to this one's, and its other alternatives now belong to this one.
func (h *Hub) promote(alt *Game) {
	if main := h.games[alt.Parent]; main != nil {
		alt.Played += main.PlayedTime(time.Now())
		if main.timer != nil {
			main.timer.Stop()
		}
		delete(h.games, main.ID)
		for _, x := range h.altsOf(main.ID) {
			if x != alt {
				x.Parent = alt.ID
			}
		}
	}
	alt.Parent, alt.From = "", 0
	alt.Seq++
	h.gamesDirty = true
	h.broadcastGame(alt, nil)
	h.broadcastLobby()
}

// branchGame moves both players into an alternative of the game; an unfinished game is adjourned.
func (h *Hub) branchGame(g *Game, now time.Time) {
	ids := h.gamePlayers(g)
	alt := g.Branch(now)
	if g.Active() {
		g.Adjourn(now)
		h.armTimer(g)
	}
	h.games[alt.ID] = alt
	for _, id := range ids {
		h.playerGame[id] = alt
	}
	h.forget(g)
	h.gamesDirty = true
	for _, id := range ids {
		h.sendGame(alt, id)
	}
	h.broadcastLobby()
}

func (h *Hub) handleGame(s *Session, m inMsg, now time.Time) {
	g := h.playerGame[s.ID]
	if g == nil {
		return
	}
	c := g.ColorOf(s.ID)
	var event map[string]any
	switch m.T {
	case "move":
		captured, err := g.Play(c, m.X, m.Y, now)
		if err != nil {
			h.sendTo(s.ID, map[string]any{"t": "error", "text": err.Error()})
			h.sendGame(g, s.ID)
			return
		}
		if captured == nil {
			captured = []int{}
		}
		event = map[string]any{"kind": "move", "color": c.Letter(), "x": m.X, "y": m.Y, "captured": captured}
	case "pass":
		if g.Pass(c, now) != nil {
			return
		}
		event = map[string]any{"kind": "pass", "color": c.Letter()}
	case "resign":
		g.Resign(c)
		event = map[string]any{"kind": "resign", "color": c.Letter()}
	case "pauseRequest":
		if g.RequestPause(c) != nil {
			return
		}
	case "pauseCancel":
		g.CancelPause(c)
	case "pauseAnswer":
		g.AnswerPause(c, m.Accept, now)
	case "ready":
		g.SetReady(c, m.Ready, now)
	case "scoreAgree":
		g.AgreeScore(c)
	case "playOn":
		g.PlayOn(now)
	case "toggleDead":
		if g.ToggleDead(m.X, m.Y, now) != nil {
			return
		}
	case "toggleNeutral":
		if g.ToggleNeutral(m.X, m.Y, now) != nil {
			return
		}

	case "ask":
		oppID := g.Players[c.Opponent()]
		if m.Kind == ReqAlt && len(h.altsOf(g.ID)) >= MaxAlts {
			return
		}
		if m.Kind == ReqBack && h.openMain(g) == nil {
			return
		}
		if m.Kind == ReqAdjourn && g.Active() && !h.online(oppID) {
			h.adjournGame(g, now) // nobody to ask: the opponent is offline
			return
		}
		if g.Ask(c, m.Kind) != nil {
			return
		}
	case "askCancel":
		g.CancelAsk(c)
	case "askAnswer":
		r := g.Req
		if r == nil || r.From == c || c == rules.Empty {
			return
		}
		g.Req = nil
		g.Seq++
		if !m.Accept {
			break
		}
		switch r.Kind {
		case ReqAlt:
			h.branchGame(g, now)
		case ReqAdjourn:
			h.adjournGame(g, now)
		case ReqClose:
			h.closeGame(g)
		case ReqBack:
			h.backToMain(g, now)
		case ReqPromote:
			h.promote(g)
		}
		return

	case "propose":
		if g.Propose(c, m.Step, m.Swap) != nil {
			h.sendGame(g, s.ID)
			return
		}
	case "proposeCancel":
		if g.Proposal == nil || g.Proposal.From != c {
			return
		}
		g.DropProposal()
	case "proposeAnswer":
		p := g.Proposal
		if p == nil || p.From == c {
			return
		}
		if !m.Accept {
			g.DropProposal()
			break
		}
		if g.AcceptProposal(c, now) != nil {
			return
		}
		event = map[string]any{"kind": "started"}

	case "awayScore":
		if g.Away != c.Opponent() {
			return
		}
		g.ScoreAway()
	case "awayAdjourn":
		if g.Away != c.Opponent() {
			return
		}
		h.adjournGame(g, now)
		return
	default:
		return
	}
	h.armTimer(g)
	h.broadcastGame(g, event)
	if !g.Active() {
		h.broadcastLobby()
	}
}

// armTimer schedules the per-move clock check for the current deadline.
func (h *Hub) armTimer(g *Game) {
	if g.timer != nil {
		g.timer.Stop()
	}
	if (g.Phase != PhasePlay && g.Phase != PhaseScoring) || g.Away != rules.Empty {
		return
	}
	d := time.Until(g.Deadline) + 50*time.Millisecond
	g.timer = time.AfterFunc(d, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		turn := g.Turn
		phase := g.Phase
		now := time.Now()
		if phase == PhasePlay && !now.Before(g.Deadline) && !h.online(g.Players[turn]) {
			// the player to move is gone and out of time: the opponent decides (count or adjourn)
			g.Away = turn
			g.PauseReq = rules.Empty
			g.stopClock(now) // the game clock waits too
			g.Seq++
			h.broadcastGame(g, nil)
			return
		}
		if g.Timeout(now) {
			var event map[string]any
			if phase == PhasePlay {
				event = map[string]any{"kind": "pass", "color": turn.Letter(), "timeout": true}
			}
			h.armTimer(g)
			h.broadcastGame(g, event)
			if !g.Active() {
				h.broadcastLobby()
			}
		} else {
			h.armTimer(g)
		}
	})
}

func (h *Hub) broadcastGame(g *Game, event map[string]any) {
	h.gamesDirty = true
	// a finished game counts once in the players' head-to-head score
	if g.Result != nil && !g.Recorded {
		g.Recorded = true
		w := rules.Black
		if g.Result.Winner == "w" {
			w = rules.White
		}
		h.results.Add(g.Nicks[w], g.Nicks[w.Opponent()])
		h.detachAlts(g)
	}
	for _, c := range []rules.Color{rules.Black, rules.White} {
		id := g.Players[c]
		if h.playerGame[id] == g {
			h.sendGame(g, id, event)
		}
	}
}

func (h *Hub) sendGame(g *Game, sessionID string, event ...map[string]any) {
	now := time.Now()
	ms := func(t time.Time) int64 { return t.UnixMilli() }
	st := map[string]any{
		"id": g.ID, "seq": g.Seq, "size": g.Settings.Size, "komi": g.Settings.Komi, "moveTime": g.Settings.MoveTime,
		"you":   g.ColorOf(sessionID).Letter(),
		"black": map[string]any{"id": g.Players[rules.Black], "nick": g.Nicks[rules.Black], "tag": g.Tags[rules.Black]},
		"white": map[string]any{"id": g.Players[rules.White], "nick": g.Nicks[rules.White], "tag": g.Tags[rules.White]},
		"board": g.Board.String(), "turn": g.Turn.Letter(), "last": g.Last, "passes": g.Passes,
		"caps":  map[string]int{"b": g.Caps[rules.Black], "w": g.Caps[rules.White]},
		"phase": g.Phase, "pauseReq": g.PauseReq.Letter(),
		"ready":     map[string]bool{"b": g.Ready[rules.Black], "w": g.Ready[rules.White]},
		"agree":     map[string]bool{"b": g.Agree[rules.Black], "w": g.Agree[rules.White]},
		"serverNow": ms(now), "deadline": ms(g.Deadline),
		"remaining": g.Remaining.Milliseconds(),
		"played":    g.PlayedTime(now).Milliseconds(), "playedRunning": !g.Running.IsZero(),
		"moves": len(g.Moves),
		"h2h":   h.h2h(g.Nicks[rules.Black], g.Nicks[rules.White]),
		"away":  g.Away.Letter(), "parent": g.Parent, "from": g.From,
		"alts": len(h.altsOf(mainOf(g))), "mainOpen": h.openMain(g) != nil,
	}
	if g.Req != nil {
		st["req"] = map[string]any{"kind": g.Req.Kind, "from": g.Req.From.Letter()}
	}
	if g.Phase == PhaseSetup {
		st["source"] = encodeMoves(g.Source)
		if p := g.Proposal; p != nil {
			st["proposal"] = map[string]any{"from": p.From.Letter(), "step": p.Step, "swap": p.Swap}
		}
	}
	if g.Phase == PhaseScoring || g.Phase == PhaseResult {
		dead := []int{}
		for i := range g.Dead {
			dead = append(dead, i)
		}
		sort.Ints(dead)
		tb, tw := rules.Territory(g.Board, g.Dead, g.Neutral)
		neutral := []int{}
		for i := range g.Neutral {
			neutral = append(neutral, i)
		}
		sort.Ints(neutral)
		st["dead"] = dead
		st["neutral"] = neutral
		st["territory"] = map[string][]int{"b": nonNil(tb), "w": nonNil(tw)}
	}
	if g.Result != nil {
		st["result"] = g.Result
		st["sgf"] = g.SGF()
	}
	msg := map[string]any{"t": "game", "game": st}
	if len(event) > 0 && event[0] != nil {
		msg["event"] = event[0]
	}
	h.sendTo(sessionID, msg)
}

func nonNil(a []int) []int {
	if a == nil {
		return []int{}
	}
	return a
}

// broadcastLobby sends every connected player their personal view of the room.
func (h *Hub) broadcastLobby() {
	type player struct {
		ID     string `json:"id"`
		Nick   string `json:"nick"`
		Tag    string `json:"tag"`
		Status string `json:"status"` // free | playing
		H2H    [2]int `json:"h2h"`    // wins of the viewer against this player, and of this player against the viewer
	}
	all := []player{}
	for id := range h.conns {
		s := h.sessions.ByID(id)
		if s == nil {
			continue
		}
		st := "free"
		if h.busy(id) {
			st = "playing"
		}
		all = append(all, player{ID: id, Nick: s.Nick, Tag: s.Tag(), Status: st})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Nick != all[j].Nick {
			return all[i].Nick < all[j].Nick
		}
		return all[i].ID < all[j].ID
	})
	for id := range h.conns {
		me := h.sessions.ByID(id)
		if me == nil {
			continue
		}
		vs := func(nick string) [2]int { a, b := h.results.Score(me.Nick, nick); return [2]int{a, b} }
		card := func(inv *Invite) any {
			if g := h.games[inv.Game]; g != nil {
				return h.gameCard(g, me.Nick)
			}
			return nil
		}
		var outgoing any
		if inv := h.invites[id]; inv != nil {
			to := h.sessions.ByID(inv.To)
			if to != nil {
				outgoing = map[string]any{"to": inv.To, "nick": to.Nick, "tag": to.Tag(), "settings": inv.Settings, "h2h": vs(to.Nick), "game": card(inv)}
			}
		}
		incoming := []map[string]any{}
		for _, inv := range h.invites {
			if inv.To == id {
				if from := h.sessions.ByID(inv.From); from != nil {
					incoming = append(incoming, map[string]any{"from": inv.From, "nick": from.Nick, "tag": from.Tag(), "settings": inv.Settings, "h2h": vs(from.Nick), "game": card(inv)})
				}
			}
		}
		games := []map[string]any{}
		for _, g := range h.myGames(me.Nick) {
			games = append(games, h.gameCard(g, me.Nick))
		}
		players := make([]player, 0, len(all))
		for _, p := range all {
			if p.ID != id {
				p.H2H = vs(p.Nick)
				players = append(players, p)
			}
		}
		h.sendTo(id, map[string]any{
			"t": "lobby", "players": players, "online": len(all),
			"settings": h.settingsOf(id), "outgoing": outgoing, "incoming": incoming, "games": games,
		})
	}
}

// h2h is the head-to-head score of the game's players: {"b": black's wins, "w": white's wins}.
func (h *Hub) h2h(black, white string) map[string]int {
	b, w := h.results.Score(black, white)
	return map[string]int{"b": b, "w": w}
}

// myGames are the player's unfinished games: the adjourned ones and the one being played.
func (h *Hub) myGames(nick string) []*Game {
	list := []*Game{}
	for _, g := range h.games {
		if (g.Phase == PhaseAdjourned || g.Active()) && g.colorOfNick(nick) != rules.Empty {
			list = append(list, g)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created.Before(list[j].Created) })
	return list
}

// adjournedCount: how many of the games can be continued.
func adjournedCount(list []*Game) int {
	n := 0
	for _, g := range list {
		if g.Phase == PhaseAdjourned {
			n++
		}
	}
	return n
}

// gameCard describes an adjourned game for "My games" and for an invitation to continue it.
func (h *Hub) gameCard(g *Game, nick string) map[string]any {
	c := g.colorOfNick(nick)
	opp := g.Nicks[c.Opponent()]
	available := false
	if s := h.sessions.ByNick(opp); s != nil && h.online(s.ID) && !h.busy(s.ID) {
		available = true
	}
	board, last, moves, turn := g.position()
	return map[string]any{
		"id": g.ID, "you": c.Letter(), "opp": opp, "size": g.Settings.Size, "komi": g.Settings.Komi, "moveTime": g.Settings.MoveTime,
		"board": board, "last": last, "moves": moves, "turn": turn.Letter(),
		"alt": g.Parent != "", "from": g.From, "setup": g.Resume == PhaseSetup || g.Phase == PhaseSetup,
		"main": mainOf(g), "current": g.Active(),
		"adjournedAt": g.AdjournedAt.UnixMilli(), "available": available,
	}
}

// mainOf is the main game of a game's row in "My games": its parent, or the game itself.
func mainOf(g *Game) string {
	if g.Parent != "" {
		return g.Parent
	}
	return g.ID
}
