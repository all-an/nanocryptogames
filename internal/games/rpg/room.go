// room.go implements one RPG game room: a 20×15 numbered grid with
// server-authoritative player movement and chat.
package rpg

import (
	"encoding/json"
	"log"
	"math/rand"
	"sync"
	"time"
)

// GridW and GridH define the RPG grid dimensions.
// 20 × 15 = 300 cells, numbered 1–300 left-to-right, top-to-bottom.
const (
	GridW = 20
	GridH = 15
)

// SpawnX and SpawnY are the default spawn coordinates for room "0,0" (cell 161).
const (
	SpawnX = 0
	SpawnY = 8
)

// roomBlocks maps room IDs to their impassable cell coordinates {x, y}.
var roomBlocks = map[string][][2]int{
	"0,0": {{6, 3}, {7, 3}, {8, 3}, {6, 4}, {8, 4}}, // cells 67,68,69,87,89
}

// roomDoors maps room IDs to passable door cell coordinates {x, y}.
// Door cells render with an arched door but players can walk through them.
var roomDoors = map[string][][2]int{
	"0,0": {{7, 4}}, // cell 88
}

// Input represents a player action submitted to a room.
type Input struct {
	PlayerID string
	Action   string // "move", "chat", or "dm"
	X, Y     int    // target cell column and row (for move)
	Text     string // message body (for chat/dm)
	To       string // target username (for dm)
}

// Room manages the players and game state of one RPG room.
type Room struct {
	ID       string
	mu       sync.Mutex
	players  map[string]*Player
	inputs   chan Input
	done     chan struct{}
	colorIdx int // cycles through colorPalette on each join
}

// newRoom creates a room and starts its game loop goroutine.
func newRoom(id string) *Room {
	r := &Room{
		ID:      id,
		players: make(map[string]*Player),
		inputs:  make(chan Input, 256),
		done:    make(chan struct{}),
	}
	go r.loop()
	return r
}

// Submit enqueues a player action for processing in the next iteration.
func (r *Room) Submit(inp Input) {
	select {
	case r.inputs <- inp:
	default:
	}
}

// Blocks returns the impassable cells for this room (sent to the client on init).
func (r *Room) Blocks() [][2]int {
	return roomBlocks[r.ID]
}

// Doors returns the passable door cells for this room (sent to the client on init).
func (r *Room) Doors() [][2]int {
	return roomDoors[r.ID]
}

// isBlockedLocked reports whether (x, y) is an impassable cell in this room.
// Caller must hold r.mu.
func (r *Room) isBlockedLocked(x, y int) bool {
	for _, b := range roomBlocks[r.ID] {
		if b[0] == x && b[1] == y {
			return true
		}
	}
	return false
}

// AddPlayer adds a player to the room, assigns color, and spawns them.
// Room "0,0" spawns at cell 161 (0,8), shifting up/down if occupied.
// Other rooms use the provided entryX/entryY, or random if invalid.
func (r *Room) AddPlayer(p *Player, entryX, entryY int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Use the player's saved color if they have one; otherwise cycle the palette.
	if p.Color == "" {
		p.Color = colorPalette[r.colorIdx%len(colorPalette)]
		r.colorIdx++
	}

	validEntry := entryX >= 0 && entryX < GridW && entryY >= 0 && entryY < GridH &&
		!r.occupiedLocked(entryX, entryY) && !r.isBlockedLocked(entryX, entryY)

	if validEntry {
		p.X, p.Y = entryX, entryY
	} else if r.ID == "0,0" {
		p.X, p.Y = r.spawnNearLocked(SpawnX, SpawnY)
	} else {
		p.X, p.Y = r.randomEmptyLocked()
	}
	r.players[p.ID] = p
	r.broadcastSystemLocked(p.Username + " entered the world")
}

// spawnNearLocked places a player at (x, y) in room "0,0", falling back to
// cells above then below if occupied or blocked. Caller must hold r.mu.
func (r *Room) spawnNearLocked(x, y int) (int, int) {
	if !r.occupiedLocked(x, y) && !r.isBlockedLocked(x, y) {
		return x, y
	}
	for d := 1; d < GridH; d++ {
		if up := y - d; up >= 0 && !r.occupiedLocked(x, up) && !r.isBlockedLocked(x, up) {
			return x, up
		}
		if down := y + d; down < GridH && !r.occupiedLocked(x, down) && !r.isBlockedLocked(x, down) {
			return x, down
		}
	}
	return r.randomEmptyLocked()
}

// occupiedLocked reports whether (x, y) is already taken by another player.
// Caller must hold r.mu.
func (r *Room) occupiedLocked(x, y int) bool {
	for _, p := range r.players {
		if p.X == x && p.Y == y {
			return true
		}
	}
	return false
}

