// faucet.go implements the faucet multiplayer mode.
// Players play for free; kills and heals earn 0.00001 XNO paid from the faucet wallet.
// Shots are free — no deposit required from players.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/allanabrahao/nanomultiplayer/internal/db"
	"github.com/allanabrahao/nanomultiplayer/internal/game"
	"github.com/allanabrahao/nanomultiplayer/internal/nano"
	"github.com/gorilla/websocket"
)

// faucetRewardRaw is 0.00001 XNO expressed in raw Nano units (10^25 raw).
const faucetRewardRaw = "10000000000000000000000000"

// maxDailyPayoutsPerIP is the per-IP daily faucet payout cap.
// Set FAUCET_TEST_MODE=true in the environment to bypass this limit during testing.
const maxDailyPayoutsPerIP = 20

// FaucetSender serialises Nano sends from the faucet wallet and pre-caches
// proof-of-work so successive sends are near-instant.
// A single FaucetSender must be shared by every handler that sends from the same wallet.
type FaucetSender struct {
	mu             sync.Mutex
	rpc            *nano.Client
	wallet         *nano.Wallet
	cachedWork     string // pre-computed PoW for the next block
	cachedFrontier string // frontier the cached work was computed for
}

// NewFaucetSender creates a FaucetSender. wallet may be nil when FAUCET_SEED is unset.
func NewFaucetSender(rpc *nano.Client, wallet *nano.Wallet) *FaucetSender {
	return &FaucetSender{rpc: rpc, wallet: wallet}
}

// WalletAddr returns the faucet wallet address, or "" when not configured.
func (s *FaucetSender) WalletAddr() string {
	if s.wallet == nil {
		return ""
	}
	return s.wallet.Address
}

// IsConfigured reports whether wallet and RPC client are both present.
func (s *FaucetSender) IsConfigured() bool {
	return s.wallet != nil && s.rpc != nil
}

// Send executes a serialised send, using any pre-cached PoW for speed.
// After a successful send it kicks off background PoW for the next block.
func (s *FaucetSender) Send(ctx context.Context, toAddress, amountRaw string) (string, error) {
	sendCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	s.mu.Lock()
	preWork := s.cachedWork
	s.cachedWork = ""
	s.cachedFrontier = ""
	hash, err := nano.SendFast(sendCtx, s.rpc, s.wallet, toAddress, amountRaw, preWork)
	if err == nil {
		newFrontier := hash
		go func() {
			workCtx, wCancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer wCancel()
			nextWork, wErr := s.rpc.GenerateWork(workCtx, newFrontier)
			if wErr == nil {
				s.mu.Lock()
				s.cachedWork = nextWork
				s.cachedFrontier = newFrontier
				s.mu.Unlock()
				log.Printf("faucet: pre-cached work for frontier %s…", newFrontier[:8])
			}
		}()
	}
	s.mu.Unlock()
	return hash, err
}

// FaucetWelcomeHandler serves the faucet mode welcome page.
type FaucetWelcomeHandler struct {
	tmpl       *template.Template
	faucetAddr string
}

// NewFaucetWelcomeHandler wires up the template and faucet address for display.
func NewFaucetWelcomeHandler(tmpl *template.Template, faucetAddr string) *FaucetWelcomeHandler {
	return &FaucetWelcomeHandler{tmpl: tmpl, faucetAddr: faucetAddr}
}

func (h *FaucetWelcomeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.tmpl.ExecuteTemplate(w, "faucet_welcome.html", map[string]string{
		"FaucetAddress": h.faucetAddr,
		"MaxDaily":      fmt.Sprintf("%d", maxDailyPayoutsPerIP),
	})
}

// FaucetLobbyHandler serves the faucet lobby page.
type FaucetLobbyHandler struct {
	tmpl *template.Template
}

// NewFaucetLobbyHandler wires up the lobby template.
func NewFaucetLobbyHandler(tmpl *template.Template) *FaucetLobbyHandler {
	return &FaucetLobbyHandler{tmpl: tmpl}
}

func (h *FaucetLobbyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.tmpl.ExecuteTemplate(w, "faucet_lobby.html", nil)
}

// FaucetBotsPageHandler serves the bot practice mode page.
type FaucetBotsPageHandler struct {
	tmpl       *template.Template
	faucetAddr string
}

// NewFaucetBotsPageHandler wires up the bots game template.
func NewFaucetBotsPageHandler(tmpl *template.Template, faucetAddr string) *FaucetBotsPageHandler {
	return &FaucetBotsPageHandler{tmpl: tmpl, faucetAddr: faucetAddr}
}

func (h *FaucetBotsPageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.tmpl.ExecuteTemplate(w, "faucet_bots.html", map[string]string{
		"FaucetAddress": h.faucetAddr,
	})
}

// FaucetBotsRewardHandler pays a faucet reward when a player kills a bot.
// Uses the same FaucetSender (mutex + PoW cache) as FaucetWSHandler.
type FaucetBotsRewardHandler struct {
	db       *db.DB
	sender   *FaucetSender
	testMode bool
}

