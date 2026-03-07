// game.go contains the game page handler and WebSocket upgrade handler.
package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"log"
	"net/http"

	"github.com/allanabrahao/nanomultiplayer/internal/db"
	"github.com/allanabrahao/nanomultiplayer/internal/game"
	"github.com/allanabrahao/nanomultiplayer/internal/nano"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	// Allow all origins for development; restrict in production.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// GamePageHandler serves the HTML page containing the game canvas.
type GamePageHandler struct {
	tmpl *template.Template
}

// NewGamePageHandler wires up the game template.
func NewGamePageHandler(tmpl *template.Template) *GamePageHandler {
	return &GamePageHandler{tmpl: tmpl}
}

func (h *GamePageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Room ID comes from the path (/game/{roomID}) or query string (/game?room=foo).
	roomID := r.PathValue("roomID")
	if roomID == "" {
		roomID = r.URL.Query().Get("room")
	}
	if roomID == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	h.tmpl.ExecuteTemplate(w, "game.html", map[string]string{"RoomID": roomID})
}

// WSHandler upgrades HTTP connections to WebSocket and drives the player I/O pumps.
// db and masterSeed are optional — when nil/empty the game runs without persistence.
type WSHandler struct {
	hub        *game.Hub
	db         *db.DB // nil when DATABASE_URL is not configured
	masterSeed []byte // used for Nano HD wallet derivation
}

// NewWSHandler wires up the hub and optional DB/seed dependencies.
func NewWSHandler(hub *game.Hub, database *db.DB, masterSeed []byte) *WSHandler {
	return &WSHandler{hub: hub, db: database, masterSeed: masterSeed}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("roomID")
	if roomID == "" {
		http.Error(w, "missing room ID", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	p := game.NewPlayer(newID(), roomID)

	// Team is chosen in the lobby and passed as a query param.
	// Default to "red" if missing or invalid.
	team := r.URL.Query().Get("team")
	if team != "red" && team != "blue" {
		team = "red"
	}
	p.Team = team

	// Persist player and session when DB is available.
	if h.db != nil && len(h.masterSeed) > 0 {
		h.persistPlayer(r.Context(), p)
	}

	room := h.hub.JoinRoom(roomID, p)

	// Tell the client its own ID, team, colour, and Nano address.
	initMsg, _ := json.Marshal(map[string]string{
		"type":        "init",
		"id":          p.ID,
		"team":        p.Team,
		"color":       p.Color,
		"nanoAddress": p.NanoAddress,
	})
	p.Send(initMsg)

	go writePump(conn, p)
	readPump(conn, p, room)

	h.hub.LeaveRoom(p)
	p.Close()
}

// persistPlayer derives a Nano wallet address and creates DB records for the player.
// Failures are logged but do not block the WebSocket connection.
func (h *WSHandler) persistPlayer(ctx context.Context, p *game.Player) {
	seedIndex, err := h.db.NextSeedIndex(ctx)
	if err != nil {
		log.Printf("wallet: next seed index: %v", err)
		return
	}

	address, err := nano.DeriveAddress(h.masterSeed, uint32(seedIndex))
	if err != nil {
		log.Printf("wallet: derive address index=%d: %v", seedIndex, err)
		return
	}
	p.NanoAddress = address

	dbID, err := h.db.CreatePlayer(ctx, address, seedIndex)
	if err != nil {
		log.Printf("db: create player: %v", err)
		return
	}
	p.DBID = dbID

	sessionID, err := h.db.CreateSession(ctx, p.RoomID, dbID)
	if err != nil {
		log.Printf("db: create session: %v", err)
		return
	}
	p.SessionID = sessionID
}

// readPump reads client input messages and forwards them to the room's input channel.
// Blocks until the WebSocket connection is closed.
func readPump(conn *websocket.Conn, p *game.Player, room *game.Room) {
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var raw struct {
			Action   string `json:"action"`
			GX       int    `json:"gx"`
			GY       int    `json:"gy"`
			TargetID string `json:"targetID"`
		}
		if json.Unmarshal(msg, &raw) != nil {
			continue
		}

		room.Submit(game.Input{
			PlayerID: p.ID,
			Action:   raw.Action,
			GX:       raw.GX,
			GY:       raw.GY,
			TargetID: raw.TargetID,
		})
	}
}

// writePump reads from the player's message channel and writes to the WebSocket.
// Exits when the channel is closed (player removed from room).
func writePump(conn *websocket.Conn, p *game.Player) {
	defer conn.Close()

	for msg := range p.Messages() {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// newID generates a random hex string used as a short player ID over WebSocket.
func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
