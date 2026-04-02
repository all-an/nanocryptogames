// game.go contains shared WebSocket utilities used by the faucet game handler.
package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/allanabrahao/nanomultiplayer/internal/games"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	// Allow all origins for development; restrict in production.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// writePump reads from the player's message channel and writes to the WebSocket.
// Exits when the channel is closed (player removed from room).
func writePump(conn *websocket.Conn, p *games.Player) {
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
