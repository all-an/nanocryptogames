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
)

func main() {
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

	if addr := os.Getenv("DONATION_ADDRESS"); addr != "" {
		log.Printf("donation address: %s", addr)
	} else {
		log.Println("DONATION_ADDRESS not set — first-shot donations disabled")
	}

	// ── Templates ────────────────────────────────────────────────────────────
	tmpl := template.Must(template.ParseGlob("internal/templates/*.html"))

	// ── Hub + WS handler ─────────────────────────────────────────────────────
	hub := game.NewHub()
	wsHandler := handler.NewWSHandler(hub, database, masterSeed, rpcClient)

	// Register the first-shot donation callback on the hub.
	// It fires asynchronously the first time each player shoots, once per session.
	hub.SetOnFirstShot(wsHandler.FireDonation)

	// ── Routes ───────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	mux.Handle("GET /static/",
		http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	mux.Handle("GET /", handler.NewHomeHandler(tmpl))

	gamePage := handler.NewGamePageHandler(tmpl)
	mux.Handle("GET /game", gamePage)
	mux.Handle("GET /game/{roomID}", gamePage)

	mux.Handle("GET /api/rooms", handler.NewRoomsHandler(hub))

	mux.Handle("GET /ws/{roomID}", wsHandler)

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
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
