// hub.go is the central registry of active RPG rooms.
package rpg

import "sync"

// Hub manages the set of active RPG game rooms.
type Hub struct {
	mu    sync.Mutex
	rooms map[string]*Room
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{rooms: make(map[string]*Room)}
}

// JoinRoom adds a player to the named room, creating it on first use.
// entryX/entryY specify the desired spawn cell; pass negative values for random spawn.
// Returns the room that was joined so the caller can submit inputs.
func (h *Hub) JoinRoom(roomID string, p *Player, entryX, entryY int) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	room, ok := h.rooms[roomID]
	if !ok {
		room = newRoom(roomID)
		h.rooms[roomID] = room
	}
	room.AddPlayer(p, entryX, entryY)
	return room
}

// LeaveRoom removes a player from their room and stops the room when it becomes empty.
func (h *Hub) LeaveRoom(roomID, playerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room, ok := h.rooms[roomID]
	if !ok {
		return
	}
	if room.RemovePlayer(playerID) {
		room.Stop()
		delete(h.rooms, roomID)
	}
}

// RoomCount returns the number of currently active rooms.
func (h *Hub) RoomCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rooms)
}
