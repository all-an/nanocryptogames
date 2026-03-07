// game.go contains the game page handler and WebSocket upgrade handler.
package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/allanabrahao/nanomultiplayer/internal/game"
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
type WSHandler struct {
	hub *game.Hub
}

// NewWSHandler wires up the hub dependency.
func NewWSHandler(hub *game.Hub) *WSHandler {
	return &WSHandler{hub: hub}
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
	room := h.hub.JoinRoom(roomID, p)

	// Tell the client its own ID and colour so it can highlight itself.
	initMsg, _ := json.Marshal(map[string]string{
		"type":  "init",
		"id":    p.ID,
		"color": p.Color,
	})
	p.Send(initMsg)

	// writePump runs concurrently; readPump blocks until the connection closes.
	go writePump(conn, p)
	readPump(conn, p, room)

	h.hub.LeaveRoom(p)
	p.Close()
}

// readPump reads client input messages and forwards them to the room's input channel.
// It blocks until the WebSocket connection is closed.
func readPump(conn *websocket.Conn, p *game.Player, room *game.Room) {
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var input struct {
			DX float64 `json:"dx"`
			DY float64 `json:"dy"`
		}
		if json.Unmarshal(msg, &input) != nil {
			continue
		}

		room.Submit(game.Input{PlayerID: p.ID, DX: input.DX, DY: input.DY})
	}
}

// writePump reads from the player's message channel and writes to the WebSocket.
// It exits when the channel is closed (player removed from room).
func writePump(conn *websocket.Conn, p *game.Player) {
	defer conn.Close()

	for msg := range p.Messages() {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// newID generates a random hex string suitable for use as a player or room ID.
func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
