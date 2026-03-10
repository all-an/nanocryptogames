// lobby.go serves the room list and team-picker page at /lobby.
package handler

import (
	"html/template"
	"net/http"
)

// LobbyHandler serves the lobby page.
type LobbyHandler struct {
	tmpl *template.Template
}

// NewLobbyHandler wires up the lobby template.
func NewLobbyHandler(tmpl *template.Template) *LobbyHandler {
	return &LobbyHandler{tmpl: tmpl}
}

func (h *LobbyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.tmpl.ExecuteTemplate(w, "paid_lobby.html", nil)
}
