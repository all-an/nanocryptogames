// room.go manages the lifecycle of a single game room and its 20 TPS tick loop.
package game

import (
	"encoding/json"
	"sync"
	"time"
)

const tickRate = 50 * time.Millisecond // 20 TPS

// spawnPoints are fixed starting positions spread across the arena.
var spawnPoints = [][2]float64{
	{100, 100}, {900, 100}, {500, 350},
	{100, 600}, {900, 600}, {300, 200},
	{700, 500}, {500, 600},
}

// Input is a movement command sent by a player each time their key state changes.
type Input struct {
	PlayerID string
	DX, DY   float64 // direction vector, each axis in [-1, 1]
}

// playerState is the per-player snapshot included in each broadcast.
type playerState struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Health int     `json:"health"`
	Color  string  `json:"color"`
}

// worldState is the full game state sent to every client each tick.
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
			r.applyVelocities()
			r.broadcastState()
		case input := <-r.inputCh:
			r.applyInput(input)
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
	p.X, p.Y = spawn[0], spawn[1]
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

// Submit queues a player input for processing on the next tick.
// Non-blocking: drops the input if the channel buffer is full.
func (r *Room) Submit(input Input) {
	select {
	case r.inputCh <- input:
	default:
	}
}

// applyInput updates a player's stored velocity direction.
func (r *Room) applyInput(input Input) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.players[input.PlayerID]
	if !ok {
		return
	}
	p.Vx = input.DX
	p.Vy = input.DY
}

// applyVelocities moves every player by their current velocity and clamps to the arena.
func (r *Room) applyVelocities() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range r.players {
		p.X, p.Y = clampToArena(
			p.X+p.Vx*MoveSpeed,
			p.Y+p.Vy*MoveSpeed,
		)
	}
}

// broadcastState serialises the current world snapshot and fans it out to all players.
func (r *Room) broadcastState() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state := worldState{Type: "state", Players: make([]playerState, 0, len(r.players))}
	for _, p := range r.players {
		state.Players = append(state.Players, playerState{
			ID:     p.ID,
			X:      p.X,
			Y:      p.Y,
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
