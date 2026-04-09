// farm.go implements HTTP handlers for the Nano Faucet Multiplayer Farm.
// Routes: login, register, logout, game page, WebSocket, balance, withdraw.
package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/allanabrahao/nanomultiplayer/internal/db"
	"github.com/allanabrahao/nanomultiplayer/internal/games/shooter"
	farmgame "github.com/allanabrahao/nanomultiplayer/internal/games/farm"
	"github.com/allanabrahao/nanomultiplayer/internal/nano"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

const (
	farmCookieName   = "farm_session"
	farmCookieMaxAge = 7 * 24 * 60 * 60 // 7 days in seconds
	farmMinUsername  = 3
	farmMaxUsername  = 20
	farmMinPassword  = 6
	farmDefaultRoom  = "0,0"
)

// ── in-memory fallback stores (used when DATABASE_URL is not set) ─────────────

type farmMemAccount struct {
	ID           string
	Username     string
	PasswordHash string
	Email        *string
	Color        string
	SeedIndex    int
	NanoAddress  string
}

type farmMemAccounts struct {
	mu      sync.RWMutex
	byName  map[string]*farmMemAccount
	byID    map[string]*farmMemAccount
	nextIdx int
}

func newFarmMemAccounts() *farmMemAccounts {
	return &farmMemAccounts{
		byName: make(map[string]*farmMemAccount),
		byID:   make(map[string]*farmMemAccount),
	}
}

func (m *farmMemAccounts) create(username, passwordHash, nanoAddress string, email *string, seedIndex int) *farmMemAccount {
	id := newID()
	acc := &farmMemAccount{
		ID: id, Username: username,
		PasswordHash: passwordHash, Email: email, Color: "", SeedIndex: seedIndex, NanoAddress: nanoAddress,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byName[username] = acc
	m.byID[id] = acc
	return acc
}

func (m *farmMemAccounts) getByName(username string) (*farmMemAccount, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	acc, ok := m.byName[username]
	return acc, ok
}

func (m *farmMemAccounts) getByID(id string) (*farmMemAccount, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	acc, ok := m.byID[id]
	return acc, ok
}

func (m *farmMemAccounts) updateAccount(id, username string, email *string, color string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.byID[id]
	if !ok {
		return
	}
	// Keep byName consistent when the username changes.
	if acc.Username != username {
		delete(m.byName, acc.Username)
		m.byName[username] = acc
		acc.Username = username
	}
	acc.Email = email
	acc.Color = color
}

func (m *farmMemAccounts) nextIndex() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.nextIdx
	m.nextIdx++
	return idx
}

type farmSessionData struct {
	AccountID   string
	Username    string
	NanoAddress string
	Email       string // empty when not set
	Color       string // empty means use palette
	SeedIndex   int
}

type farmMemSessions struct {
	mu       sync.RWMutex
	sessions map[string]*farmSessionData
}

func newFarmMemSessions() *farmMemSessions {
	return &farmMemSessions{sessions: make(map[string]*farmSessionData)}
}

func (s *farmMemSessions) set(token string, d *farmSessionData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = d
}

func (s *farmMemSessions) get(token string) (*farmSessionData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.sessions[token]
	return d, ok
}

func (s *farmMemSessions) delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

// ── main handler ──────────────────────────────────────────────────────────────

// FarmHandler holds shared state for all Nano Faucet Multiplayer Farm endpoints.
type FarmHandler struct {
	tmpl       *template.Template
	db         *db.DB
	rpc        *nano.Client
	masterSeed []byte
	hub        *farmgame.Hub
	memAccts   *farmMemAccounts
	memSess    *farmMemSessions
}

// NewFarmHandler wires up all Farm handler dependencies.
func NewFarmHandler(tmpl *template.Template, database *db.DB, rpc *nano.Client, masterSeed []byte) *FarmHandler {
	return &FarmHandler{
		tmpl:       tmpl,
		db:         database,
		rpc:        rpc,
		masterSeed: masterSeed,
		hub:        farmgame.NewHub(),
		memAccts:   newFarmMemAccounts(),
		memSess:    newFarmMemSessions(),
	}
}

// ── session helpers ───────────────────────────────────────────────────────────

// session returns the authenticated session data for the request, or nil.
func (h *FarmHandler) session(r *http.Request) *farmSessionData {
	cookie, err := r.Cookie(farmCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	token := cookie.Value

	if h.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		accountID, err := h.db.GetFarmSession(ctx, token)
		if err != nil {
			return nil
		}
		acc, err := h.db.GetFarmAccountByID(ctx, accountID)
		if err != nil {
			return nil
		}
		sess := &farmSessionData{
			AccountID: acc.ID, Username: acc.Username,
			NanoAddress: acc.NanoAddress, Color: acc.Color, SeedIndex: acc.SeedIndex,
		}
		if acc.Email != nil {
			sess.Email = *acc.Email
		}
		return sess
	}

	d, ok := h.memSess.get(token)
	if !ok {
		return nil
	}
	return d
}

