// room.go manages the lifecycle of a single game room and its tick loop.
package games

import (
	"encoding/json"
	"log"
	"math/big"
	"math/rand"
	"sync"
	"time"
)

const healItemCount   = 4               // number of heal packs on the map at once
const healItemRespawn = 15 * time.Second // delay before a consumed pack reappears

const tickRate = 50 * time.Millisecond // 20 TPS — used for state broadcast heartbeat

// defaultShotCostRaw is 0.0001 XNO expressed in raw Nano units (10^26 raw).
// This value is used when the DB is not connected; it can be overridden via SetShotCost.
var defaultShotCostRaw, _ = new(big.Int).SetString("100000000000000000000000000", 10)

// faucetRewardXNO is the human-readable faucet reward amount shown in round-over messages.
const faucetRewardXNO = "0.00001"

// redSpawnPoints are fixed positions on the left side of the arena (GX ≤ 3).
var redSpawnPoints = [][2]int{
	{1, 1}, {1, 8}, {1, 15}, {2, 4}, {2, 12},
}

// blueSpawnPoints are fixed positions on the right side of the arena (GX ≥ 21).
var blueSpawnPoints = [][2]int{
	{23, 1}, {23, 8}, {23, 15}, {22, 4}, {22, 12},
}

// Input is a command sent by a player: "move", "shoot", "help", or "reload".
type Input struct {
	PlayerID string
	Action   string // "move" (default), "shoot", "help", or "reload"
	GX, GY   int    // target grid cell for move
	TargetID string // target player ID for shoot/help
}

// shotEvent is broadcast to all players when a shot is fired.
type shotEvent struct {
	Type      string `json:"type"`
	ShooterID string `json:"shooterID"`
	TargetID  string `json:"targetID"`
}

// reloadingEvent is broadcast to all other players when a player starts reloading.
type reloadingEvent struct {
	Type     string `json:"type"`
	PlayerID string `json:"playerID"`
	Nickname string `json:"nickname"`
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
	Raw  string `json:"raw"` // raw Nano units as decimal string, for client-side math
}

// healItemState is the position of one heal pack broadcast to clients each tick.
type healItemState struct {
	GX int `json:"gx"`
	GY int `json:"gy"`
}

// playerState is the per-player snapshot included in each broadcast.
type playerState struct {
	ID       string `json:"id"`
	GX       int    `json:"gx"`
	GY       int    `json:"gy"`
	Health   int    `json:"health"`
	Color    string `json:"color"`
	Team     string `json:"team"`
	Nickname string `json:"nickname"`
}

// worldState is the full game snapshot sent to every client each tick.
type worldState struct {
	Type      string          `json:"type"`
	Players   []playerState   `json:"players"`
	HealItems []healItemState `json:"healItems"`
}

// Room represents one active game session with its own goroutine and tick loop.
type Room struct {
	ID                 string
	Mode               string // "paid" (default) or "faucet"
	DisableSameIPCheck bool   // when true, same-IP kills/heals still earn faucet rewards
	players            map[string]*Player
	healItems          map[[2]int]bool // grid cells that currently hold a heal pack
	inputCh            chan Input
	done               chan struct{}
	mu                 sync.RWMutex
	redCount           int      // red-team players ever joined; used for colour and spawn assignment
	blueCount          int      // blue-team players ever joined; used for colour and spawn assignment
	shotCostRaw        *big.Int // cost per shot in raw Nano units (unused in faucet mode)
}

