// rooms.go serves the /api/rooms endpoint used by the lobby to list active rooms.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/allanabrahao/nanomultiplayer/internal/games"
)

// RoomsHandler returns a JSON array of currently active rooms.
type RoomsHandler struct {
	hub *games.Hub
}

// NewRoomsHandler wires up the hub dependency.
func NewRoomsHandler(hub *games.Hub) *RoomsHandler {
	return &RoomsHandler{hub: hub}
}

func (h *RoomsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rooms := h.hub.ActiveRooms()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}
