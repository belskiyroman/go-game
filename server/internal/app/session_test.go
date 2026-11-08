package app

import (
	"path/filepath"
	"testing"
)

func TestNickIsLogin(t *testing.T) {
	st := NewSessionStore(filepath.Join(t.TempDir(), "s.json"))
	a := st.Login("", "Romanos")

	// another browser with the same nick (any case) signs in as the same player
	b := st.Login("", "romanos")
	if b.ID != a.ID || b.Nick != "Romanos" || b.Token == a.Token {
		t.Fatalf("second browser: id %s nick %q, want id %s nick Romanos and its own token", b.ID, b.Nick, a.ID)
	}

	// a new nick creates a new player; this browser switches to it, the other one stays
	c := st.Login(b.Token, "Странник")
	if c.ID == a.ID || c.Token != b.Token {
		t.Fatalf("new nick: id %s token %s", c.ID, c.Token)
	}
	if p := st.ByID(a.ID); p == nil || p.Nick != "Romanos" || p.Token != a.Token {
		t.Fatalf("the first player must stay signed in on the first browser: %+v", p)
	}
	if p := st.ByID(c.ID); p == nil || p.Nick != "Странник" {
		t.Fatalf("the new player: %+v", p)
	}
}
