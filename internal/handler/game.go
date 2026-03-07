// game.go contains the game page handler and WebSocket upgrade handler.
package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"log"
	"math/big"
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

// pushBalance queries the real on-chain balance and sends it to the player.
// Called on connect so the sidebar always shows the correct balance immediately,
// even when there are no pending blocks to receive.
func (h *WSHandler) pushBalance(ctx context.Context, p *game.Player) {
	if h.rpcClient == nil || p.NanoAddress == "" {
		return
	}
	wallet, err := nano.DeriveWallet(h.masterSeed, p.SeedIndex)
	if err != nil {
		return
	}
	info, err := h.rpcClient.GetAccountInfo(ctx, wallet.Address)
	if err != nil {
		return // account not yet opened — balance is genuinely zero
	}
	bal, ok := new(big.Int).SetString(info.Balance, 10)
	if !ok {
		return
	}
	b, _ := json.Marshal(map[string]string{
		"type": "balance",
		"xno":  game.FormatXNO(bal),
		"raw":  info.Balance,
	})
	p.Send(b)
}

// pollDeposits checks for incoming Nano to the player's session address every 30 seconds.
// Receiving pending blocks and DB recording require a DB connection, but balance display
// works even without one.
// The loop exits when ctx is cancelled (i.e. the player's WebSocket closes).
func (h *WSHandler) pollDeposits(ctx context.Context, p *game.Player) {
	if h.rpcClient == nil || p.NanoAddress == "" {
		return
	}

	// Check immediately on connect, then every 30 seconds.
	h.checkDeposits(ctx, p)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.checkDeposits(ctx, p)
		}
	}
}

// checkDeposits looks for receivable blocks on the player's address, receives them
// on-chain, updates the player's balance display, and records each deposit in the DB.
// DB recording is skipped gracefully when no DB is connected.
func (h *WSHandler) checkDeposits(ctx context.Context, p *game.Player) {
	hashes, err := h.rpcClient.Receivable(ctx, p.NanoAddress)
	if err != nil || len(hashes) == 0 {
		return
	}

	// Collect sender details before receiving (block_info only works on unconfirmed blocks).
	type pending struct {
		hash    string
		details *nano.BlockDetails
	}
	blocks := make([]pending, 0, len(hashes))
	for _, hash := range hashes {
		details, err := h.rpcClient.BlockInfo(ctx, hash)
		if err != nil {
			log.Printf("deposit: block_info %s: %v", hash[:8], err)
			continue
		}
		blocks = append(blocks, pending{hash: hash, details: details})
	}

	if len(blocks) == 0 {
		return
	}

	// Receive all pending blocks on-chain in one pass.
	wallet, err := nano.DeriveWallet(h.masterSeed, p.SeedIndex)
	if err != nil {
		log.Printf("deposit: derive wallet: %v", err)
		return
	}
	if err := nano.ReceivePending(ctx, h.rpcClient, wallet); err != nil {
		log.Printf("deposit: receive pending for %s: %v", p.NanoAddress, err)
		return
	}

	// Push the real on-chain balance and enforce the 0.001 XNO session cap.
	if info, err := h.rpcClient.GetAccountInfo(ctx, wallet.Address); err == nil {
		bal, ok := new(big.Int).SetString(info.Balance, 10)
		if ok {
			// 0.001 XNO = 10^27 raw
			maxRaw, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
			if bal.Cmp(maxRaw) > 0 {
				excess := new(big.Int).Sub(bal, maxRaw)
				// Return excess to the most recent sender.
				if len(blocks) > 0 {
					senderAddr := blocks[len(blocks)-1].details.Account
					if _, sendErr := nano.Send(ctx, h.rpcClient, wallet, senderAddr, excess.String()); sendErr != nil {
						log.Printf("deposit cap: return excess to %s: %v", senderAddr, sendErr)
					} else {
						log.Printf("deposit cap: returned %s raw excess to %s", excess, senderAddr)
						bal.Set(maxRaw)
						info.Balance = maxRaw.String()
					}
				}
			}
			b, _ := json.Marshal(map[string]string{
				"type": "balance",
				"xno":  game.FormatXNO(bal),
				"raw":  info.Balance,
			})
			p.Send(b)
		}
	}

	// Record each deposit in the DB with its sender address.
	for _, b := range blocks {
		if p.SessionID == "" {
			continue // DB not connected, skip recording
		}
		if err := h.db.RecordDeposit(ctx, p.SessionID, b.details.Account, b.details.Amount, b.hash); err != nil {
			log.Printf("deposit: record %s: %v", b.hash[:8], err)
			continue
		}
		log.Printf("deposit: player %s received %s raw from %s (block %s)",
			p.NanoAddress, b.details.Amount, b.details.Account, b.hash[:8])
	}
}

