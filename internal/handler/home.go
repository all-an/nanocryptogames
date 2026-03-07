// home.go serves the lobby page where players create or join a room.
package handler

import (
	"html/template"
	"net/http"
)

// HomeHandler serves the lobby page.
type HomeHandler struct {
	tmpl *template.Template
}

// NewHomeHandler wires up the lobby template.
func NewHomeHandler(tmpl *template.Template) *HomeHandler {
	return &HomeHandler{tmpl: tmpl}
}

func (h *HomeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only handle the root path; let the mux 404 everything else.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	h.tmpl.ExecuteTemplate(w, "lobby.html", nil)
}
