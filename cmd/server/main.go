// main.go is the entry point for the nano-multiplayer server.
// It wires HTTP routes, the WebSocket hub, and static file serving.
package main

import (
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/allanabrahao/nanomultiplayer/internal/game"
	"github.com/allanabrahao/nanomultiplayer/internal/handler"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// Parse all HTML templates from disk.
	// Path is relative to the working directory (project root when using `go run ./cmd/server`).
	tmpl := template.Must(template.ParseGlob("internal/templates/*.html"))

	hub := game.NewHub()

	mux := http.NewServeMux()

	// Static assets (CSS, JS)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	// Lobby page
	mux.Handle("GET /", handler.NewHomeHandler(tmpl))

	// Game page — supports both /game?room=foo (form POST) and /game/{roomID}
	gamePage := handler.NewGamePageHandler(tmpl)
	mux.Handle("GET /game", gamePage)
	mux.Handle("GET /game/{roomID}", gamePage)

	// WebSocket endpoint — one connection per player
	mux.Handle("GET /ws/{roomID}", handler.NewWSHandler(hub))

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}
