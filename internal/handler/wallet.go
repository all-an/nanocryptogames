// wallet.go handles the POST /wallet/create endpoint.
// It generates a fresh Nano wallet and returns the seed and address as JSON.
// The seed is never stored server-side; the client is responsible for saving it.
package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/allanabrahao/nanomultiplayer/internal/wallet"
)

// WalletHandler serves POST /wallet/create.
type WalletHandler struct{}

// NewWalletHandler constructs a WalletHandler.
func NewWalletHandler() *WalletHandler {
	return &WalletHandler{}
}

func (h *WalletHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	gen, err := wallet.Generate()
	if err != nil {
		log.Printf("wallet create: %v", err)
		http.Error(w, "failed to generate wallet", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(gen); err != nil {
		log.Printf("wallet create: encode response: %v", err)
	}
}
