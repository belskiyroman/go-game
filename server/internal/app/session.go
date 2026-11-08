package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// SessionTTL: a session lives two days since the last visit.
const SessionTTL = 48 * time.Hour

// Session is an anonymous player: a nickname bound to a browser by a cookie. No accounts.
type Session struct {
	ID       string    `json:"id"`
	Token    string    `json:"token"`
	Nick     string    `json:"nick"`
	LastSeen time.Time `json:"lastSeen"`
}

// Tag is a short form of the player ID (no longer shown: nicknames are unique).
func (s *Session) Tag() string { return "#" + s.ID[:4] }

// SessionStore keeps sessions in memory and mirrors them to a JSON file so they survive restarts.
type SessionStore struct {
	mu      sync.Mutex
	path    string
	byToken map[string]*Session
	byID    map[string]*Session
	dirty   bool
}

func NewSessionStore(path string) *SessionStore {
	st := &SessionStore{path: path, byToken: map[string]*Session{}, byID: map[string]*Session{}}
	if data, err := os.ReadFile(path); err == nil {
		var list []*Session
		if json.Unmarshal(data, &list) == nil {
			for _, s := range list {
				if time.Since(s.LastSeen) < SessionTTL {
					st.byToken[s.Token] = s
					st.byID[s.ID] = s
				}
			}
		}
	}
	go st.loop()
	return st
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CleanNick trims a nickname to 1..20 printable characters.
func CleanNick(nick string) (string, bool) {
	nick = strings.TrimSpace(strings.Join(strings.Fields(nick), " "))
	if nick == "" {
		return "", false
	}
	if utf8.RuneCountInString(nick) > 20 {
		nick = string([]rune(nick)[:20])
	}
	return nick, true
}

// Get returns a live session by token and extends its life.
func (st *SessionStore) Get(token string) *Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.byToken[token]
	if s == nil {
		return nil
	}
	if time.Since(s.LastSeen) > SessionTTL {
		delete(st.byToken, s.Token)
		st.unlink(s)
		st.dirty = true
		return nil
	}
	s.LastSeen = time.Now()
	st.dirty = true
	return s
}

// Login: the nickname is the player's login (no password yet). A known nickname (case does not
// matter) signs this browser in as that player; a new nickname creates a new player. Several
// browsers may be signed in as the same player at once: their sessions share the player's ID.
func (st *SessionStore) Login(token, nick string) *Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now()
	id := ""
	for _, o := range st.byToken {
		if time.Since(o.LastSeen) < SessionTTL && strings.EqualFold(o.Nick, nick) {
			id, nick = o.ID, o.Nick // keep the player's own spelling
			break
		}
	}
	if id == "" {
		id = randomHex(8) // a new player
	}
	s := st.byToken[token]
	if s == nil || time.Since(s.LastSeen) >= SessionTTL {
		s = &Session{Token: randomHex(24)}
		st.byToken[s.Token] = s
	} else if s.ID != id {
		st.unlink(s) // this browser leaves its previous player
	}
	s.ID, s.Nick, s.LastSeen = id, nick, now
	st.byID[id] = s
	st.dirty = true
	return s
}

// unlink drops s from the player index; another live session of the same player takes its place.
func (st *SessionStore) unlink(s *Session) {
	if st.byID[s.ID] != s {
		return
	}
	delete(st.byID, s.ID)
	for _, o := range st.byToken {
		if o != s && o.ID == s.ID {
			st.byID[s.ID] = o
			return
		}
	}
}

func (st *SessionStore) ByID(id string) *Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.byID[id]
}

// ByNick finds the live player with this nickname (case does not matter), or nil.
func (st *SessionStore) ByNick(nick string) *Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, s := range st.byID {
		if strings.EqualFold(s.Nick, nick) && time.Since(s.LastSeen) < SessionTTL {
			return s
		}
	}
	return nil
}

// loop expires old sessions and writes the file when something changed.
func (st *SessionStore) loop() {
	for range time.Tick(15 * time.Second) {
		st.mu.Lock()
		for tok, s := range st.byToken {
			if time.Since(s.LastSeen) > SessionTTL {
				delete(st.byToken, tok)
				st.unlink(s)
				st.dirty = true
			}
		}
		st.mu.Unlock()
		st.Save()
	}
}

// Save writes the sessions to the file if something changed (also called when the server stops).
func (st *SessionStore) Save() {
	st.mu.Lock()
	if !st.dirty {
		st.mu.Unlock()
		return
	}
	list := make([]*Session, 0, len(st.byToken))
	for _, s := range st.byToken {
		list = append(list, s)
	}
	st.dirty = false
	st.mu.Unlock()
	data, _ := json.Marshal(list)
	_ = os.MkdirAll(filepath.Dir(st.path), 0o755)
	tmp := st.path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, st.path)
	}
}
