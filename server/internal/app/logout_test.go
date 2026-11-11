package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogout(t *testing.T) {
	dir := t.TempDir()
	st := NewSessionStore(filepath.Join(dir, "s.json"))
	srv := &Server{Sessions: st, Hub: NewHub(st, NewResultStore(filepath.Join(dir, "r.json")), filepath.Join(dir, "g.json"))}
	h := srv.Routes()
	a := st.Login("", "Romanos")

	req := httptest.NewRequest("POST", "/api/logout", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: a.Token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout: %d, Set-Cookie %q: the cookie must be removed", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	// the player is kept: the same nickname signs in as them again
	if b := st.Login("", "romanos"); b.ID != a.ID {
		t.Fatalf("after logout the nick gave a new player %s, want %s", b.ID, a.ID)
	}
}