// NewFaucetBotsRewardHandler wires up the bot kill reward handler.
// sender must be the same FaucetSender used by FaucetWSHandler.
func NewFaucetBotsRewardHandler(database *db.DB, sender *FaucetSender) *FaucetBotsRewardHandler {
	return &FaucetBotsRewardHandler{
		db:       database,
		sender:   sender,
		testMode: os.Getenv("FAUCET_TEST_MODE") == "true",
	}
}

func (h *FaucetBotsRewardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.sender.IsConfigured() {
		log.Printf("bots reward: faucet not configured")
		http.Error(w, "faucet not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Address == "" {
		http.Error(w, "address required", http.StatusBadRequest)
		return
	}

	ip := faucetClientIP(r)
	log.Printf("bots reward: request from ip=%s addr=%.20s…", ip, req.Address)

	if !h.testMode && h.db != nil && ip != "" {
		count, err := h.db.FaucetPayoutsToday(r.Context(), ip)
		if err == nil && count >= maxDailyPayoutsPerIP {
			log.Printf("bots reward: daily limit reached for ip=%s (count=%d)", ip, count)
			http.Error(w, "daily limit reached", http.StatusTooManyRequests)
			return
		}
	}

	// Respond immediately — PoW generation can take several seconds on CPU.
	// The actual Nano send runs in the background via the shared FaucetSender.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"xno": "0.000010"})

	addr := req.Address
	go func() {
		ctx := context.Background()
		log.Printf("bots reward: sending to %s…", addr)
		hash, err := h.sender.Send(ctx, addr, faucetRewardRaw)
		if err != nil {
			log.Printf("bots reward ERROR → %s: %v", addr, err)
			return
		}
		log.Printf("bots reward: paid %s raw → %s (hash %.12s…)", faucetRewardRaw, addr, hash)
		if h.db != nil {
			dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer dbCancel()
			if err := h.db.RecordFaucetPayout(dbCtx, "bot_kill", addr, ip, faucetRewardRaw, hash); err != nil {
				log.Printf("bots reward DB: %v", err)
			}
		}
	}()
}

// FaucetGamePageHandler serves the faucet game canvas page.
type FaucetGamePageHandler struct {
	tmpl       *template.Template
	faucetAddr string
}

// NewFaucetGamePageHandler wires up the game template and faucet address for display.
func NewFaucetGamePageHandler(tmpl *template.Template, faucetAddr string) *FaucetGamePageHandler {
	return &FaucetGamePageHandler{tmpl: tmpl, faucetAddr: faucetAddr}
}

func (h *FaucetGamePageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("roomID")
	if roomID == "" {
		roomID = r.URL.Query().Get("room")
	}
	if roomID == "" {
		http.Redirect(w, r, "/faucet/lobby", http.StatusFound)
		return
	}
	h.tmpl.ExecuteTemplate(w, "faucet_game.html", map[string]string{
		"RoomID":        roomID,
		"FaucetAddress": h.faucetAddr,
	})
}

// FaucetWSHandler upgrades HTTP connections to WebSocket for faucet game sessions.
// All Nano reward sends go through the shared FaucetSender (serialised + PoW-cached).
type FaucetWSHandler struct {
	hub      *game.Hub
	db       *db.DB
	sender   *FaucetSender
	testMode bool // when true, bypass anti-abuse checks (FAUCET_TEST_MODE=true)
}

// NewFaucetWSHandler wires up all faucet WebSocket dependencies.
func NewFaucetWSHandler(hub *game.Hub, database *db.DB, sender *FaucetSender) *FaucetWSHandler {
	return &FaucetWSHandler{
		hub:      hub,
		db:       database,
		sender:   sender,
		testMode: os.Getenv("FAUCET_TEST_MODE") == "true",
	}
}

func (h *FaucetWSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	// Team — default to "red".
	team := r.URL.Query().Get("team")
	if team != "red" && team != "blue" {
		team = "red"
	}
	p.Team = team

	// Nickname: strip control chars, trim, cap at 12 runes.
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

	// Faucet address — where rewards will be sent.
	p.FaucetAddress = strings.TrimSpace(r.URL.Query().Get("address"))

	// Client IP — used for anti-abuse same-IP kill detection.
	// In test mode the IP is left blank so all checks are skipped,
	// allowing multiple tabs from the same machine to earn rewards.
	if !h.testMode {
		p.RemoteAddr = faucetClientIP(r)
	}

	// Initialise faucet fields so the room can signal reward events.
	p.FaucetRewardCh = make(chan string, 16)
	p.FaucetEarned = new(big.Int)

	// Log session to DB (best effort).
	if h.db != nil {
		if err := h.db.LogSession(r.Context(), "", "", roomID, p.Team, p.RemoteAddr, p.Nickname); err != nil {
			log.Printf("faucet session_log: %v", err)
		}
	}

	room := h.hub.JoinRoom(roomID, p)

	// Tell the client its ID, team, colour, and faucet address.
	initMsg, _ := json.Marshal(map[string]string{
		"type":          "init",
		"id":            p.ID,
		"team":          p.Team,
		"color":         p.Color,
		"nanoAddress":   p.FaucetAddress,
		"nickname":      p.Nickname,
		"faucetAddress": h.walletAddr(),
	})
	p.Send(initMsg)

	ctx, cancel := context.WithCancel(context.Background())

	// payoutLoop processes faucet reward signals from the room sequentially.
	go h.payoutLoop(ctx, p)

	go writePump(conn, p)
	h.faucetReadPump(conn, p, room)

	cancel()
	h.hub.LeaveRoom(p)
	p.Close()
	log.Printf("faucet ws [%s]: disconnected", p.ID)
}

// walletAddr returns the faucet wallet address, or empty when not configured.
func (h *FaucetWSHandler) walletAddr() string { return h.sender.WalletAddr() }

// payoutLoop reads reward signals from the room and sends on-chain payments sequentially.
// Running one payout at a time per player keeps FaucetEarned race-free and ensures
// the global sendMu is not held longer than one Nano round-trip per reward.
func (h *FaucetWSHandler) payoutLoop(ctx context.Context, p *game.Player) {
	for {
		select {
		case reason, ok := <-p.FaucetRewardCh:
			if !ok {
				return
			}
			// Use a detached context so the payment completes even if the
			// player disconnects before the on-chain send finishes.
			payCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			h.sendReward(payCtx, p, reason)
			cancel()
		case <-ctx.Done():
			// Drain any pending rewards before exiting so kills just before
			// disconnect still get paid out.
			for {
				select {
				case reason, ok := <-p.FaucetRewardCh:
					if !ok {
						return
					}
					payCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
					h.sendReward(payCtx, p, reason)
					cancel()
				default:
					return
				}
			}
		}
	}
}

// sendReward executes a single faucet payout: daily-limit check → nano.Send → notify player.
func (h *FaucetWSHandler) sendReward(ctx context.Context, p *game.Player, reason string) {
	if !h.sender.IsConfigured() {
		return
	}
	if p.FaucetAddress == "" {
		log.Printf("faucet payout [%s]: no address provided, skipping", p.ID)
		return
	}

	// Enforce daily per-IP limit unless test mode is active.
	if !h.testMode && h.db != nil && p.RemoteAddr != "" {
		count, err := h.db.FaucetPayoutsToday(ctx, p.RemoteAddr)
		if err != nil {
			log.Printf("faucet payout [%s]: daily limit check: %v", p.ID, err)
		} else if count >= maxDailyPayoutsPerIP {
			log.Printf("faucet payout [%s]: daily limit %d reached for IP %s", p.ID, count, p.RemoteAddr)
			b, _ := json.Marshal(map[string]string{
				"type":    "faucet_limit",
				"message": "Daily faucet limit reached. Come back tomorrow!",
			})
			p.Send(b)
			return
		}
	}

	// All sends are serialised inside FaucetSender; pre-cached PoW makes them near-instant.
	hash, err := h.sender.Send(ctx, p.FaucetAddress, faucetRewardRaw)

	if err != nil {
		log.Printf("faucet payout [%s] %s → %s: %v", p.ID, reason, p.FaucetAddress, err)
		b, _ := json.Marshal(map[string]string{
			"type":    "faucet_err",
			"message": "Payout failed — faucet may be empty. Try again later.",
		})
		p.Send(b)
		return
	}

	rewardAmt, _ := new(big.Int).SetString(faucetRewardRaw, 10)
	p.FaucetEarned.Add(p.FaucetEarned, rewardAmt)

	log.Printf("faucet payout [%s] %s: 0.00001 XNO → %s block %s", p.ID, reason, p.FaucetAddress, hash[:8])

	b, _ := json.Marshal(map[string]string{
		"type":   "faucet_reward",
		"reason": reason,
		"xno":    "0.000010",
		"earned": game.FormatXNO(p.FaucetEarned),
	})
	p.Send(b)

	// Audit trail (best effort).
	if h.db != nil {
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dbCancel()
		if err := h.db.RecordFaucetPayout(dbCtx, reason, p.FaucetAddress, p.RemoteAddr, faucetRewardRaw, hash); err != nil {
			log.Printf("faucet payout [%s]: DB record: %v", p.ID, err)
		}
	}
}

// faucetReadPump reads WebSocket messages and dispatches game actions.
// Unlike the paid-mode readPump there is no withdraw or deposit handling.
func (h *FaucetWSHandler) faucetReadPump(conn *websocket.Conn, p *game.Player, room *game.Room) {
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

		switch raw.Action {
		case "move", "shoot", "help", "reload":
			room.Submit(game.Input{
				PlayerID: p.ID,
				Action:   raw.Action,
				GX:       raw.GX,
				GY:       raw.GY,
				TargetID: raw.TargetID,
			})
		}
	}
}

// faucetClientIP extracts the real client IP from the request, respecting
// X-Forwarded-For and X-Real-IP headers for reverse-proxy deployments.
func faucetClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i != -1 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
