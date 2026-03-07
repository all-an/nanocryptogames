// main.go is the entry point for the nano-multiplayer server.
// It wires HTTP routes, the WebSocket hub, static file serving, and the database.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"os"

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

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
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
		primaryURL = "https://nanoslo.0x.no"
	}
	fallbackURL := os.Getenv("NANO_RPC_FALLBACK_URL")
	if fallbackURL == "" {
		fallbackURL = "https://node.somenano.com"
	}
	rpcClient := nano.NewClient(nano.Config{
		PrimaryURL:  primaryURL,
		FallbackURL: fallbackURL,
	})

	// ── Templates ────────────────────────────────────────────────────────────
	tmpl := template.Must(template.ParseGlob("internal/templates/*.html"))

	// ── Hub + WS handler ─────────────────────────────────────────────────────
	hub := game.NewHub()
	wsHandler := handler.NewWSHandler(hub, database, masterSeed, rpcClient)

	// ── Routes ───────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	mux.Handle("GET /static/",
		http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	mux.Handle("GET /", handler.NewHomeHandler(tmpl))
	mux.Handle("GET /lobby", handler.NewLobbyHandler(tmpl))

	gamePage := handler.NewGamePageHandler(tmpl)
	mux.Handle("GET /game", gamePage)
	mux.Handle("GET /game/{roomID}", gamePage)

	mux.Handle("GET /api/rooms", handler.NewRoomsHandler(hub))

	mux.Handle("GET /ws/{roomID}", wsHandler)

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, closedMiddleware(database, tmpl, mux)); err != nil {
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
