package app

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gogame/internal/rules"
)

// The game history: every game that was played (finished, adjourned or closed) with its final
// SGF. A record is written when a game ends, is adjourned or is closed; a game adjourned and
// finished later keeps one record whose status changes. Records are kept for good, by nickname.

// History statuses.
const (
	HistFinished  = "finished"
	HistAdjourned = "adjourned"
	HistClosed    = "closed"
)

type HistoryGame struct {
	ID       string    `json:"id"`
	Status   string    `json:"status"`
	Alt      bool      `json:"alt"`  // an alternative game of a main game
	From     int       `json:"from"` // an alternative: the move it started from
	Black    string    `json:"black"`
	White    string    `json:"white"`
	Size     int       `json:"size"`
	Komi     float64   `json:"komi"`
	MoveTime int       `json:"moveTime"`
	Started  time.Time `json:"started"`
	Ended    time.Time `json:"ended"`
	Moves    int       `json:"moves"`
	Caps     [2]int    `json:"caps"` // stones captured during play by black, by white
	Result   *Result   `json:"result,omitempty"`
	H2H      [2]int    `json:"h2h"`   // head-to-head wins when the record was written: black's, white's
	Board    string    `json:"board"` // the final position, for the list's small board
	Last     int       `json:"last"`
	SGF      string    `json:"sgf,omitempty"`
}

type HistoryStore struct {
	mu    sync.Mutex
	path  string // "" keeps the history in memory only
	games map[string]*HistoryGame
}

func NewHistoryStore(path string) *HistoryStore {
	hs := &HistoryStore{path: path, games: map[string]*HistoryGame{}}
	if path == "" {
		return hs
	}
	if data, err := os.ReadFile(path); err == nil {
		var list []*HistoryGame
		if err := json.Unmarshal(data, &list); err != nil {
			log.Printf("load history: %v", err)
		}
		for _, r := range list {
			if r != nil {
				hs.games[r.ID] = r
			}
		}
	}
	return hs
}

// HistoryPath puts the history next to the saved games: games.json → history.json.
func HistoryPath(gamesPath string) string {
	if gamesPath == "" {
		return ""
	}
	dir, base := filepath.Split(gamesPath)
	if strings.Contains(base, "games") {
		return filepath.Join(dir, strings.Replace(base, "games", "history", 1))
	}
	return filepath.Join(dir, strings.TrimSuffix(base, filepath.Ext(base))+".history.json")
}

// Put adds or replaces a record and writes the file at once (records are rare and must not be lost).
func (hs *HistoryStore) Put(r *HistoryGame) {
	hs.mu.Lock()
	hs.games[r.ID] = r
	list := make([]*HistoryGame, 0, len(hs.games))
	for _, x := range hs.games {
		list = append(list, x)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Started.Before(list[j].Started) })
	data, err := json.Marshal(list)
	hs.mu.Unlock()
	if hs.path == "" || err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(hs.path), 0o755)
	tmp := hs.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("save history: %v", err)
		return
	}
	_ = os.Rename(tmp, hs.path)
}

// Of lists the games of a player (by nickname), the latest first, without their SGF.
func (hs *HistoryStore) Of(nick string) []HistoryGame {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	out := []HistoryGame{}
	for _, r := range hs.games {
		if strings.EqualFold(r.Black, nick) || strings.EqualFold(r.White, nick) {
			c := *r
			c.SGF = ""
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	return out
}

// Get returns a record if the player (by nickname) played in it.
func (hs *HistoryStore) Get(id, nick string) *HistoryGame {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	r := hs.games[id]
	if r == nil || !(strings.EqualFold(r.Black, nick) || strings.EqualFold(r.White, nick)) {
		return nil
	}
	c := *r
	return &c
}

// archive writes the game's record with the given status. Games without moves of their own
// (an alternative still choosing its start move) are not history.
func (h *Hub) archive(g *Game, status string) {
	if g == nil || len(g.Moves) == 0 || g.Phase == PhaseSetup {
		return
	}
	now := time.Now()
	started := g.Created
	if started.IsZero() {
		started = now
	}
	b, w := h.results.Score(g.Nicks[rules.Black], g.Nicks[rules.White])
	h.history.Put(&HistoryGame{
		ID: g.ID, Status: status, Alt: g.Parent != "", From: g.From,
		Black: g.Nicks[rules.Black], White: g.Nicks[rules.White],
		Size: g.Settings.Size, Komi: g.Settings.Komi, MoveTime: g.Settings.MoveTime,
		Started: started, Ended: now, Moves: len(g.Moves),
		Caps:   [2]int{g.Caps[rules.Black], g.Caps[rules.White]},
		Result: g.Result, H2H: [2]int{b, w},
		Board: g.Board.String(), Last: g.Last,
		SGF: g.SGF(),
	})
}
