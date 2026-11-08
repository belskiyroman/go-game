// Command server runs the Go game: the web client, the session API and the WebSocket game server.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gogame/internal/app"
)

func main() {
	addr := flag.String("addr", ":3000", "listen address")
	web := flag.String("web", "../web/dist", "directory with the built web client")
	data := flag.String("data", "./data/sessions.json", "where sessions are kept between restarts")
	flag.Parse()

	sessions := app.NewSessionStore(*data)
	results := app.NewResultStore(app.ResultsPath(*data))
	hub := app.NewHub(sessions, results, app.GamesPath(*data))
	srv := &app.Server{Sessions: sessions, Hub: hub, WebDir: *web}
	hs := &http.Server{Addr: *addr, Handler: srv.Routes(), ReadHeaderTimeout: 10 * time.Second}

	// on stop (systemd, Ctrl+C) games and sessions are saved before exiting: after a restart
	// the games go on and the players' browsers reconnect
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		hub.SaveGames()
		sessions.Save()
		log.Print("games and sessions saved, stopping")
		os.Exit(0)
	}()

	log.Printf("listening on %s, serving %s", *addr, *web)
	log.Fatal(hs.ListenAndServe())
}