// RemovePlayer removes a player from the room and notifies others.
// Returns true when the room is now empty (caller should stop and delete it).
func (r *Room) RemovePlayer(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.players[id]; ok {
		delete(r.players, id)
		r.broadcastSystemLocked(p.Username + " left the world")
	}
	return len(r.players) == 0
}

// Stop shuts down the room's game loop. Call only after RemovePlayer returns true.
func (r *Room) Stop() {
	close(r.done)
}

// loop runs at 10 TPS, draining inputs between ticks and broadcasting state each tick.
func (r *Room) loop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.done:
			return
		case inp := <-r.inputs:
			r.processInput(inp)
		case <-ticker.C:
			r.broadcastState()
		}
	}
}

// processInput applies a player action. Movement is validated server-side.
func (r *Room) processInput(inp Input) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.players[inp.PlayerID]
	if !ok {
		return
	}

	switch inp.Action {
	case "move":
		// Only allow movement to adjacent cells (8-directional, distance 1).
		dx := inp.X - p.X
		dy := inp.Y - p.Y
		if dx < -1 || dx > 1 || dy < -1 || dy > 1 || (dx == 0 && dy == 0) {
			return // not adjacent or same cell
		}
		if inp.X < 0 || inp.X >= GridW || inp.Y < 0 || inp.Y >= GridH {
			return // out of bounds
		}
		if r.isBlockedLocked(inp.X, inp.Y) {
			return // impassable block
		}
		// Prevent two players occupying the same cell.
		for _, other := range r.players {
			if other.ID != p.ID && other.X == inp.X && other.Y == inp.Y {
				return
			}
		}
		p.X = inp.X
		p.Y = inp.Y

	case "chat":
		text := inp.Text
		if text == "" || len([]rune(text)) > 200 {
			return
		}
		r.broadcastChatLocked(p.Username, text)

	case "color":
		// Update the player's live color so future state broadcasts carry it.
		c := inp.Text
		if len(c) != 7 || c[0] != '#' {
			return
		}
		p.Color = c

	case "username":
		// Update the player's live username so future state broadcasts carry it.
		u := inp.Text
		if u == "" || len([]rune(u)) > 20 {
			return
		}
		p.Username = u

	case "dm":
		text := inp.Text
		if text == "" || len([]rune(text)) > 200 {
			return
		}
		// Find target by username; silently drop if not in this room.
		var target *Player
		for _, other := range r.players {
			if other.Username == inp.To {
				target = other
				break
			}
		}
		if target == nil {
			return
		}
		r.sendDMLocked(p.Username, target.Username, text, p, target)
	}
}

// broadcastState sends the current player grid positions to all connected players.
func (r *Room) broadcastState() {
	r.mu.Lock()
	defer r.mu.Unlock()

	type playerSnap struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		X        int    `json:"x"`
		Y        int    `json:"y"`
		Color    string `json:"color"`
	}

	snaps := make([]playerSnap, 0, len(r.players))
	for _, p := range r.players {
		snaps = append(snaps, playerSnap{
			ID:       p.ID,
			Username: p.Username,
			X:        p.X,
			Y:        p.Y,
			Color:    p.Color,
		})
	}

	msg, err := json.Marshal(map[string]any{
		"type":    "state",
		"players": snaps,
	})
	if err != nil {
		log.Printf("rpg room [%s]: broadcast marshal: %v", r.ID, err)
		return
	}
	for _, p := range r.players {
		p.Send(msg)
	}
}

// broadcastChatLocked delivers a chat message to all players. Caller must hold r.mu.
func (r *Room) broadcastChatLocked(from, text string) {
	msg, _ := json.Marshal(map[string]string{
		"type": "chat",
		"from": from,
		"text": text,
	})
	for _, p := range r.players {
		p.Send(msg)
	}
}

// sendDMLocked delivers a private message to the target player and echoes it to
// the sender so both sides can render the conversation. Caller must hold r.mu.
func (r *Room) sendDMLocked(from, to, text string, sender, target *Player) {
	msg, _ := json.Marshal(map[string]string{
		"type": "dm",
		"from": from,
		"to":   to,
		"text": text,
	})
	target.Send(msg)
	sender.Send(msg) // echo so sender sees their own sent card
}

// broadcastSystemLocked delivers a system notification to all players. Caller must hold r.mu.
func (r *Room) broadcastSystemLocked(text string) {
	msg, _ := json.Marshal(map[string]string{
		"type": "system",
		"text": text,
	})
	for _, p := range r.players {
		p.Send(msg)
	}
}

// randomEmptyLocked returns a random unoccupied, unblocked grid cell.
// Caller must hold r.mu. Falls back to (0,0) if every cell is taken.
func (r *Room) randomEmptyLocked() (int, int) {
	maxAttempts := GridW * GridH * 2
	for range maxAttempts {
		x := rand.Intn(GridW)
		y := rand.Intn(GridH)
		if !r.occupiedLocked(x, y) && !r.isBlockedLocked(x, y) {
			return x, y
		}
	}
	return 0, 0 // fallback
}