// NewRoom creates a Room ready to accept players in the given mode ("paid" or "faucet").
func NewRoom(id, mode string) *Room {
	r := &Room{
		ID:          id,
		Mode:        mode,
		players:     make(map[string]*Player),
		healItems:   make(map[[2]int]bool),
		inputCh:     make(chan Input, 256),
		done:        make(chan struct{}),
		shotCostRaw: new(big.Int).Set(defaultShotCostRaw),
	}
	r.initHealItems()
	return r
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

// Join adds a player to the room, assigning their team colour and spawn position.
// Red-team players receive warm colours; blue-team players receive cool colours.
func (r *Room) Join(p *Player) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var spawn [2]int
	if p.Team == "red" {
		p.Color = redColorPalette[r.redCount%len(redColorPalette)]
		spawn = redSpawnPoints[r.redCount%len(redSpawnPoints)]
		r.redCount++
	} else {
		p.Color = blueColorPalette[r.blueCount%len(blueColorPalette)]
		spawn = blueSpawnPoints[r.blueCount%len(blueSpawnPoints)]
		r.blueCount++
	}

	p.GX, p.GY = spawn[0], spawn[1]
	p.SpawnGX, p.SpawnGY = spawn[0], spawn[1]

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

// initHealItems seeds the room with the initial set of heal packs.
// Called once from NewRoom — no concurrent access yet so no lock needed.
func (r *Room) initHealItems() {
	for i := 0; i < healItemCount; i++ {
		if cell, ok := r.randomFreeCell(); ok {
			r.healItems[cell] = true
		}
	}
}

// spawnHealItem adds one heal pack at a random free cell and broadcasts the update.
// Called from a goroutine after a pack is consumed.
func (r *Room) spawnHealItem() {
	r.mu.Lock()
	if cell, ok := r.randomFreeCell(); ok {
		r.healItems[cell] = true
	}
	r.mu.Unlock()
	r.broadcastState()
}

// randomFreeCell returns a random grid cell that is not a barrier and does not
// already hold a heal pack. Must be called with r.mu held (or before the room starts).
func (r *Room) randomFreeCell() ([2]int, bool) {
	candidates := make([][2]int, 0, GridCols*GridRows)
	for gy := 0; gy < GridRows; gy++ {
		for gx := 0; gx < GridCols; gx++ {
			cell := [2]int{gx, gy}
			if !IsBarrier(gx, gy) && !r.healItems[cell] {
				candidates = append(candidates, cell)
			}
		}
	}
	if len(candidates) == 0 {
		return [2]int{}, false
	}
	return candidates[rand.Intn(len(candidates))], true
}

// applyInput dispatches to the correct handler based on the action type.
func (r *Room) applyInput(input Input) {
	switch input.Action {
	case "shoot":
		r.applyShoot(input)
	case "help":
		r.applyHelp(input)
	case "reload":
		r.applyReload(input)
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
	if !ok || p.Health == 0 {
		return // dead players cannot move
	}

	// Server enforces the movement radius — clients cannot teleport beyond it.
	if !isValidMove(p.GX, p.GY, input.GX, input.GY) {
		return
	}

	// In faucet mode, players cannot enter barrier cells.
	if r.Mode == "faucet" && IsBarrier(input.GX, input.GY) {
		return
	}

	p.GX, p.GY = clampToGrid(input.GX, input.GY)

	// Pick up a heal pack if the player stepped onto one.
	cell := [2]int{p.GX, p.GY}
	if r.healItems[cell] {
		delete(r.healItems, cell)
		if p.Health+33 > 99 {
			p.Health = 99
		} else {
			p.Health += 33
		}
		go func() {
			time.Sleep(healItemRespawn)
			r.spawnHealItem()
		}()
	}
}

// applyShoot handles a shoot action: validates, deducts shot cost, applies damage,
// and on a kill awards the prize then schedules a round restart after 3 seconds.
func (r *Room) applyShoot(input Input) {
	r.mu.Lock()

	shooter, ok := r.players[input.PlayerID]
	if !ok || shooter.Health == 0 {
		r.mu.Unlock()
		return // dead players cannot shoot
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

	// In paid mode, validate that the target is within shooting range.
	if r.Mode != "faucet" && !isValidMove(shooter.GX, shooter.GY, target.GX, target.GY) {
		r.mu.Unlock()
		return
	}

	// In faucet mode, barriers block line-of-sight.
	if r.Mode == "faucet" && !HasLineOfSight(shooter.GX, shooter.GY, target.GX, target.GY) {
		r.mu.Unlock()
		return
	}

	// In paid mode: deduct shot cost. In faucet mode: shots are free.
	var shooterBalanceXNO string
	if r.Mode != "faucet" {
		shooter.BalanceRaw.Sub(shooter.BalanceRaw, r.shotCostRaw)
		shooterBalanceXNO = shooter.BalanceXNO()
	}

	target.Health -= 33
	isKill := target.Health == 0

	// Check whether every player on the target's team is now dead.
	teamWiped := false
	if isKill {
		teamWiped = true
		for _, p := range r.players {
			if p.Team == target.Team && p.Health > 0 {
				teamWiped = false
				break
			}
		}
	}

	// Broadcast the shot event so clients can animate the bullet.
	evt, _ := json.Marshal(shotEvent{
		Type:      "shot",
		ShooterID: input.PlayerID,
		TargetID:  input.TargetID,
	})
	for _, p := range r.players {
		p.Send(evt)
	}

	var prizeXNO string
	if isKill {
		diedMsg, _ := json.Marshal(map[string]string{"type": "died"})
		target.Send(diedMsg)

		if r.Mode != "faucet" {
			prize := new(big.Int).Mul(r.shotCostRaw, big.NewInt(3))
			shooter.BalanceRaw.Add(shooter.BalanceRaw, prize)
			shooterBalanceXNO = shooter.BalanceXNO()
			prizeXNO = FormatXNO(r.shotCostRaw)
		} else {
			prizeXNO = faucetRewardXNO
			sameIP := !r.DisableSameIPCheck && shooter.RemoteAddr != "" && shooter.RemoteAddr == target.RemoteAddr
			if sameIP {
				log.Printf("faucet: same-IP kill blocked (IP %s)", shooter.RemoteAddr)
				notice, _ := json.Marshal(map[string]string{
					"type":    "faucet_sameip",
					"message": "Please help the developer — be fair, play with other players, do not try to cheat the game 🙏",
				})
				shooter.Send(notice)
			} else if shooter.FaucetRewardCh != nil {
				select {
				case shooter.FaucetRewardCh <- "kill":
				default:
				}
			}
		}

		if teamWiped {
			roundOver, _ := json.Marshal(roundOverEvent{
				Type:     "roundover",
				KillerID: input.PlayerID,
				Prize:    prizeXNO,
			})
			for _, p := range r.players {
				p.Send(roundOver)
			}
		}
	}

	r.mu.Unlock()

	if r.Mode != "faucet" {
		balMsg, _ := json.Marshal(balanceEvent{Type: "balance", XNO: shooterBalanceXNO, Raw: shooter.BalanceRaw.String()})
		shooter.Send(balMsg)
	}

	if teamWiped {
		go func() {
			time.Sleep(3 * time.Second)
			r.restartRound()
		}()
	}
}

// applyReload notifies all other players that a player has started reloading.
func (r *Room) applyReload(input Input) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.players[input.PlayerID]
	if !ok || p.Health == 0 {
		return
	}

	evt, _ := json.Marshal(reloadingEvent{
		Type:     "reloading",
		PlayerID: p.ID,
		Nickname: p.Nickname,
	})
	for _, other := range r.players {
		if other.ID != p.ID {
			other.Send(evt)
		}
	}
}

// applyHelp handles a medical help action.
// A player adjacent to an incapacitated teammate can restore them to partial health.
func (r *Room) applyHelp(input Input) {
	r.mu.Lock()
	defer r.mu.Unlock()

	helper, ok := r.players[input.PlayerID]
	if !ok || helper.Health == 0 {
		return
	}

	target, ok := r.players[input.TargetID]
	if !ok || target.Health != 33 {
		return // can only help incapacitated players (health == 33)
	}

	if helper.Team != target.Team {
		return
	}

	dx := helper.GX - target.GX
	dy := helper.GY - target.GY
	if dx < -1 || dx > 1 || dy < -1 || dy > 1 {
		return
	}

	target.Health += 33

	if r.Mode == "faucet" && helper.FaucetRewardCh != nil {
		sameIP := !r.DisableSameIPCheck && helper.RemoteAddr != "" && helper.RemoteAddr == target.RemoteAddr
		if sameIP {
			log.Printf("faucet: same-IP heal blocked (IP %s)", helper.RemoteAddr)
			notice, _ := json.Marshal(map[string]string{
				"type":    "faucet_sameip",
				"message": "Please help the developer — be fair, play with other players, do not try to cheat the game 🙏",
			})
			helper.Send(notice)
		} else {
			select {
			case helper.FaucetRewardCh <- "heal":
			default:
			}
		}
	}

	evt, _ := json.Marshal(helpEvent{
		Type:     "helped",
		HelperID: input.PlayerID,
		TargetID: input.TargetID,
	})
	for _, p := range r.players {
		p.Send(evt)
	}
}

// restartRound resets every player to full health at their spawn position.
func (r *Room) restartRound() {
	r.mu.Lock()

	newRound, _ := json.Marshal(map[string]string{"type": "newround"})
	for _, p := range r.players {
		p.Health = 99
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

	state := worldState{
		Type:      "state",
		Players:   make([]playerState, 0, len(r.players)),
		HealItems: make([]healItemState, 0, len(r.healItems)),
	}
	for _, p := range r.players {
		state.Players = append(state.Players, playerState{
			ID:       p.ID,
			GX:       p.GX,
			GY:       p.GY,
			Health:   p.Health,
			Color:    p.Color,
			Team:     p.Team,
			Nickname: p.Nickname,
		})
	}
	for cell := range r.healItems {
		state.HealItems = append(state.HealItems, healItemState{GX: cell[0], GY: cell[1]})
	}

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	for _, p := range r.players {
		p.Send(data)
	}
}
