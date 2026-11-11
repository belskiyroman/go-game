package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const cookieName = "gogame_session"

type Server struct {
	Sessions *SessionStore
	Hub      *Hub
	WebDir   string
}

func (srv *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", srv.me)
	mux.HandleFunc("POST /api/login", srv.login)
	mux.HandleFunc("POST /api/logout", srv.logout)
	mux.HandleFunc("GET /ws", srv.ws)
	mux.HandleFunc("/", srv.static)
	return mux
}

// isHTTPS: behind ngrok the browser talks HTTPS, the tunnel talks plain HTTP to us.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (srv *Server) setCookie(w http.ResponseWriter, r *http.Request, s *Session) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: s.Token, Path: "/", HttpOnly: true,
		Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode,
		MaxAge: int(SessionTTL / time.Second),
	})
}

func (srv *Server) session(r *http.Request) *Session {
	ck, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	return srv.Sessions.Get(ck.Value)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (srv *Server) me(w http.ResponseWriter, r *http.Request) {
	s := srv.session(r)
	if s == nil {
		writeJSON(w, http.StatusOK, nil) // no session yet: the client shows the nickname form
		return
	}
	srv.setCookie(w, r, s) // sliding: two more days from now
	writeJSON(w, http.StatusOK, map[string]string{"id": s.ID, "nick": s.Nick, "tag": s.Tag()})
}

func (srv *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Nick string `json:"nick"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	nick, ok := CleanNick(body.Nick)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty nick"})
		return
	}
	token := ""
	if ck, err := r.Cookie(cookieName); err == nil {
		token = ck.Value
	}
	s := srv.Sessions.Login(token, nick)
	srv.setCookie(w, r, s)
	writeJSON(w, http.StatusOK, map[string]string{"id": s.ID, "nick": s.Nick, "tag": s.Tag()})
}

// logout signs this browser out: the cookie goes and its connections close. The player stays:
// signing in with the same nickname comes back to them.
func (srv *Server) logout(w http.ResponseWriter, r *http.Request) {
	if s := srv.session(r); s != nil {
		srv.Hub.Drop(s)
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, nil)
}

func (srv *Server) ws(w http.ResponseWriter, r *http.Request) {
	s := srv.session(r)
	if s == nil {
		http.Error(w, "no session", http.StatusUnauthorized)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*.ngrok-free.app", "*.ngrok.app", "*.ngrok.io", "localhost:*", "127.0.0.1:*"},
	})
	if err != nil {
		return
	}
	srv.Hub.Serve(r.Context(), s, c)
}

// static serves the built client; unknown paths fall back to index.html (one-page app).
func (srv *Server) static(w http.ResponseWriter, r *http.Request) {
	p := filepath.Join(srv.WebDir, filepath.Clean("/"+r.URL.Path))
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		if strings.Contains(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFile(w, r, p)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(srv.WebDir, "index.html"))
}
