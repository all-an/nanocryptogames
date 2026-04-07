// Package wallet provides client-side Nano wallet generation.
// It is responsible only for creating a fresh random seed and
// deriving the corresponding address — no persistence, no RPC.
package wallet

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/allanabrahao/nanomultiplayer/internal/nano"
)

// Generated holds the one-time output of a new wallet.
// The caller must show SeedHex to the user and instruct them to save it;
// it is never stored server-side.
type Generated struct {
	SeedHex string `json:"seed"`
	Address string `json:"address"`
}

// Generate creates a cryptographically random 32-byte seed, derives the
// Nano address at index 0, and returns both. The seed is never stored.
func Generate() (Generated, error) {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return Generated{}, fmt.Errorf("wallet: read random seed: %w", err)
	}

	addr, err := nano.DeriveAddress(seed, 0)
	if err != nil {
		return Generated{}, fmt.Errorf("wallet: derive address: %w", err)
	}

	return Generated{
		SeedHex: hex.EncodeToString(seed),
		Address: addr,
	}, nil
}
