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
	"strings"
	"time"
	"unicode"

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
	h.tmpl.ExecuteTemplate(w, "paid_game.html", map[string]string{"RoomID": roomID})
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
		log.Printf("pushBalance [%s]: derive wallet: %v", p.NanoAddress, err)
		return
	}
	log.Printf("pushBalance [%s]: querying on-chain balance", p.NanoAddress)
	info, err := h.rpcClient.GetAccountInfo(ctx, wallet.Address)
	if err != nil {
		log.Printf("pushBalance [%s]: account not opened yet (balance=0): %v", p.NanoAddress, err)
		return // account not yet opened — balance is genuinely zero
	}
	bal, ok := new(big.Int).SetString(info.Balance, 10)
	if !ok {
		log.Printf("pushBalance [%s]: invalid balance string %q", p.NanoAddress, info.Balance)
		return
	}
	log.Printf("pushBalance [%s]: balance=%s raw (%s XNO)", p.NanoAddress, info.Balance, game.FormatXNO(bal))
	b, _ := json.Marshal(map[string]string{
		"type": "balance",
		"xno":  game.FormatXNO(bal),
		"raw":  info.Balance,
	})
	p.Send(b)
}

// pollDeposits checks for incoming Nano to the player's session address every 10 seconds.
// Receiving pending blocks and DB recording require a DB connection, but balance display
// works even without one.
// The loop exits when ctx is cancelled (i.e. the player's WebSocket closes).
func (h *WSHandler) pollDeposits(ctx context.Context, p *game.Player) {
	if h.rpcClient == nil || p.NanoAddress == "" {
		log.Printf("pollDeposits [%s]: rpcClient or address not set, skipping", p.ID)
		return
	}

	log.Printf("pollDeposits [%s]: starting for address %s", p.ID, p.NanoAddress)

	// Check immediately on connect, then every 10 seconds.
	h.checkDeposits(ctx, p)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("pollDeposits [%s]: context cancelled, stopping poll loop", p.NanoAddress)
			return
		case <-ticker.C:
			log.Printf("pollDeposits [%s]: tick — checking for deposits", p.NanoAddress)
			h.checkDeposits(ctx, p)
		}
	}
}

