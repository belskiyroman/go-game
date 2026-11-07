// Command server runs the Go game: the web client, the session API and the WebSocket game server.
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"gogame/internal/app"
)

func main() {
	addr := flag.String("addr", ":3000", "listen address")
	web := flag.String("web", "../web/dist", "directory with the built web client")
	data := flag.String("data", "./data/sessions.json", "where sessions are kept between restarts")
	flag.Parse()

	sessions := app.NewSessionStore(*data)
	srv := &app.Server{Sessions: sessions, Hub: app.NewHub(sessions), WebDir: *web}
	hs := &http.Server{Addr: *addr, Handler: srv.Routes(), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("listening on %s, serving %s", *addr, *web)
	log.Fatal(hs.ListenAndServe())
}
