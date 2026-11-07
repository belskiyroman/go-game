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

// Tag is the short id shown next to a nickname, so equal nicknames can be told apart.
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
		delete(st.byID, s.ID)
		st.dirty = true
		return nil
	}
	s.LastSeen = time.Now()
	st.dirty = true
	return s
}

// Login creates a session, or renames the existing one when the token is still valid.
func (st *SessionStore) Login(token, nick string) *Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	if s := st.byToken[token]; s != nil && time.Since(s.LastSeen) < SessionTTL {
		s.Nick = nick
		s.LastSeen = time.Now()
		st.dirty = true
		return s
	}
	s := &Session{ID: randomHex(8), Token: randomHex(24), Nick: nick, LastSeen: time.Now()}
	st.byToken[s.Token] = s
	st.byID[s.ID] = s
	st.dirty = true
	return s
}

func (st *SessionStore) ByID(id string) *Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.byID[id]
}

// loop expires old sessions and writes the file when something changed.
func (st *SessionStore) loop() {
	for range time.Tick(15 * time.Second) {
		st.mu.Lock()
		for tok, s := range st.byToken {
			if time.Since(s.LastSeen) > SessionTTL {
				delete(st.byToken, tok)
				delete(st.byID, s.ID)
				st.dirty = true
			}
		}
		if !st.dirty {
			st.mu.Unlock()
			continue
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
}