// checkDeposits looks for receivable blocks on the player's address, receives them
// on-chain, updates the player's balance display, and records each deposit in the DB.
// DB recording is skipped gracefully when no DB is connected.
func (h *WSHandler) checkDeposits(ctx context.Context, p *game.Player) {
	log.Printf("checkDeposits [%s]: querying receivable blocks", p.NanoAddress)
	hashes, err := h.rpcClient.Receivable(ctx, p.NanoAddress)
	if err != nil {
		log.Printf("checkDeposits [%s]: receivable query error: %v", p.NanoAddress, err)
		return
	}
	if len(hashes) == 0 {
		log.Printf("checkDeposits [%s]: no pending blocks", p.NanoAddress)
		return
	}
	log.Printf("checkDeposits [%s]: %d pending block(s) found", p.NanoAddress, len(hashes))

	// Collect sender details for the DB audit trail (best-effort).
	// BlockInfo failures are logged but do NOT block receiving — we always
	// proceed to ReceivePending regardless of how many details we obtained.
	type pending struct {
		hash    string
		details *nano.BlockDetails
	}
	blocks := make([]pending, 0, len(hashes))
	for _, hash := range hashes {
		log.Printf("checkDeposits [%s]: fetching block_info for %s", p.NanoAddress, hash[:8])
		details, err := h.rpcClient.BlockInfo(ctx, hash)
		if err != nil {
			log.Printf("checkDeposits [%s]: block_info %s error: %v (will still receive)", p.NanoAddress, hash[:8], err)
			continue
		}
		log.Printf("checkDeposits [%s]: block %s — amount=%s raw from %s", p.NanoAddress, hash[:8], details.Amount, details.Account)
		blocks = append(blocks, pending{hash: hash, details: details})
	}

	// Receive all pending blocks on-chain in one pass.
	log.Printf("checkDeposits [%s]: deriving wallet (seed index %d)", p.NanoAddress, p.SeedIndex)
	wallet, err := nano.DeriveWallet(h.masterSeed, p.SeedIndex)
	if err != nil {
		log.Printf("checkDeposits [%s]: derive wallet: %v", p.NanoAddress, err)
		return
	}
	log.Printf("checkDeposits [%s]: calling ReceivePending", p.NanoAddress)
	if err := nano.ReceivePending(ctx, h.rpcClient, wallet); err != nil {
		log.Printf("checkDeposits [%s]: ReceivePending error: %v", p.NanoAddress, err)
		return
	}
	log.Printf("checkDeposits [%s]: ReceivePending succeeded", p.NanoAddress)

	// Push the real on-chain balance and enforce the 0.001 XNO session cap.
	info, err := h.rpcClient.GetAccountInfo(ctx, wallet.Address)
	if err != nil {
		log.Printf("checkDeposits [%s]: GetAccountInfo after receive: %v", p.NanoAddress, err)
	} else {
		bal, ok := new(big.Int).SetString(info.Balance, 10)
		if ok {
			log.Printf("checkDeposits [%s]: post-receive balance=%s raw (%s XNO)", p.NanoAddress, info.Balance, game.FormatXNO(bal))
			// 0.001 XNO = 10^27 raw
			maxRaw, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
			if bal.Cmp(maxRaw) > 0 {
				excess := new(big.Int).Sub(bal, maxRaw)
				log.Printf("checkDeposits [%s]: balance exceeds cap — returning %s raw excess", p.NanoAddress, excess)
				// Return excess to the most recent sender.
				if len(blocks) > 0 {
					senderAddr := blocks[len(blocks)-1].details.Account
					if _, sendErr := nano.Send(ctx, h.rpcClient, wallet, senderAddr, excess.String()); sendErr != nil {
						log.Printf("checkDeposits [%s]: cap return to %s failed: %v", p.NanoAddress, senderAddr, sendErr)
					} else {
						log.Printf("checkDeposits [%s]: returned %s raw excess to %s", p.NanoAddress, excess, senderAddr)
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
			log.Printf("checkDeposits [%s]: sent balance update to player (%s XNO)", p.NanoAddress, game.FormatXNO(bal))
		}
	}

	// Record each deposit in the DB with its sender address.
	for _, b := range blocks {
		if p.SessionID == "" {
			log.Printf("checkDeposits [%s]: no session ID, skipping DB record for block %s", p.NanoAddress, b.hash[:8])
			continue // DB not connected, skip recording
		}
		if err := h.db.RecordDeposit(ctx, p.SessionID, b.details.Account, b.details.Amount, b.hash); err != nil {
			log.Printf("checkDeposits [%s]: DB record for block %s failed: %v", p.NanoAddress, b.hash[:8], err)
			continue
		}
		log.Printf("checkDeposits [%s]: recorded deposit — %s raw from %s (block %s)",
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

	// Nickname: strip control characters, trim whitespace, cap at 12 runes.
	nick := strings.TrimSpace(r.URL.Query().Get("nick"))
	var nickRunes []rune
	for _, ch := range nick {
		if unicode.IsPrint(ch) {
			nickRunes = append(nickRunes, ch)
		}
	}
	if len(nickRunes) > 12 {
		nickRunes = nickRunes[:12]
	}
	if len(nickRunes) == 0 {
		http.Error(w, "nickname is required", http.StatusBadRequest)
		return
	}
	p.Nickname = string(nickRunes)

	// Always derive a Nano address — master seed is guaranteed non-empty.
	// Persist the full player/session record only when a DB is connected.
	if h.db != nil {
		h.persistPlayer(r.Context(), p)
	} else if len(h.masterSeed) > 0 {
		h.deriveAddressOnly(p)
	}

	// Log the session start regardless of DB state (no-op when db is nil).
	if h.db != nil {
		if err := h.db.LogSession(r.Context(), p.DBID, p.NanoAddress, roomID, p.Team, r.RemoteAddr, p.Nickname); err != nil {
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
		"nickname":    p.Nickname,
	})
	p.Send(initMsg)

	// Create a dedicated context for background goroutines tied to this WebSocket session.
	// We use context.Background() as the parent so that after the WebSocket upgrade
	// the goroutines are not affected by ambiguities in r.Context() lifetime.
	wsCtx, wsCancel := context.WithCancel(context.Background())

	// Push the current on-chain balance immediately so the sidebar never shows
	// stale zero even when the player reconnects with an existing balance.
	go h.pushBalance(wsCtx, p)

	// Poll for incoming Nano deposits in the background.
	// The goroutine exits when wsCancel is called after the WebSocket closes.
	go h.pollDeposits(wsCtx, p)

	go writePump(conn, p)
	h.readPump(conn, p, room)

	log.Printf("ws [%s]: WebSocket closed, stopping background goroutines", p.NanoAddress)
	wsCancel() // Stop background polling goroutines.

	h.hub.LeaveRoom(p)

	// Auto-return any remaining session balance to the original deposit sender.
	// Runs in a goroutine so p.Close() is not blocked on network I/O.
	go h.autoReturnFunds(p)

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

		if raw.Action == "refresh_balance" {
			go func() {
				ctx2, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
				h.checkDeposits(ctx2, p)
				h.pushBalance(ctx2, p)
			}()
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

// autoReturnFunds is called when a player's WebSocket closes.
// It first receives any pending blocks that arrived just before disconnect, then
// sends the full session balance back to the original deposit sender so funds are
// never stranded in an inaccessible temporary wallet.
// Runs in a goroutine; uses an independent context so it is not affected by
// the session context being cancelled on disconnect.
func (h *WSHandler) autoReturnFunds(p *game.Player) {
	if h.db == nil || h.rpcClient == nil || p.NanoAddress == "" || p.SessionID == "" {
		log.Printf("auto-return [%s]: skipping (db=%v rpc=%v addr=%q session=%q)",
			p.ID, h.db != nil, h.rpcClient != nil, p.NanoAddress, p.SessionID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	wallet, err := nano.DeriveWallet(h.masterSeed, p.SeedIndex)
	if err != nil {
		log.Printf("auto-return [%s]: derive wallet: %v", p.NanoAddress, err)
		return
	}

	// Receive any blocks that arrived just before the WebSocket closed.
	log.Printf("auto-return [%s]: checking for pending blocks before return", p.NanoAddress)
	if err := nano.ReceivePending(ctx, h.rpcClient, wallet); err != nil {
		log.Printf("auto-return [%s]: receive pending: %v (continuing anyway)", p.NanoAddress, err)
	} else {
		log.Printf("auto-return [%s]: ReceivePending completed", p.NanoAddress)
	}

	info, err := h.rpcClient.GetAccountInfo(ctx, wallet.Address)
	if err != nil {
		log.Printf("auto-return [%s]: account not opened — nothing to return", p.NanoAddress)
		return
	}
	balance, ok := new(big.Int).SetString(info.Balance, 10)
	if !ok || balance.Sign() <= 0 {
		log.Printf("auto-return [%s]: balance is zero — nothing to return", p.NanoAddress)
		return
	}
	log.Printf("auto-return [%s]: balance to return = %s raw (%s XNO)", p.NanoAddress, info.Balance, game.FormatXNO(balance))

	fromAddr, err := h.db.GetDepositSender(ctx, p.SessionID)
	if err != nil {
		log.Printf("auto-return [%s]: no deposit sender on record for session %s: %v — funds remain in wallet (recoverable via master seed)",
			p.NanoAddress, p.SessionID, err)
		return
	}
	log.Printf("auto-return [%s]: returning %s XNO to original sender %s", p.NanoAddress, game.FormatXNO(balance), fromAddr)

	hash, err := nano.Send(ctx, h.rpcClient, wallet, fromAddr, info.Balance)
	if err != nil {
		log.Printf("auto-return [%s]: send to %s failed: %v", p.NanoAddress, fromAddr, err)
		return
	}
	log.Printf("auto-return [%s]: returned %s XNO to %s on disconnect, block %s",
		p.NanoAddress, game.FormatXNO(balance), fromAddr, hash[:8])

	if err := h.db.RecordTransaction(ctx, p.SessionID, "withdrawal", info.Balance, hash); err != nil {
		log.Printf("auto-return [%s]: record transaction: %v", p.NanoAddress, err)
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
