// main.go is the entry point for the nano-multiplayer server.
// It wires HTTP routes, the WebSocket hub, static file serving, and the database.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/allanabrahao/nanomultiplayer/internal/db"
	"github.com/allanabrahao/nanomultiplayer/internal/game"
	"github.com/allanabrahao/nanomultiplayer/internal/handler"
	"github.com/allanabrahao/nanomultiplayer/internal/nano"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/blake2b"
)

func main() {
	// Load .env if present (dev convenience). Real env vars always take precedence.
	if err := godotenv.Load(); err == nil {
		log.Println("loaded .env")
	}

	// ── File logging ──────────────────────────────────────────────────────────
	// File logging is opt-in via LOG_FILE. When not set (e.g. on Render where
	// the filesystem is ephemeral), all output goes to stdout only, which is
	// captured by the platform's log aggregator.
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	writers := []io.Writer{os.Stdout}
	if logPath := os.Getenv("LOG_FILE"); logPath != "" {
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("warning: could not open log file %s: %v — logging to stdout only", logPath, err)
		} else {
			defer logFile.Close()
			writers = append(writers, logFile)
			log.Printf("logging to file: %s", logPath)
		}
	}
	log.SetOutput(io.MultiWriter(writers...))

	addr := os.Getenv("ADDR")
	if addr == "" {
		if port := os.Getenv("PORT"); port != "" {
			addr = ":" + port
		} else {
			addr = ":8080"
		}
	}

	ctx := context.Background()

	// ── Database (optional for local dev) ────────────────────────────────────
	var database *db.DB
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		var err error
		database, err = db.Connect(ctx, dbURL)
		if err != nil {
			log.Fatalf("db connect: %v", err)
		}
		defer database.Close()

		if err := database.Migrate(ctx); err != nil {
			log.Fatalf("db migrate: %v", err)
		}
		log.Println("database connected and migrated")
	} else {
		log.Println("DATABASE_URL not set — running without persistence")
	}

	// ── Nano master seed ─────────────────────────────────────────────────────
	masterSeed := loadMasterSeed()

	// Store a fingerprint of the master seed in the DB so we can later verify
	// which seed generated the wallets — without ever persisting the seed itself.
	if database != nil {
		h := blake2b.Sum256(masterSeed)
		if err := database.StoreMasterSeedFingerprint(ctx, hex.EncodeToString(h[:])); err != nil {
			log.Printf("warning: could not store seed fingerprint: %v", err)
		}
	}

	// ── Nano RPC client ───────────────────────────────────────────────────────
	primaryURL := os.Getenv("NANO_RPC_PRIMARY_URL")
	if primaryURL == "" {
		primaryURL = "https://rpc.nano.to"
	}
	fallbackURL := os.Getenv("NANO_RPC_FALLBACK_URL")
	if fallbackURL == "" {
		fallbackURL = "https://rpc.nano.to"
	}
	nanoAPIKey := os.Getenv("NANO_RPC_API_KEY")
	if nanoAPIKey != "" {
		log.Printf("nano RPC: API key configured (%.8s…)", nanoAPIKey)
	} else {
		log.Printf("nano RPC: no API key — work_generate may be rate-limited (set NANO_RPC_API_KEY)")
	}
	rpcClient := nano.NewClient(nano.Config{
		PrimaryURL:  primaryURL,
		FallbackURL: fallbackURL,
		APIKey:      nanoAPIKey,
	})

	// ── Templates ────────────────────────────────────────────────────────────
	tmpl := template.Must(template.ParseGlob("internal/templates/*.html"))
	tmpl = template.Must(tmpl.ParseGlob("internal/templates/faucet_game/*.html"))
	tmpl = template.Must(tmpl.ParseGlob("internal/templates/paid_game/*.html"))

	// ── Hub + WS handler (paid game) ─────────────────────────────────────────
	hub := game.NewHub()
	wsHandler := handler.NewWSHandler(hub, database, masterSeed, rpcClient)

	// ── Faucet hub + wallet ───────────────────────────────────────────────────
	faucetHub := game.NewFaucetHub()
	if database != nil && database.FaucetDisableSameIPCheck(ctx) {
		faucetHub.DisableSameIPCheck = true
		log.Println("faucet: same-IP check disabled (faucet_disable_same_ip_check=true)")
	}
	var faucetWallet *nano.Wallet
	var faucetAddr string
	if faucetSeedHex := os.Getenv("FAUCET_SEED"); faucetSeedHex != "" {
		faucetSeed, err := hex.DecodeString(faucetSeedHex)
		if err != nil || len(faucetSeed) != 32 {
			log.Fatalf("FAUCET_SEED must be 64 hex characters (32 bytes)")
		}
		w, err := nano.DeriveWallet(faucetSeed, 0)
		if err != nil {
			log.Fatalf("faucet wallet derivation: %v", err)
		}
		faucetWallet = w
		faucetAddr = w.Address
		log.Printf("faucet wallet: %s", faucetAddr)
	} else {
		log.Println("FAUCET_SEED not set — faucet rewards disabled")
	}
	faucetSender := handler.NewFaucetSender(rpcClient, faucetWallet)
	faucetSender.Init() // pre-warm PoW cache in background at startup
	faucetWSHandler := handler.NewFaucetWSHandler(faucetHub, database, faucetSender)

	// ── Routes ───────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	mux.Handle("GET /static/",
		http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	mux.Handle("GET /", handler.NewLandingHandler(tmpl, database, faucetAddr))
	mux.Handle("GET /welcome", handler.NewWelcomeHandler(tmpl))
	mux.Handle("GET /lobby", handler.NewLobbyHandler(tmpl))

	gamePage := handler.NewGamePageHandler(tmpl)
	mux.Handle("GET /game", gamePage)
	mux.Handle("GET /game/{roomID}", gamePage)

	mux.Handle("GET /api/rooms", handler.NewRoomsHandler(hub))

	mux.Handle("GET /ws/{roomID}", wsHandler)

	// ── Faucet routes ─────────────────────────────────────────────────────────
	faucetGamePage := handler.NewFaucetGamePageHandler(tmpl, faucetAddr)
	mux.Handle("GET /faucet", handler.NewFaucetWelcomeHandler(tmpl, faucetAddr))
	mux.Handle("GET /faucet/lobby", handler.NewFaucetLobbyHandler(tmpl))
	mux.Handle("GET /faucet/game", faucetGamePage)
	mux.Handle("GET /faucet/game/{roomID}", faucetGamePage)
	mux.Handle("GET /faucet/api/rooms", handler.NewRoomsHandler(faucetHub))
	mux.Handle("GET /faucet/ws/{roomID}", faucetWSHandler)
	mux.Handle("GET /faucet/bots", handler.NewFaucetBotsPageHandler(tmpl, faucetAddr))
	mux.Handle("POST /faucet/bots/reward", handler.NewFaucetBotsRewardHandler(database, faucetSender))

	rpcTestHandler := handler.NewRPCTestHandler(tmpl, database, rpcClient, masterSeed)
	mux.Handle("GET /rpc-test", rpcTestHandler)
	mux.Handle("GET /rpc-test/balance", rpcTestHandler)
	mux.Handle("POST /rpc-test/receive", rpcTestHandler)
	mux.Handle("POST /rpc-test/withdraw", rpcTestHandler)

	log.Printf("listening on %s", addr)
	chain := closedMiddleware(database, tmpl, mux)
	chain = accessLogMiddleware(database, chain)
	if err := http.ListenAndServe(addr, chain); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// closedMiddleware intercepts every request and serves the "game closed" page
