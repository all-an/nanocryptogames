// room.go manages the lifecycle of a single game room and its tick loop.
package game

import (
	"encoding/json"
	"math/big"
	"sync"
	"time"
)

const tickRate = 50 * time.Millisecond // 20 TPS — used for state broadcast heartbeat

// defaultShotCostRaw is 0.0001 XNO expressed in raw Nano units (10^26 raw).
// This value is used when the DB is not connected; it can be overridden via SetShotCost.
var defaultShotCostRaw, _ = new(big.Int).SetString("100000000000000000000000000", 10)

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

// roundOverEvent is broadcast when a player is killed, before the restart.
type roundOverEvent struct {
	Type     string `json:"type"`
	KillerID string `json:"killerID"`
	Prize    string `json:"prize"` // human-readable XNO amount
}

// balanceEvent is sent privately to a single player after their balance changes.
type balanceEvent struct {
	Type string `json:"type"`
	XNO  string `json:"xno"` // human-readable balance, e.g. "0.000300"
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
	playerCount int          // total players ever joined; used for colour and spawn assignment
	shotCostRaw *big.Int     // cost per shot in raw Nano units
	onFirstShot func(*Player) // called in a goroutine on each player's first-ever shot
}

// NewRoom creates a Room ready to accept players.
func NewRoom(id string) *Room {
	return &Room{
		ID:          id,
		players:     make(map[string]*Player),
		inputCh:     make(chan Input, 256),
		done:        make(chan struct{}),
		shotCostRaw: new(big.Int).Set(defaultShotCostRaw),
	}
}

// SetShotCost overrides the default shot cost. Call this before players join.
func (r *Room) SetShotCost(raw *big.Int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shotCostRaw = new(big.Int).Set(raw)
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
	p.SpawnGX, p.SpawnGY = spawn[0], spawn[1]
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

// teamCounts returns the number of red and blue players currently in the room.
func (r *Room) teamCounts() (red, blue int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.players {
		if p.Team == "red" {
			red++
		} else {
			blue++
		}
	}
	return
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

// applyShoot handles a shoot action: validates, deducts shot cost, applies damage,
// and on a kill awards the prize (2 × shot_cost refund + 1 × shot_cost bonus)
// then schedules a round restart after 3 seconds.
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

	// Fire the first-shot donation callback exactly once per player session.
	// This runs in a goroutine so the game loop is never blocked.
	if !shooter.FirstShotFired {
		shooter.FirstShotFired = true
		if r.onFirstShot != nil {
			go r.onFirstShot(shooter)
		}
	}

	// Deduct shot cost from shooter's balance.
	shooter.BalanceRaw.Sub(shooter.BalanceRaw, r.shotCostRaw)
	shooterBalanceXNO := shooter.BalanceXNO()

	target.Health -= 50
	isKill := target.Health == 0

	// Broadcast the shot event so clients can animate the bullet.
	evt, _ := json.Marshal(shotEvent{
		Type:      "shot",
		ShooterID: input.PlayerID,
		TargetID:  input.TargetID,
	})
	for _, p := range r.players {
		p.Send(evt)
	}

	// On a kill: award the prize (3 × shot_cost = 2 refund + 1 bonus) and
	// broadcast the round-over event. The round restarts after 3 seconds.
	var prizeXNO string
	if isKill {
		prize := new(big.Int).Mul(r.shotCostRaw, big.NewInt(3))
		shooter.BalanceRaw.Add(shooter.BalanceRaw, prize)
		shooterBalanceXNO = shooter.BalanceXNO() // refresh after prize
		prizeXNO = FormatXNO(r.shotCostRaw)       // prize = 1 × shot_cost net gain

		roundOver, _ := json.Marshal(roundOverEvent{
			Type:     "roundover",
			KillerID: input.PlayerID,
			Prize:    prizeXNO,
		})
		for _, p := range r.players {
			p.Send(roundOver)
		}
	}

	r.mu.Unlock()

	// Send updated balance privately to the shooter.
	balMsg, _ := json.Marshal(balanceEvent{Type: "balance", XNO: shooterBalanceXNO})
	shooter.Send(balMsg)

	// After a kill, restart the round after a 3-second grace period.
	if isKill {
		go func() {
			time.Sleep(3 * time.Second)
			r.restartRound()
		}()
	}
}

// applyHelp handles a medical help action.
// A healthy player adjacent (Chebyshev distance ≤ 1) to an incapacitated teammate
// can give medical help, restoring the target to full health.
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

// restartRound resets every player to full health at their spawn position and
// broadcasts a "newround" event followed by the updated world state.
func (r *Room) restartRound() {
	r.mu.Lock()

	newRound, _ := json.Marshal(map[string]string{"type": "newround"})
	for _, p := range r.players {
		p.Health = 100
		p.GX, p.GY = p.SpawnGX, p.SpawnGY
		p.Send(newRound)
	}

	r.mu.Unlock()

	r.broadcastState()
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
