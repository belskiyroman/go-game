package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ResultStore keeps head-to-head wins between pairs of players. Players are keyed by nickname
// (case does not matter): the nickname is the login, while a player ID changes when a player
// expires. Results are kept for good, separately from sessions.
type ResultStore struct {
	mu   sync.Mutex
	path string
	wins map[string]int // "winner\x00loser" (lower-case nicknames) → games won
}

func NewResultStore(path string) *ResultStore {
	rs := &ResultStore{path: path, wins: map[string]int{}}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &rs.wins)
	}
	return rs
}

func pairKey(winner, loser string) string {
	return strings.ToLower(winner) + "\x00" + strings.ToLower(loser)
}

// Add counts one win and writes the file at once (results are rare and must not be lost).
func (rs *ResultStore) Add(winner, loser string) {
	rs.mu.Lock()
	rs.wins[pairKey(winner, loser)]++
	data, _ := json.Marshal(rs.wins)
	rs.mu.Unlock()
	_ = os.MkdirAll(filepath.Dir(rs.path), 0o755)
	tmp := rs.path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, rs.path)
	}
}

// Score returns how many games a won against b and b won against a.
func (rs *ResultStore) Score(a, b string) (int, int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.wins[pairKey(a, b)], rs.wins[pairKey(b, a)]
}

// ResultsPath puts the results next to the sessions file: sessions.json → results.json.
func ResultsPath(sessionsPath string) string { return dataPath(sessionsPath, "results") }

// GamesPath puts the saved games next to the sessions file: sessions.json → games.json.
func GamesPath(sessionsPath string) string { return dataPath(sessionsPath, "games") }

func dataPath(sessionsPath, name string) string {
	dir, base := filepath.Split(sessionsPath)
	if strings.Contains(base, "sessions") {
		return filepath.Join(dir, strings.Replace(base, "sessions", name, 1))
	}
	return filepath.Join(dir, strings.TrimSuffix(base, filepath.Ext(base))+"."+name+".json")
}
