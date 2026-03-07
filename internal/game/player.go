// player.go defines per-player state held in memory during a game session.
package game

import (
	"fmt"
	"math/big"
)

// colorPalette is the fixed set of player colours, assigned by join order.
var colorPalette = []string{
	"#4A90D9", // Nano blue
	"#E05252", // Crimson
	"#52C07A", // Emerald
	"#F5A623", // Amber
	"#9B59B6", // Violet
	"#1ABC9C", // Teal
	"#E67E22", // Orange
	"#E91E8C", // Rose
}

// Player holds the in-memory state for a single connected player.
type Player struct {
	ID          string // short hex ID used over WebSocket
	DBID        string // UUID from the players table (empty when DB is not configured)
	SessionID   string // UUID from game_sessions (empty when DB is not configured)
	NanoAddress string // derived nano_ address for this session wallet
	RoomID      string
	Color       string
	Team        string // "red" or "blue" — chosen in the lobby
	SeedIndex      uint32   // HD wallet index used to derive this player's key pair
	GX, GY         int      // current grid position (column, row)
	SpawnGX        int      // spawn column assigned on join — used for round restarts
	SpawnGY        int      // spawn row assigned on join — used for round restarts
	Health         int
	BalanceRaw *big.Int   // session balance in raw Nano units (tracks credits/debits)
	send       chan []byte // outbound messages to this player's WebSocket
}

// NewPlayer creates a Player with full health and a buffered send channel.
// Color, position, and Nano fields are assigned by the room or handler.
func NewPlayer(id, roomID string) *Player {
	return &Player{
		ID:         id,
		RoomID:     roomID,
		Health:     100,
		BalanceRaw: new(big.Int),
		send:       make(chan []byte, 64),
	}
}

// BalanceXNO returns the player's balance formatted as a human-readable XNO
// string with 6 decimal places, e.g. "0.000300". Negative balances are shown
// with a leading minus sign.
func (p *Player) BalanceXNO() string {
	return FormatXNO(p.BalanceRaw)
}

// FormatXNO converts a raw Nano amount to a human-readable XNO string (6 dp).
// 1 XNO = 10^30 raw.
func FormatXNO(raw *big.Int) string {
	if raw == nil || raw.Sign() == 0 {
		return "0.000000"
	}
	neg := raw.Sign() < 0
	abs := new(big.Int).Abs(raw)

	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)
	whole := new(big.Int).Div(abs, divisor)
	rem := new(big.Int).Mod(abs, divisor)

	// Scale remainder to 6 decimal places: rem * 10^6 / 10^30 = rem / 10^24.
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)
	frac := new(big.Int).Div(rem, scale)

	result := fmt.Sprintf("%d.%06d", whole, frac)
	if neg {
		return "-" + result
	}
	return result
}

// Send queues a message for delivery over the WebSocket.
// Drops the message silently if the buffer is full to avoid blocking the game loop.
func (p *Player) Send(msg []byte) {
	select {
	case p.send <- msg:
	default:
	}
}

// Messages returns a receive-only channel of outbound messages.
// The WebSocket write pump reads from this channel.
func (p *Player) Messages() <-chan []byte {
	return p.send
}

// Close shuts down the player's send channel, causing the write pump to exit.
// Call this only after the player has been removed from their room.
func (p *Player) Close() {
	close(p.send)
}

// IsAlive reports whether the player still has health remaining.
func (p *Player) IsAlive() bool {
	return p.Health > 0
}
