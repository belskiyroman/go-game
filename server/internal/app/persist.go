package app

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Games survive server restarts (a deploy, a reboot): the hub writes them to a file every few
// seconds and when the server stops, and reads them back on start. The clocks stand still while
// the server is down. Clients reconnect on their own and get the game state again.

type gamesFile struct {
	Games   []*Game
	Players map[string]string // player ID → game ID (finished games stay until the player leaves)
}

// SaveGames writes the games that players are in. Safe to call at any time.
func (h *Hub) SaveGames() {
	if h.gamesPath == "" {
		return
	}
	h.mu.Lock()
	now := time.Now()
	f := gamesFile{Players: map[string]string{}}
	saved := map[*Game]bool{}
	for pid, g := range h.playerGame {
		f.Players[pid] = g.ID
		if saved[g] {
			continue
		}
		saved[g] = true
		c := *g // a copy with the clocks as of now
		c.Played = g.PlayedTime(now)
		if !g.Running.IsZero() {
			c.Running = now
		}
		if g.Phase == PhasePlay || g.Phase == PhaseScoring {
			c.ClockLeft = g.Deadline.Sub(now)
		}
		f.Games = append(f.Games, &c)
	}
	data, err := json.Marshal(f)
	h.gamesDirty = false
	h.mu.Unlock()
	if err != nil {
		log.Printf("save games: %v", err)
		return
	}
	_ = os.MkdirAll(filepath.Dir(h.gamesPath), 0o755)
	tmp := h.gamesPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("save games: %v", err)
		return
	}
	_ = os.Rename(tmp, h.gamesPath)
}

// loadGames restores the saved games; called once when the hub starts.
func (h *Hub) loadGames() {
	data, err := os.ReadFile(h.gamesPath)
	if err != nil {
		return
	}
	var f gamesFile
	if err := json.Unmarshal(data, &f); err != nil {
		log.Printf("load games: %v", err)
		return
	}
	now := time.Now()
	byID := map[string]*Game{}
	for _, g := range f.Games {
		if g == nil || g.Board == nil {
			continue
		}
		if !g.Running.IsZero() {
			g.Running = now
		}
		if g.Phase == PhasePlay || g.Phase == PhaseScoring {
			g.Deadline = now.Add(g.ClockLeft)
		}
		byID[g.ID] = g
		h.games[g.ID] = g
		h.armTimer(g)
	}
	for pid, gid := range f.Players {
		if g := byID[gid]; g != nil {
			h.playerGame[pid] = g
		}
	}
	log.Printf("restored %d games", len(byID))
}

// saveLoop writes the games every few seconds when something changed, so that even a crash
// loses only the last moments.
func (h *Hub) saveLoop() {
	for range time.Tick(5 * time.Second) {
		h.mu.Lock()
		dirty := h.gamesDirty
		h.mu.Unlock()
		if dirty {
			h.SaveGames()
		}
	}
}
