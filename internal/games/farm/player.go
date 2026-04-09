// player.go defines per-player state held in memory during an Farm session.
package farm

// colorPalette cycles through 8 distinct colors, assigned to players by join order.
var colorPalette = []string{
	"#E05252", "#4A90D9", "#50C878", "#F5A623",
	"#9B59B6", "#1ABC9C", "#E91E8C", "#F39C12",
}

// Player holds in-memory state for a single connected Farm player.
type Player struct {
	ID          string // random hex, unique per WebSocket connection
	AccountID   string // persistent account ID (UUID or in-memory ID)
	Username    string // display name
	NanoAddress string // game wallet nano_ address (derived from master seed)
	SeedIndex   int    // HD wallet index used to derive this player's key pair
	X, Y        int    // current grid position (column, row)
	Color       string // assigned display color from colorPalette
	RoomID      string // the room this player has joined
	send        chan []byte
}

// NewPlayer creates a player with a buffered outbound message channel.
// Provide a non-empty color to use the player's saved preference; pass "" to
// let the room assign one from the palette on join.
func NewPlayer(id, accountID, username, nanoAddress, roomID, color string, seedIndex int) *Player {
	return &Player{
		ID:          id,
		AccountID:   accountID,
		Username:    username,
		NanoAddress: nanoAddress,
		Color:       color,
		SeedIndex:   seedIndex,
		RoomID:      roomID,
		send:        make(chan []byte, 64),
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

// Close shuts down the send channel, causing the write pump to exit.
// Call only after the player has been removed from their room.
func (p *Player) Close() {
	close(p.send)
}
