// wallet_api.go serves the wallet page and its JSON API endpoints.
//
//   GET  /wallet          — serve the wallet SPA shell
//   POST /wallet/import   — validate seed, return derived address
//   POST /wallet/balance  — fetch confirmed + pending balance for an address
//   POST /wallet/send     — sign and broadcast a send block
//   POST /wallet/receive  — receive all pending blocks
package handler

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math/big"
	"net/http"
	"strings"

	"github.com/allanabrahao/nanomultiplayer/internal/nano"
)

// rawPerXNO is the number of raw units in one XNO (10^30).
var rawPerXNO = new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)

// WalletAPIHandler serves all /wallet/* endpoints.
type WalletAPIHandler struct {
	tmpl *template.Template
	rpc  *nano.Client
}

// NewWalletAPIHandler creates the handler.
func NewWalletAPIHandler(tmpl *template.Template, rpc *nano.Client) *WalletAPIHandler {
	return &WalletAPIHandler{tmpl: tmpl, rpc: rpc}
}

// Page serves GET /wallet — the client-side wallet SPA shell.
func (h *WalletAPIHandler) Page() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h.tmpl.ExecuteTemplate(w, "wallet.html", nil); err != nil {
			log.Printf("wallet page: %v", err)
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	})
}

// Import handles POST /wallet/import — validates a seed and returns address at index 0.
func (h *WalletAPIHandler) Import() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Seed string `json:"seed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request", http.StatusBadRequest)
			return
		}
		seedBytes, err := parseSeed(req.Seed)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		addr, err := nano.DeriveAddress(seedBytes, 0)
		if err != nil {
			log.Printf("wallet import: %v", err)
			jsonError(w, "could not derive address", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"address": addr})
	})
}

// Balance handles POST /wallet/balance — returns XNO balance for an address.
func (h *WalletAPIHandler) Balance() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Address string `json:"address"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request", http.StatusBadRequest)
			return
		}
		bal, err := h.rpc.GetBalance(r.Context(), req.Address)
		if err != nil {
			log.Printf("wallet balance: %v", err)
			jsonError(w, "could not fetch balance", http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{
			"balance_xno": rawToXNO(bal.Balance),
			"pending_xno": rawToXNO(bal.Pending),
		})
	})
}

// Send handles POST /wallet/send — signs and broadcasts a send block.
func (h *WalletAPIHandler) Send() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Seed      string `json:"seed"`
			To        string `json:"to"`
			AmountXNO string `json:"amount_xno"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request", http.StatusBadRequest)
			return
		}
		seedBytes, err := parseSeed(req.Seed)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		amountRaw, err := xnoToRaw(req.AmountXNO)
		if err != nil {
			jsonError(w, "invalid amount: "+err.Error(), http.StatusBadRequest)
			return
		}
		wallet, err := nano.DeriveWallet(seedBytes, 0)
		if err != nil {
			jsonError(w, "could not derive wallet", http.StatusInternalServerError)
			return
		}
		hash, err := nano.Send(r.Context(), h.rpc, wallet, req.To, amountRaw)
		if err != nil {
			log.Printf("wallet send: %v", err)
			jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"hash": hash})
	})
}

// Receive handles POST /wallet/receive — receives all pending blocks.
func (h *WalletAPIHandler) Receive() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Seed string `json:"seed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request", http.StatusBadRequest)
			return
		}
		seedBytes, err := parseSeed(req.Seed)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		wallet, err := nano.DeriveWallet(seedBytes, 0)
		if err != nil {
			jsonError(w, "could not derive wallet", http.StatusInternalServerError)
			return
		}
		// Count pending before receiving to report how many were processed.
		hashes, err := h.rpc.Receivable(r.Context(), wallet.Address)
		if err != nil {
			log.Printf("wallet receive: receivable: %v", err)
			jsonError(w, "could not check pending", http.StatusBadGateway)
			return
		}
		count := len(hashes)
		if count > 0 {
			if err := nano.ReceivePending(r.Context(), h.rpc, wallet); err != nil {
				log.Printf("wallet receive: %v", err)
				jsonError(w, err.Error(), http.StatusBadGateway)
				return
			}
		}
		writeJSON(w, map[string]int{"received": count})
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseSeed(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if len(s) != 64 {
		return nil, fmt.Errorf("seed must be 64 hex characters, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("seed must be valid hex")
	}
	return b, nil
}

// rawToXNO converts a raw string to an XNO string with up to 6 decimal places.
func rawToXNO(raw string) string {
	r := new(big.Int)
	r.SetString(raw, 10)
	if r == nil || r.Sign() == 0 {
		return "0"
	}
	whole := new(big.Int).Div(r, rawPerXNO)
	remainder := new(big.Int).Mod(r, rawPerXNO)

	if remainder.Sign() == 0 {
		return whole.String()
	}

	// Keep 6 significant decimal digits.
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)
	frac := new(big.Int).Div(remainder, scale)
	return fmt.Sprintf("%s.%06d", whole.String(), frac.Int64())
}

// xnoToRaw converts an XNO string (e.g. "0.001") to raw units string.
func xnoToRaw(xno string) (string, error) {
	xno = strings.TrimSpace(xno)
	parts := strings.SplitN(xno, ".", 2)
	whole := new(big.Int)
	if _, ok := whole.SetString(parts[0], 10); !ok {
		return "", fmt.Errorf("invalid number")
	}
	result := new(big.Int).Mul(whole, rawPerXNO)

	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) > 30 {
			frac = frac[:30]
		}
		// Pad to 30 decimal places.
		padded := frac + strings.Repeat("0", 30-len(frac))
		fracInt := new(big.Int)
		if _, ok := fracInt.SetString(padded, 10); !ok {
			return "", fmt.Errorf("invalid decimal")
		}
		result.Add(result, fracInt)
	}
	if result.Sign() <= 0 {
		return "", fmt.Errorf("amount must be greater than zero")
	}
	return result.String(), nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