// newSessionToken generates a random 32-byte hex string for use as a session token.
func newSessionToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func setFarmCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     farmCookieName,
		Value:    token,
		Path:     "/farm",
		MaxAge:   farmCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearFarmCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     farmCookieName,
		Value:    "",
		Path:     "/farm",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// nextSeedIndex gets the next HD wallet index from DB or the in-memory counter.
func (h *FarmHandler) nextSeedIndex(ctx context.Context) (int, error) {
	if h.db != nil {
		return h.db.NextSeedIndex(ctx)
	}
	return h.memAccts.nextIndex(), nil
}

// validateUsername checks length and allowed characters. Returns an error message or "".
func validateUsername(u string) string {
	runes := []rune(strings.TrimSpace(u))
	if len(runes) < farmMinUsername {
		return fmt.Sprintf("Username must be at least %d characters", farmMinUsername)
	}
	if len(runes) > farmMaxUsername {
		return fmt.Sprintf("Username must be at most %d characters", farmMaxUsername)
	}
	for _, ch := range runes {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' && ch != '-' {
			return "Username may only contain letters, digits, _ and -"
		}
	}
	return ""
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

// LoginPage renders the combined login/register page (GET /farm).
func (h *FarmHandler) LoginPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.session(r) != nil {
			http.Redirect(w, r, "/farm/game", http.StatusFound)
			return
		}
		h.tmpl.ExecuteTemplate(w, "farm_login.html", map[string]string{
			"Error": r.URL.Query().Get("error"),
		})
	}
}

// Register handles POST /farm/register — create a new account and session.
func (h *FarmHandler) Register() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		emailRaw := strings.TrimSpace(r.FormValue("email"))

		// Normalise email: store NULL when blank, reject obviously invalid values.
		var email *string
		if emailRaw != "" {
			if !strings.Contains(emailRaw, "@") {
				http.Redirect(w, r, "/farm?error=Invalid+email+address", http.StatusSeeOther)
				return
			}
			email = &emailRaw
		}

		if msg := validateUsername(username); msg != "" {
			http.Redirect(w, r, "/farm?error="+urlEncode(msg), http.StatusSeeOther)
			return
		}
		if len(password) < farmMinPassword {
			http.Redirect(w, r, fmt.Sprintf("/farm?error=Password+must+be+at+least+%d+characters", farmMinPassword), http.StatusSeeOther)
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("farm register: bcrypt: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()
		idx, err := h.nextSeedIndex(ctx)
		if err != nil {
			log.Printf("farm register: seed index: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		wallet, err := nano.DeriveWallet(h.masterSeed, uint32(idx))
		if err != nil {
			log.Printf("farm register: derive wallet: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		var accountID string
		if h.db != nil {
			acc, err := h.db.CreateFarmAccount(ctx, username, string(hash), email, idx, wallet.Address)
			if err != nil {
				log.Printf("farm register: create account: %v", err)
				http.Redirect(w, r, "/farm?error=Username+or+email+already+taken", http.StatusSeeOther)
				return
			}
			accountID = acc.ID
		} else {
			if _, exists := h.memAccts.getByName(username); exists {
				http.Redirect(w, r, "/farm?error=Username+already+taken", http.StatusSeeOther)
				return
			}
			acc := h.memAccts.create(username, string(hash), wallet.Address, email, idx)
			accountID = acc.ID
		}

		log.Printf("farm: new account %q wallet %s", username, wallet.Address)

		token := newSessionToken()
		if h.db != nil {
			if err := h.db.CreateFarmSession(ctx, token, accountID); err != nil {
				log.Printf("farm register: create session: %v", err)
			}
		} else {
			h.memSess.set(token, &farmSessionData{
				AccountID: accountID, Username: username,
				NanoAddress: wallet.Address, Color: "", SeedIndex: idx,
			})
		}

		setFarmCookie(w, token)
		http.Redirect(w, r, "/farm/game", http.StatusSeeOther)
	}
}

// Login handles POST /farm/login — verify credentials and create a session.
func (h *FarmHandler) Login() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		ctx := r.Context()

		var accountID, nanoAddress, passwordHash, color string
		var seedIndex int

		if h.db != nil {
			acc, err := h.db.GetFarmAccountByUsername(ctx, username)
			if err != nil {
				http.Redirect(w, r, "/farm?error=Invalid+username+or+password", http.StatusSeeOther)
				return
			}
			accountID, passwordHash = acc.ID, acc.PasswordHash
			nanoAddress, seedIndex, color = acc.NanoAddress, acc.SeedIndex, acc.Color
		} else {
			acc, ok := h.memAccts.getByName(username)
			if !ok {
				http.Redirect(w, r, "/farm?error=Invalid+username+or+password", http.StatusSeeOther)
				return
			}
			accountID, passwordHash = acc.ID, acc.PasswordHash
			nanoAddress, seedIndex, color = acc.NanoAddress, acc.SeedIndex, acc.Color
		}

		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
			http.Redirect(w, r, "/farm?error=Invalid+username+or+password", http.StatusSeeOther)
			return
		}

		token := newSessionToken()
		if h.db != nil {
			if err := h.db.CreateFarmSession(ctx, token, accountID); err != nil {
				log.Printf("farm login: create session: %v", err)
			}
		} else {
			h.memSess.set(token, &farmSessionData{
				AccountID: accountID, Username: username,
				NanoAddress: nanoAddress, Color: color, SeedIndex: seedIndex,
			})
		}

		setFarmCookie(w, token)
		http.Redirect(w, r, "/farm/game", http.StatusSeeOther)
	}
}

// Logout handles POST /farm/logout — destroy the session and redirect to login.
func (h *FarmHandler) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(farmCookieName)
		if err == nil && cookie.Value != "" {
			if h.db != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				defer cancel()
				h.db.DeleteFarmSession(ctx, cookie.Value)
			} else {
				h.memSess.delete(cookie.Value)
			}
		}
		clearFarmCookie(w)
		http.Redirect(w, r, "/farm", http.StatusSeeOther)
	}
}

