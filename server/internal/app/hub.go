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

// Invite: one outgoing invitation per player. The inviter's settings apply.
type Invite struct {
	From, To string
	Settings Settings
	At       time.Time
}

// Hub holds the room (lobby), invitations and games. One mutex keeps it simple: the load is tiny.
type Hub struct {
	mu         sync.Mutex
	sessions   *SessionStore
	conns      map[string]map[*Conn]bool
	settings   map[string]Settings
	invites    map[string]*Invite // by inviter id
	games      map[string]*Game
	playerGame map[string]*Game // the game a player is in (also a finished one until they leave)
}

func NewHub(sessions *SessionStore) *Hub {
	return &Hub{
		sessions: sessions, conns: map[string]map[*Conn]bool{}, settings: map[string]Settings{},
		invites: map[string]*Invite{}, games: map[string]*Game{}, playerGame: map[string]*Game{},
	}
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
		h.sendGame(g, s.ID)
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
			h.sendTo(m.From, map[string]any{"t": "notice", "text": s.Nick + " отклонил приглашение"})
		}
		h.broadcastLobby()

	case "acceptInvite":
		inv := h.invites[m.From]
		if inv == nil || inv.To != s.ID || h.busy(s.ID) || h.busy(m.From) {
			h.broadcastLobby()
			return
		}
		h.startGame(inv)

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
			delete(h.playerGame, s.ID)
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
	// both players are now busy: drop every invitation that involves them
	for from, i := range h.invites {
		if from == a.ID || from == b.ID || i.To == a.ID || i.To == b.ID {
			delete(h.invites, from)
		}
	}
	h.playerGame[a.ID] = g
	h.playerGame[b.ID] = g
	h.armTimer(g)
	h.sendGame(g, a.ID)
	h.sendGame(g, b.ID)
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
	if g.Phase != PhasePlay && g.Phase != PhaseScoring {
		return
	}
	d := time.Until(g.Deadline) + 50*time.Millisecond
	g.timer = time.AfterFunc(d, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		turn := g.Turn
		phase := g.Phase
		if g.Timeout(time.Now()) {
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
		"serverNow": ms(now), "deadline": ms(g.Deadline), "inputOpenAt": ms(g.InputOpenAt),
		"remaining": g.Remaining.Milliseconds(),
		"moves":     len(g.Moves),
	}
	if g.Phase == PhaseScoring || g.Phase == PhaseResult {
		dead := []int{}
		for i := range g.Dead {
			dead = append(dead, i)
		}
		sort.Ints(dead)
		tb, tw := rules.Territory(g.Board, g.Dead)
		st["dead"] = dead
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
		var outgoing any
		if inv := h.invites[id]; inv != nil {
			to := h.sessions.ByID(inv.To)
			if to != nil {
				outgoing = map[string]any{"to": inv.To, "nick": to.Nick, "tag": to.Tag(), "settings": inv.Settings}
			}
		}
		incoming := []map[string]any{}
		for _, inv := range h.invites {
			if inv.To == id {
				if from := h.sessions.ByID(inv.From); from != nil {
					incoming = append(incoming, map[string]any{"from": inv.From, "nick": from.Nick, "tag": from.Tag(), "settings": inv.Settings})
				}
			}
		}
		players := make([]player, 0, len(all))
		for _, p := range all {
			if p.ID != id {
				players = append(players, p)
			}
		}
		h.sendTo(id, map[string]any{
			"t": "lobby", "players": players, "online": len(all),
			"settings": h.settingsOf(id), "outgoing": outgoing, "incoming": incoming,
		})
	}
}
