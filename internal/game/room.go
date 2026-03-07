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

// Input is a command sent by a player: "move", "shoot", or "help".
type Input struct {
	PlayerID string
	Action   string // "move" (default), "shoot", or "help"
	GX, GY   int    // target grid cell for move
	TargetID string // target player ID for shoot/help
}

// shotEvent is broadcast to all players when a shot is fired.
type shotEvent struct {
	Type      string `json:"type"`
	ShooterID string `json:"shooterID"`
	TargetID  string `json:"targetID"`
}

// helpEvent is broadcast to all players when a player gives medical help.
type helpEvent struct {
	Type     string `json:"type"`
	HelperID string `json:"helperID"`
	TargetID string `json:"targetID"`
}

// playerState is the per-player snapshot included in each broadcast.
type playerState struct {
	ID     string `json:"id"`
	GX     int    `json:"gx"`
	GY     int    `json:"gy"`
	Health int    `json:"health"`
	Color  string `json:"color"`
	Team   string `json:"team"`
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
			// Apply the action and immediately push the updated state.
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

// Submit queues a player action for processing. Non-blocking: drops if buffer is full.
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

// applyInput dispatches to the correct handler based on the action type.
func (r *Room) applyInput(input Input) {
	switch input.Action {
	case "shoot":
		r.applyShoot(input)
	case "help":
		r.applyHelp(input)
	default:
		r.applyMove(input)
	}
}

// applyMove validates and applies a movement command.
// Only healthy players (health == 100) may move.
func (r *Room) applyMove(input Input) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.players[input.PlayerID]
	if !ok || p.Health < 100 {
		return // incapacitated and dead players cannot move
	}

	// Server enforces the movement radius — clients cannot teleport beyond it.
	if !isValidMove(p.GX, p.GY, input.GX, input.GY) {
		return
	}

	p.GX, p.GY = clampToGrid(input.GX, input.GY)
}

// applyShoot handles a shoot action: validates, applies damage, broadcasts the shot event.
// A healthy player at health 100 can shoot; the shot reduces target health by 50.
// First hit (100→50) incapacitates; second hit (50→0) kills.
// Dead players are removed from the room after a 2-second grace period.
func (r *Room) applyShoot(input Input) {
	r.mu.Lock()

	shooter, ok := r.players[input.PlayerID]
	if !ok || shooter.Health < 100 {
		r.mu.Unlock()
		return // only healthy players can shoot
	}

	target, ok := r.players[input.TargetID]
	if !ok || target.Health == 0 {
		r.mu.Unlock()
		return // target must exist and be alive
	}

	// Cannot shoot a teammate.
	if shooter.Team == target.Team {
		r.mu.Unlock()
		return
	}

	// Validate that the target is within shooting range (same radius as movement).
	if !isValidMove(shooter.GX, shooter.GY, target.GX, target.GY) {
		r.mu.Unlock()
		return
	}

	target.Health -= 50
	isDead := target.Health == 0
	deadTarget := target

	// Broadcast the shot event so clients can animate the bullet.
	evt, _ := json.Marshal(shotEvent{
		Type:      "shot",
		ShooterID: input.PlayerID,
		TargetID:  input.TargetID,
	})
	for _, p := range r.players {
		p.Send(evt)
	}

	r.mu.Unlock()

	// After a 2-second grace period, remove the dead player from the room.
	if isDead {
		go func() {
			time.Sleep(2 * time.Second)
			r.mu.Lock()
			delete(r.players, deadTarget.ID)
			r.mu.Unlock()
		}()
	}
}

// applyHelp handles a medical help action.
// A healthy player adjacent (Chebyshev distance ≤ 1) to an incapacitated player
// can give medical help, restoring the target to full health.
// The heal_reward Nano credit is handled by Phase 9 (shot economy).
func (r *Room) applyHelp(input Input) {
	r.mu.Lock()
	defer r.mu.Unlock()

	helper, ok := r.players[input.PlayerID]
	if !ok || helper.Health < 100 {
		return // only healthy players can give medical help
	}

	target, ok := r.players[input.TargetID]
	if !ok || target.Health != 50 {
		return // can only help incapacitated players (health == 50)
	}

	// Can only help a teammate.
	if helper.Team != target.Team {
		return
	}

	// Helper must be standing next to the target (Chebyshev distance ≤ 1).
	dx := helper.GX - target.GX
	dy := helper.GY - target.GY
	if dx < -1 || dx > 1 || dy < -1 || dy > 1 {
		return
	}

	target.Health = 100

	// Notify all players so they can show a visual cue.
	evt, _ := json.Marshal(helpEvent{
		Type:     "helped",
		HelperID: input.PlayerID,
		TargetID: input.TargetID,
	})
	for _, p := range r.players {
		p.Send(evt)
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
			GX:     p.GX,
			GY:     p.GY,
			Health: p.Health,
			Color:  p.Color,
			Team:   p.Team,
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
