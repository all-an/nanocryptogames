// hub.go is the central registry of all active game rooms.
// It replaces Redis pub/sub with a mutex-protected in-process map.
package game

import "sync"

// Hub manages the set of live rooms.
// All room creation and teardown is serialised through the Hub mutex.
type Hub struct {
	mu    sync.Mutex
	rooms map[string]*Room
}

// RoomSummary is a lightweight snapshot of a room used for the lobby listing.
type RoomSummary struct {
	ID          string `json:"id"`
	PlayerCount int    `json:"playerCount"`
	RedCount    int    `json:"redCount"`
	BlueCount   int    `json:"blueCount"`
}

// NewHub creates an empty Hub ready for use.
func NewHub() *Hub {
	return &Hub{rooms: make(map[string]*Room)}
}

// ActiveRooms returns a snapshot of all rooms that currently have players.
func (h *Hub) ActiveRooms() []RoomSummary {
	h.mu.Lock()
	defer h.mu.Unlock()

	rooms := make([]RoomSummary, 0, len(h.rooms))
	for _, r := range h.rooms {
		red, blue := r.teamCounts()
		rooms = append(rooms, RoomSummary{
			ID:          r.ID,
			PlayerCount: red + blue,
			RedCount:    red,
			BlueCount:   blue,
		})
	}
	return rooms
}

// JoinRoom adds the player to the named room, creating the room if it does not exist.
// Returns the room so the caller can submit inputs to it directly.
func (h *Hub) JoinRoom(roomID string, p *Player) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()

	r, ok := h.rooms[roomID]
	if !ok {
		r = NewRoom(roomID)
		h.rooms[roomID] = r
		go r.Run()
	}

	r.Join(p)
	return r
}

// LeaveRoom removes the player from their room.
// If the room becomes empty it is stopped and deleted.
func (h *Hub) LeaveRoom(p *Player) {
	h.mu.Lock()
	defer h.mu.Unlock()

	r, ok := h.rooms[p.RoomID]
	if !ok {
		return
	}

	r.Leave(p)

	if r.Empty() {
		r.Close()
		delete(h.rooms, p.RoomID)
	}
}