// GamePage renders the Farm game canvas (GET /farm/game, requires auth).
func (h *FarmHandler) GamePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := h.session(r)
		if sess == nil {
			http.Redirect(w, r, "/farm", http.StatusFound)
			return
		}
		room := r.URL.Query().Get("room")
		if room == "" {
			room = farmDefaultRoom
		}
		ex, ey := parseEntryCoords(r)
		h.tmpl.ExecuteTemplate(w, "farm_game.html", map[string]any{
			"Username":    sess.Username,
			"NanoAddress": sess.NanoAddress,
			"Email":       sess.Email,
			"Color":       sess.Color,
			"Room":        room,
			"EntryX":      ex,
			"EntryY":      ey,
		})
	}
}

// WebSocket handles GET /farm/ws — upgrades to WebSocket, joins a room, starts pumps.
func (h *FarmHandler) WebSocket() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := h.session(r)
		if sess == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		room := r.URL.Query().Get("room")
		if room == "" {
			room = farmDefaultRoom
		}

		p := farmgame.NewPlayer(newID(), sess.AccountID, sess.Username, sess.NanoAddress, room, sess.Color, sess.SeedIndex)
		ex, ey := parseEntryCoords(r)
		joinedRoom := h.hub.JoinRoom(room, p, ex, ey)

		initMsg, _ := json.Marshal(map[string]any{
			"type":        "init",
			"id":          p.ID,
			"username":    p.Username,
			"nanoAddress": p.NanoAddress,
			"color":       p.Color,
			"room":        room,
			"gridW":       farmgame.GridW,
			"gridH":       farmgame.GridH,
			"blocks":      joinedRoom.Blocks(),
			"doors":       joinedRoom.Doors(),
		})
		p.Send(initMsg)

		go farmWritePump(conn, p)
		farmReadPump(conn, p, joinedRoom)

		h.hub.LeaveRoom(room, p.ID)
		p.Close()
		log.Printf("farm ws [%s/%s]: disconnected", room, sess.Username)
	}
}

// Balance handles GET /farm/api/balance — returns the player's game wallet balance as JSON.
func (h *FarmHandler) Balance() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := h.session(r)
		if sess == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if h.rpc == nil {
			json.NewEncoder(w).Encode(map[string]string{"xno": "0.000000", "raw": "0"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		bal, err := h.rpc.GetBalance(ctx, sess.NanoAddress)
		if err != nil {
			// Account not opened yet — return zero balance.
			json.NewEncoder(w).Encode(map[string]string{"xno": "0.000000", "raw": "0"})
			return
		}

		raw, _ := new(big.Int).SetString(bal.Balance, 10)
		if raw == nil {
			raw = new(big.Int)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"xno": shooter.FormatXNO(raw),
			"raw": bal.Balance,
		})
	}
}