// when the game_closed setting is "true" in the database.
// Static assets are always served so the closed page renders correctly.
func closedMiddleware(database *db.DB, tmpl *template.Template, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if database == nil {
			next.ServeHTTP(w, r)
			return
		}
		// Always allow static assets through.
		if len(r.URL.Path) >= 8 && r.URL.Path[:8] == "/static/" {
			next.ServeHTTP(w, r)
			return
		}
		val, err := database.Setting(r.Context(), "game_closed")
		if err == nil && val == "true" {
			msg, _ := database.Setting(r.Context(), "game_closed_message")
			if msg == "" {
				msg = "The game is temporarily closed. Check back soon!"
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			tmpl.ExecuteTemplate(w, "closed.html", map[string]string{"Message": msg})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// accessLogMiddleware records every non-static request in access_log and
// increments the access_daily counter. Country lookup runs in a goroutine so
// it never slows down the response.
func accessLogMiddleware(database *db.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if database != nil && r.URL.Path == "/" {
			ip := realIP(r)
			id, err := database.LogAccess(r.Context(), ip, r.URL.Path)
			if err != nil {
				log.Printf("access_log insert: %v", err)
			} else if id != 0 {
				go func() {
					country := geoCountry(ip)
					if country == "" {
						return
					}
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := database.SetAccessCountry(ctx, id, country); err != nil {
						log.Printf("access_log country update: %v", err)
					}
				}()
			}
		}
		next.ServeHTTP(w, r)
	})
}

// realIP extracts the client IP from common proxy headers, falling back to
// RemoteAddr. Takes the first address in X-Forwarded-For when multiple are present.
func realIP(r *http.Request) string {
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

// geoCountry calls the ip-api.com free endpoint to resolve an IP to a country name.
// Returns an empty string on any failure so the caller can skip the update.
func geoCountry(ip string) string {
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return "local"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/" + ip + "?fields=country") //nolint:noctx
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var result struct {
		Country string `json:"country"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}
	return result.Country
}

// loadMasterSeed reads NANO_MASTER_SEED from the environment (hex-encoded 32 bytes).
// If unset, a random seed is generated and logged — wallets are ephemeral in this mode.
func loadMasterSeed() []byte {
	raw := os.Getenv("NANO_MASTER_SEED")
	if raw != "" {
		seed, err := hex.DecodeString(raw)
		if err != nil || len(seed) != 32 {
			log.Fatalf("NANO_MASTER_SEED must be 64 hex characters (32 bytes)")
		}
		return seed
	}

	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		log.Fatalf("generate dev seed: %v", err)
	}
	log.Printf("WARNING: NANO_MASTER_SEED not set — using ephemeral dev seed %s", hex.EncodeToString(seed))
	return seed
}