// processDonate handles a manual donation request from the player.
// It sends amountRaw from the player's session wallet to DONATION_ADDRESS.
func (h *WSHandler) processDonate(ctx context.Context, p *game.Player, amountRaw string) {
	errMsg := func(text string) {
		b, _ := json.Marshal(map[string]string{"type": "donate_err", "message": text})
		p.Send(b)
	}

	donationAddr := os.Getenv("DONATION_ADDRESS")
	if donationAddr == "" {
		errMsg("donation address is not configured")
		return
	}
	if h.rpcClient == nil {
		errMsg("RPC node not configured")
		return
	}

	amount, ok := new(big.Int).SetString(amountRaw, 10)
	if !ok || amount.Sign() <= 0 {
		errMsg("invalid amount")
		return
	}

	wallet, err := nano.DeriveWallet(h.masterSeed, p.SeedIndex)
	if err != nil {
		log.Printf("donate: derive wallet: %v", err)
		errMsg("wallet error")
		return
	}

	// Validate against real on-chain balance.
	info, err := h.rpcClient.GetAccountInfo(ctx, wallet.Address)
	if err != nil {
		errMsg("your session account has no balance")
		return
	}
	onChain, ok := new(big.Int).SetString(info.Balance, 10)
	if !ok || onChain.Cmp(amount) < 0 {
		errMsg("amount exceeds your session balance")
		return
	}

	hash, err := nano.Send(ctx, h.rpcClient, wallet, donationAddr, amountRaw)
	if err != nil {
		log.Printf("donate: send from %s: %v", p.NanoAddress, err)
		errMsg("send failed: " + err.Error())
		return
	}

	xno := game.FormatXNO(amount)
	log.Printf("donate: %s → %s (%s XNO), block %s", p.NanoAddress, donationAddr, xno, hash[:8])

	if h.db != nil && p.SessionID != "" {
		if err := h.db.RecordTransaction(ctx, p.SessionID, "donation", amountRaw, hash); err != nil {
			log.Printf("donate: record transaction: %v", err)
		}
	}

	b, _ := json.Marshal(map[string]string{
		"type":      "donate_ok",
		"xno":       xno,
		"blockHash": hash,
	})
	p.Send(b)
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

	// Log the session start regardless of DB state (no-op when db is nil).
	if h.db != nil {
		if err := h.db.LogSession(r.Context(), p.DBID, p.NanoAddress, roomID, p.Team, r.RemoteAddr); err != nil {
			log.Printf("session_log: %v", err)
		}
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

	// Push the current on-chain balance immediately so the sidebar never shows
	// stale zero even when the player reconnects with an existing balance.
	go h.pushBalance(r.Context(), p)

	// Poll for incoming Nano deposits in the background.
	// The goroutine exits automatically when the WebSocket closes (ctx cancelled).
	go h.pollDeposits(r.Context(), p)

	go writePump(conn, p)
	h.readPump(conn, p, room)

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
// Intercepts "withdraw" actions and handles them directly without entering the game loop.
// Blocks until the WebSocket connection is closed.
func (h *WSHandler) readPump(conn *websocket.Conn, p *game.Player, room *game.Room) {
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var raw struct {
			Action    string `json:"action"`
			GX        int    `json:"gx"`
			GY        int    `json:"gy"`
			TargetID  string `json:"targetID"`
			AmountRaw string `json:"amountRaw"`
		}
		if json.Unmarshal(msg, &raw) != nil {
			continue
		}

		if raw.Action == "withdraw" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			h.processWithdraw(ctx, p)
			cancel()
			continue
		}

		if raw.Action == "donate" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			h.processDonate(ctx, p, raw.AmountRaw)
			cancel()
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

// processWithdraw sends the player's full on-chain session balance back to the
// address that originally deposited Nano into the session wallet.
func (h *WSHandler) processWithdraw(ctx context.Context, p *game.Player) {
	errMsg := func(text string) {
		b, _ := json.Marshal(map[string]string{"type": "withdraw_err", "message": text})
		p.Send(b)
	}

	if h.db == nil || h.rpcClient == nil {
		errMsg("withdrawal requires a connected database and RPC node")
		return
	}
	if p.SessionID == "" {
		errMsg("no active session — reconnect with the database configured")
		return
	}

	fromAddr, err := h.db.GetDepositSender(ctx, p.SessionID)
	if err != nil {
		errMsg("no deposit on record — send Nano to your session address first")
		return
	}

	wallet, err := nano.DeriveWallet(h.masterSeed, p.SeedIndex)
	if err != nil {
		log.Printf("withdraw: derive wallet: %v", err)
		errMsg("wallet derivation error")
		return
	}

	info, err := h.rpcClient.GetAccountInfo(ctx, wallet.Address)
	if err != nil {
		errMsg("your session account has no balance to withdraw")
		return
	}

	balance, ok := new(big.Int).SetString(info.Balance, 10)
	if !ok || balance.Sign() <= 0 {
		errMsg("balance is zero")
		return
	}

	hash, err := nano.Send(ctx, h.rpcClient, wallet, fromAddr, info.Balance)
	if err != nil {
		log.Printf("withdraw: send from %s: %v", p.NanoAddress, err)
		errMsg("send failed: " + err.Error())
		return
	}

	xno := game.FormatXNO(balance)
	log.Printf("withdraw: %s → %s (%s XNO), block %s", p.NanoAddress, fromAddr, xno, hash[:8])

	// Record the withdrawal in the audit log.
	if err := h.db.RecordTransaction(ctx, p.SessionID, "withdrawal", info.Balance, hash); err != nil {
		log.Printf("withdraw: record transaction: %v", err)
	}

	b, _ := json.Marshal(map[string]string{
		"type":      "withdraw_ok",
		"toAddress": fromAddr,
		"xno":       xno,
		"blockHash": hash,
	})
	p.Send(b)
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