// Withdraw handles POST /farm/withdraw — sends the player's game wallet balance to an address.
func (h *FarmHandler) Withdraw() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := h.session(r)
		if sess == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if h.rpc == nil || len(h.masterSeed) == 0 {
			http.Error(w, "nano not configured", http.StatusServiceUnavailable)
			return
		}

		var req struct {
			ToAddress string `json:"toAddress"`
			AmountRaw string `json:"amountRaw"` // leave empty to send full balance
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ToAddress == "" {
			http.Error(w, "toAddress required", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(req.ToAddress, "nano_") && !strings.HasPrefix(req.ToAddress, "xrb_") {
			http.Error(w, "invalid nano address", http.StatusBadRequest)
			return
		}

		wallet, err := nano.DeriveWallet(h.masterSeed, uint32(sess.SeedIndex))
		if err != nil {
			log.Printf("farm withdraw: derive wallet: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		amountRaw := req.AmountRaw
		if amountRaw == "" {
			info, err := h.rpc.GetAccountInfo(ctx, wallet.Address)
			if err != nil {
				http.Error(w, "could not read balance: "+err.Error(), http.StatusBadGateway)
				return
			}
			amountRaw = info.Balance
		}
		if amountRaw == "0" || amountRaw == "" {
			http.Error(w, "nothing to withdraw", http.StatusBadRequest)
			return
		}

		hash, err := nano.Send(ctx, h.rpc, wallet, req.ToAddress, amountRaw)
		if err != nil {
			log.Printf("farm withdraw [%s]: %v", sess.Username, err)
			http.Error(w, "send failed: "+err.Error(), http.StatusBadGateway)
			return
		}

		raw, _ := new(big.Int).SetString(amountRaw, 10)
		if raw == nil {
			raw = new(big.Int)
		}
		log.Printf("farm withdraw [%s]: %s raw → %s block %s", sess.Username, amountRaw, req.ToAddress, hash[:8])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"hash": hash,
			"xno":  shooter.FormatXNO(raw),
		})
	}
}

// UpdateAccount handles POST /farm/account — updates the player's email address.
func (h *FarmHandler) UpdateAccount() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := h.session(r)
		if sess == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req struct {
			Username string `json:"username"`
			Email    string `json:"email"`
			Color    string `json:"color"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		newUsername := strings.TrimSpace(req.Username)
		if msg := validateUsername(newUsername); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}

		emailRaw := strings.TrimSpace(req.Email)
		if emailRaw == "" || !strings.Contains(emailRaw, "@") {
			http.Error(w, "valid email is required", http.StatusBadRequest)
			return
		}

		color := strings.TrimSpace(req.Color)
		if !isValidHexColor(color) {
			http.Error(w, "invalid color", http.StatusBadRequest)
			return
		}

		email := &emailRaw
		ctx := r.Context()

		if h.db != nil {
			if err := h.db.UpdateFarmAccount(ctx, sess.AccountID, newUsername, email, color); err != nil {
				log.Printf("farm update account [%s]: %v", sess.Username, err)
				http.Error(w, "Username or email already taken", http.StatusConflict)
				return
			}
		} else {
			h.memAccts.updateAccount(sess.AccountID, newUsername, email, color)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"username": newUsername,
			"email":    emailRaw,
			"color":    color,
		})
	}
}

// ── WebSocket pumps ───────────────────────────────────────────────────────────

// farmWritePump reads from the player's channel and writes to the WebSocket.
func farmWritePump(conn *websocket.Conn, p *farmgame.Player) {
	defer conn.Close()
	for msg := range p.Messages() {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// farmReadPump reads WebSocket messages and submits actions to the room.
func farmReadPump(conn *websocket.Conn, p *farmgame.Player, room *farmgame.Room) {
	defer conn.Close()
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var raw struct {
			Action string `json:"action"`
			X      int    `json:"x"`
			Y      int    `json:"y"`
			Text   string `json:"text"`
			To     string `json:"to"`
		}
		if json.Unmarshal(msg, &raw) != nil {
			continue
		}
		switch raw.Action {
		case "move", "chat", "dm", "color", "username":
			room.Submit(farmgame.Input{
				PlayerID: p.ID,
				Action:   raw.Action,
				X:        raw.X,
				Y:        raw.Y,
				Text:     raw.Text,
				To:       raw.To,
			})
		}
	}
}

// parseEntryCoords reads optional ?ex=&ey= query params for room-transition spawn.
// Returns (-1, -1) when absent or invalid, signalling random spawn.
func parseEntryCoords(r *http.Request) (int, int) {
	ex, ey := -1, -1
	if v := r.URL.Query().Get("ex"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			ex = n
		}
	}
	if v := r.URL.Query().Get("ey"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			ey = n
		}
	}
	return ex, ey
}

// urlEncode replaces spaces with + for use in redirect query strings.
func urlEncode(s string) string {
	return strings.ReplaceAll(s, " ", "+")
}

// isValidHexColor reports whether s is a valid 6-digit CSS hex color (#RRGGBB).
func isValidHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
