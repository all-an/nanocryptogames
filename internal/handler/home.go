// home.go serves the landing page (mode selector) and the docs page.
package handler

import (
	"html/template"
	"net/http"

	"github.com/allanabrahao/nanomultiplayer/internal/db"
)

// LandingHandler serves the top-level mode-selector page.
type LandingHandler struct {
	tmpl       *template.Template
	db         *db.DB
	faucetAddr string
}

// NewLandingHandler wires up the landing template and optional DB for settings.
func NewLandingHandler(tmpl *template.Template, database *db.DB, faucetAddr string) *LandingHandler {
	return &LandingHandler{tmpl: tmpl, db: database, faucetAddr: faucetAddr}
}

// DocsHandler serves the documentation page.
type DocsHandler struct {
	tmpl *template.Template
}

// NewDocsHandler wires up the docs template.
func NewDocsHandler(tmpl *template.Template) *DocsHandler {
	return &DocsHandler{tmpl: tmpl}
}

func (h *DocsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.tmpl.ExecuteTemplate(w, "index.html", nil)
}

// ShooterCodeFlowHandler serves the shooter code-flow diagram page.
type ShooterCodeFlowHandler struct {
	tmpl *template.Template
}

// NewShooterCodeFlowHandler wires up the shooter code-flow template.
func NewShooterCodeFlowHandler(tmpl *template.Template) *ShooterCodeFlowHandler {
	return &ShooterCodeFlowHandler{tmpl: tmpl}
}

func (h *ShooterCodeFlowHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.tmpl.ExecuteTemplate(w, "shooter-code-flow.html", nil)
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
	h.tmpl.ExecuteTemplate(w, "landing.html", map[string]any{
		"DisablePaid":   disablePaid,
		"FaucetAddress": h.faucetAddr,
	})
}
