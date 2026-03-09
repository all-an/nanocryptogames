// rpctest.go serves the /rpc-test diagnostic page.
// It replicates the exact add-funds / withdraw cycle used in the game:
//   1. Shows a dedicated test wallet address the user sends Nano to.
//   2. POST /rpc-test/receive  — receives pending blocks (identical to checkDeposits).
//   3. POST /rpc-test/withdraw — sends the full balance to a caller-supplied address
//      (identical to processWithdraw).
//
// The page is only accessible when the database setting rpc_test = 'true'.
// Test wallet is derived at seed index math.MaxUint32-1 so it never collides
// with real player wallets allocated by the DB sequence.
package handler

import (
	"encoding/json"
	"html/template"
	"log"
	"math"
	"net/http"

	"github.com/allanabrahao/nanomultiplayer/internal/db"
	"github.com/allanabrahao/nanomultiplayer/internal/nano"
)

// testWalletIndex is a reserved HD seed index used exclusively by the test page.
// It is far above what the DB sequence will ever reach in practice.
const testWalletIndex = math.MaxUint32 - 1

// RPCTestHandler serves the /rpc-test page and its JSON sub-endpoints.
type RPCTestHandler struct {
	tmpl       *template.Template
	db         *db.DB
	rpc        *nano.Client
	masterSeed []byte
}

// NewRPCTestHandler wires up the handler.
func NewRPCTestHandler(tmpl *template.Template, database *db.DB, rpc *nano.Client, masterSeed []byte) *RPCTestHandler {
	return &RPCTestHandler{tmpl: tmpl, db: database, rpc: rpc, masterSeed: masterSeed}
}

func (h *RPCTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(r) {
		http.Error(w, "RPC test page is disabled. Set rpc_test = 'true' in the settings table.", http.StatusForbidden)
		return
	}

	// Sub-endpoints for AJAX calls from the page.
	switch r.URL.Path {
	case "/rpc-test/balance":
		h.handleBalance(w, r)
		return
	case "/rpc-test/receive":
		if r.Method == http.MethodPost {
			h.handleReceive(w, r)
			return
		}
	case "/rpc-test/withdraw":
		if r.Method == http.MethodPost {
			h.handleWithdraw(w, r)
			return
		}
	}

	// Render the page.
	wallet, err := nano.DeriveWallet(h.masterSeed, testWalletIndex)
	if err != nil {
		http.Error(w, "wallet derivation error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.tmpl.ExecuteTemplate(w, "rpctest.html", map[string]string{
		"Address": wallet.Address,
	})
}

// handleBalance returns the current on-chain balance as JSON.
func (h *RPCTestHandler) handleBalance(w http.ResponseWriter, r *http.Request) {
	wallet, err := nano.DeriveWallet(h.masterSeed, testWalletIndex)
	if err != nil {
		jsonErr(w, "wallet derivation error: "+err.Error())
		return
	}

	info, err := h.rpc.GetAccountInfo(r.Context(), wallet.Address)
	if err != nil {
		// Unopened account — balance is genuinely zero.
		jsonOK(w, map[string]string{"balance": "0", "xno": "0.000000"})
		return
	}
	jsonOK(w, map[string]string{"balance": info.Balance, "xno": formatXNO(info.Balance)})
}

// handleReceive receives all pending blocks into the test wallet, mirroring checkDeposits.
func (h *RPCTestHandler) handleReceive(w http.ResponseWriter, r *http.Request) {
	wallet, err := nano.DeriveWallet(h.masterSeed, testWalletIndex)
	if err != nil {
		jsonErr(w, "wallet derivation error: "+err.Error())
		return
	}

	hashes, err := h.rpc.Receivable(r.Context(), wallet.Address)
	if err != nil {
		jsonErr(w, "receivable query failed: "+err.Error())
		return
	}
	if len(hashes) == 0 {
		jsonOK(w, map[string]any{"received": 0, "balance": "0", "xno": "0.000000", "message": "No pending blocks found."})
		return
	}

	log.Printf("rpc-test: receiving %d pending block(s) for %s", len(hashes), wallet.Address)
	if err := nano.ReceivePending(r.Context(), h.rpc, wallet); err != nil {
		jsonErr(w, "receive failed: "+err.Error())
		return
	}

	info, err := h.rpc.GetAccountInfo(r.Context(), wallet.Address)
	if err != nil {
		jsonErr(w, "received OK but could not fetch new balance: "+err.Error())
		return
	}
	xno := formatXNO(info.Balance)
	log.Printf("rpc-test: received %d block(s), new balance %s XNO (%s raw)", len(hashes), xno, info.Balance)
	jsonOK(w, map[string]any{
		"received": len(hashes),
		"balance":  info.Balance,
		"xno":      xno,
	})
}

// handleWithdraw sends the full test wallet balance to the caller-supplied address,
// mirroring processWithdraw exactly.
func (h *RPCTestHandler) handleWithdraw(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.To == "" {
		jsonErr(w, "missing or invalid 'to' address")
		return
	}

	wallet, err := nano.DeriveWallet(h.masterSeed, testWalletIndex)
	if err != nil {
		jsonErr(w, "wallet derivation error: "+err.Error())
		return
	}

	info, err := h.rpc.GetAccountInfo(r.Context(), wallet.Address)
	if err != nil {
		jsonErr(w, "test wallet has no balance — send Nano to the address first")
		return
	}
	if info.Balance == "0" || info.Balance == "" {
		jsonErr(w, "balance is zero — nothing to withdraw")
		return
	}

	log.Printf("rpc-test: withdraw %s raw (%s XNO) → %s", info.Balance, formatXNO(info.Balance), body.To)
	hash, err := nano.Send(r.Context(), h.rpc, wallet, body.To, info.Balance)
	if err != nil {
		log.Printf("rpc-test: withdraw failed: %v", err)
		jsonErr(w, "send failed: "+err.Error())
		return
	}

	xno := formatXNO(info.Balance)
	log.Printf("rpc-test: withdraw OK — %s XNO → %s, block %s", xno, body.To, hash[:8])
	jsonOK(w, map[string]string{
		"xno":       xno,
		"toAddress": body.To,
		"blockHash": hash,
	})
}

// enabled returns true when rpc_test = 'true' in the settings table.
func (h *RPCTestHandler) enabled(r *http.Request) bool {
	if h.db == nil {
		return false
	}
	val, err := h.db.Setting(r.Context(), "rpc_test")
	return err == nil && val == "true"
}

// jsonOK writes a 200 JSON response.
func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// jsonErr writes a 400 JSON error response.
func jsonErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// formatXNO converts a raw balance string to a human-readable XNO string.
// 1 XNO = 10^30 raw.
func formatXNO(raw string) string {
	// Reuse the game package helper via a local wrapper.
	// Import kept minimal — replicate the 6-decimal formatting inline.
	if raw == "" || raw == "0" {
		return "0.000000"
	}
	// Pad to at least 31 chars so we can split at position len-30.
	for len(raw) < 31 {
		raw = "0" + raw
	}
	intPart := raw[:len(raw)-30]
	fracPart := raw[len(raw)-30:]
	if intPart == "" {
		intPart = "0"
	}
	if len(fracPart) > 6 {
		fracPart = fracPart[:6]
	}
	return intPart + "." + fracPart
}
