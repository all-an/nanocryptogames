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
	"os"
	"time"

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
	db         *db.DB       // nil when DATABASE_URL is not configured
	masterSeed []byte       // used for Nano HD wallet derivation
	rpcClient  *nano.Client // used for the first-shot donation send
}

// NewWSHandler wires up the hub, optional DB/seed dependencies, and the Nano RPC client.
func NewWSHandler(hub *game.Hub, database *db.DB, masterSeed []byte, rpc *nano.Client) *WSHandler {
	return &WSHandler{hub: hub, db: database, masterSeed: masterSeed, rpcClient: rpc}
}

// FireDonation is the first-shot callback registered with the Hub.
// It is called in a goroutine by the room on each player's very first shot.
// DONATION_ADDRESS env var must be set to a valid nano_ address for donations to fire.
func (h *WSHandler) FireDonation(p *game.Player) {
	donationAddr := os.Getenv("DONATION_ADDRESS")
	if donationAddr == "" {
		log.Println("donation: DONATION_ADDRESS not set — skipping first-shot donation")
		return
	}

	wallet, err := nano.DeriveWallet(h.masterSeed, p.SeedIndex)
	if err != nil {
		log.Printf("donation: derive wallet index=%d: %v", p.SeedIndex, err)
		return
	}

	// Default donation: 0.001 XNO = 10^27 raw. Override with DONATION_AMOUNT_RAW.
	amountRaw := os.Getenv("DONATION_AMOUNT_RAW")
	if amountRaw == "" {
		amountRaw = "1000000000000000000000000000"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hash, err := nano.Send(ctx, h.rpcClient, wallet, donationAddr, amountRaw)
	if err != nil {
		log.Printf("donation: send failed from player %s: %v", p.NanoAddress, err)
		return
	}
	log.Printf("donation: sent from %s → %s, block %s", p.NanoAddress, donationAddr, hash)
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

	// Always derive a Nano address — master seed is guaranteed non-empty.
	// Persist the full player/session record only when a DB is connected.
	if h.db != nil {
		h.persistPlayer(r.Context(), p)
	} else if len(h.masterSeed) > 0 {
		h.deriveAddressOnly(p)
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
	p.SeedIndex = uint32(seedIndex)

	address, err := nano.DeriveAddress(h.masterSeed, p.SeedIndex)
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

// deriveAddressOnly derives a Nano address for the player without touching the DB.
// Used in local dev when DATABASE_URL is not set; the index is random so addresses
// are ephemeral and not guaranteed unique across restarts.
func (h *WSHandler) deriveAddressOnly(p *game.Player) {
	var buf [4]byte
	rand.Read(buf[:])
	p.SeedIndex = uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	address, err := nano.DeriveAddress(h.masterSeed, p.SeedIndex)
	if err != nil {
		log.Printf("wallet: derive address index=%d: %v", p.SeedIndex, err)
		return
	}
	p.NanoAddress = address
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
