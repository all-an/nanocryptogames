// room.go manages the lifecycle of a single game room and its tick loop.
package game

import (
	"encoding/json"
	"sync"
	"time"
)

const tickRate = 50 * time.Millisecond // 20 TPS — used for state broadcast heartbeat

// spawnPoints are fixed grid positions spread across the arena.
var spawnPoints = [][2]int{
	{1, 1}, {23, 1}, {12, 8},
	{1, 15}, {23, 15}, {6, 5},
	{18, 5}, {12, 15},
}

// Input is a move command sent by a player targeting a specific grid cell.
type Input struct {
	PlayerID string
	GX, GY   int // target grid column and row
}

// playerState is the per-player snapshot included in each broadcast.
type playerState struct {
	ID     string `json:"id"`
	GX     int    `json:"gx"`
	GY     int    `json:"gy"`
	Health int    `json:"health"`
	Color  string `json:"color"`
}

// worldState is the full game snapshot sent to every client each tick.
type worldState struct {
	Type    string        `json:"type"`
	Players []playerState `json:"players"`
}

// Room represents one active game session with its own goroutine and tick loop.
type Room struct {
	ID          string
	players     map[string]*Player
	inputCh     chan Input
	done        chan struct{}
	mu          sync.RWMutex
	playerCount int // total players ever joined; used for colour and spawn assignment
}

// NewRoom creates a Room ready to accept players.
func NewRoom(id string) *Room {
	return &Room{
		ID:      id,
		players: make(map[string]*Player),
		inputCh: make(chan Input, 256),
		done:    make(chan struct{}),
	}
}

// Run starts the room's tick loop. Call this in a dedicated goroutine.
func (r *Room) Run() {
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Heartbeat broadcast keeps clients in sync even without movement.
			r.broadcastState()
		case input := <-r.inputCh:
			// Apply move and immediately push the updated state.
			r.applyInput(input)
			r.broadcastState()
		case <-r.done:
			return
		}
	}
}

// Join adds a player to the room, assigning their colour and spawn position.
func (r *Room) Join(p *Player) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p.Color = colorPalette[r.playerCount%len(colorPalette)]
	spawn := spawnPoints[r.playerCount%len(spawnPoints)]
	p.GX, p.GY = spawn[0], spawn[1]
	r.playerCount++

	r.players[p.ID] = p
}

// Leave removes a player from the room.
func (r *Room) Leave(p *Player) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.players, p.ID)
}

// Empty reports whether the room has no connected players.
func (r *Room) Empty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.players) == 0
}

// Close signals the tick loop to stop.
func (r *Room) Close() {
	close(r.done)
}

// Submit queues a player move for processing. Non-blocking: drops if buffer is full.
func (r *Room) Submit(input Input) {
	select {
	case r.inputCh <- input:
	default:
	}
}

// currentPlayerCount returns the number of players currently in the room.
func (r *Room) currentPlayerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.players)
}

// applyInput validates and applies a move. Ignores non-adjacent or out-of-bounds moves.
func (r *Room) applyInput(input Input) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.players[input.PlayerID]
	if !ok {
		return
	}

	// Server enforces adjacency — clients cannot teleport.
	if !isAdjacentMove(p.GX, p.GY, input.GX, input.GY) {
		return
	}

	p.GX, p.GY = clampToGrid(input.GX, input.GY)
}

// broadcastState serialises the current world snapshot and fans it out to all players.
func (r *Room) broadcastState() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state := worldState{Type: "state", Players: make([]playerState, 0, len(r.players))}
	for _, p := range r.players {
		state.Players = append(state.Players, playerState{
			ID:     p.ID,
			GX:     p.GX,
			GY:     p.GY,
			Health: p.Health,
			Color:  p.Color,
		})
	}

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	for _, p := range r.players {
		p.Send(data)
	}
}
