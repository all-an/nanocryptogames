// player.go defines per-player state held in memory during a game session.
package game

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
	GX, GY      int    // grid position (column, row)
	Health      int
	send        chan []byte // outbound messages to this player's WebSocket
}

// NewPlayer creates a Player with full health and a buffered send channel.
// Color, position, and Nano fields are assigned by the room or handler.
func NewPlayer(id, roomID string) *Player {
	return &Player{
		ID:     id,
		RoomID: roomID,
		Health: 100,
		send:   make(chan []byte, 64),
	}
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
