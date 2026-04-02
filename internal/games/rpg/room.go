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

// Input represents a player action submitted to a room.
type Input struct {
	PlayerID string
	Action   string // "move" or "chat"
	X, Y     int    // target cell column and row (for move)
	Text     string // message body (for chat)
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

// AddPlayer adds a player to the room, assigns color and a random empty spawn cell,
// and notifies all existing players of the join.
func (r *Room) AddPlayer(p *Player) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.Color = colorPalette[r.colorIdx%len(colorPalette)]
	r.colorIdx++
	p.X, p.Y = r.randomEmptyLocked()
	r.players[p.ID] = p
	r.broadcastSystemLocked(p.Username + " entered the world")
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

// randomEmptyLocked returns a random unoccupied grid cell.
// Caller must hold r.mu. Falls back to (0,0) if every cell is occupied.
func (r *Room) randomEmptyLocked() (int, int) {
	maxAttempts := GridW * GridH * 2
	for range maxAttempts {
		x := rand.Intn(GridW)
		y := rand.Intn(GridH)
		occupied := false
		for _, p := range r.players {
			if p.X == x && p.Y == y {
				occupied = true
				break
			}
		}
		if !occupied {
			return x, y
		}
	}
	return 0, 0 // fallback: grid is completely full
}
