// home.go serves the landing page (mode selector) and the paid-mode welcome page.
package handler

import (
	"html/template"
	"net/http"

	"github.com/allanabrahao/nanomultiplayer/internal/db"
)

// LandingHandler serves the top-level mode-selector page.
type LandingHandler struct {
	tmpl *template.Template
	db   *db.DB
}

// NewLandingHandler wires up the landing template and optional DB for settings.
func NewLandingHandler(tmpl *template.Template, database *db.DB) *LandingHandler {
	return &LandingHandler{tmpl: tmpl, db: database}
}

func (h *LandingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only handle the root path; let the mux 404 everything else.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	disablePaid := false
	if h.db != nil {
		val, _ := h.db.Setting(r.Context(), "disable_paid")
		disablePaid = val == "true"
	}
	h.tmpl.ExecuteTemplate(w, "landing.html", map[string]bool{
		"DisablePaid": disablePaid,
	})
}

// WelcomeHandler serves the paid-multiplayer welcome/info page.
type WelcomeHandler struct {
	tmpl *template.Template
}

// NewWelcomeHandler wires up the welcome template.
func NewWelcomeHandler(tmpl *template.Template) *WelcomeHandler {
	return &WelcomeHandler{tmpl: tmpl}
}

func (h *WelcomeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.tmpl.ExecuteTemplate(w, "welcome.html", nil)
}
