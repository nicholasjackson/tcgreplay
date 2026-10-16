// Package webui serves the playmat: a Svelte app embedded in the binary and
// a websocket that pushes the board on every change. The UI is a dumb
// renderer; every interaction becomes a verb on the server.
package webui

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// dist holds the built Svelte app under dist/app (see web/vite.config.js)
// and a placeholder page used when the app has not been built.
//
//go:embed all:dist
var dist embed.FS

// Server is the playmat's HTTP and websocket server.
type Server struct {
	M *game.Manager
	// Human is the seat the UI plays; Claude plays the other.
	Human state.Seat
	hub   *hub
}

// New returns a server for m with the UI playing human.
func New(m *game.Manager, human state.Seat) *Server {
	s := &Server{M: m, Human: human}
	s.hub = newHub(s)
	m.OnLoad(s.hub.watch)
	if g, err := m.Current(); err == nil {
		s.hub.watch(g)
	}
	return s
}

// Run serves until ctx is done.
func (s *Server) Run(ctx context.Context, ln net.Listener) error {
	go s.hub.run(ctx)
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Handler returns the HTTP routes. The hub must be running (Run starts it;
// tests call StartHub).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws", s.hub.serveWS)
	mux.HandleFunc("GET /api/card", s.serveCard)
	mux.Handle("GET /", http.FileServerFS(assets()))
	return localOnly(mux)
}

// StartHub runs the update hub until ctx is done, for use with Handler.
func (s *Server) StartHub(ctx context.Context) { go s.hub.run(ctx) }

// assets returns the built app, or the placeholder if it is missing.
func assets() fs.FS {
	if app, err := fs.Sub(dist, "dist/app"); err == nil {
		if _, err := fs.Stat(app, "index.html"); err == nil {
			return app
		}
	}
	ph, _ := fs.Sub(dist, "dist/placeholder")
	return ph
}

// localOnly rejects requests whose Host is not a loopback name, so a web
// page on another site cannot reach the playmat through DNS rebinding.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			http.Error(w, "playmat is only served to localhost", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CardInfo is what the UI needs to render and zoom a card.
type CardInfo struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Category    string          `json:"category"`
	Stage       string          `json:"stage,omitempty"`
	TrainerType string          `json:"trainer_type,omitempty"`
	EnergyType  string          `json:"energy_type,omitempty"`
	HP          int             `json:"hp,omitempty"`
	Types       []string        `json:"types,omitempty"`
	EvolveFrom  string          `json:"evolve_from,omitempty"`
	Retreat     int             `json:"retreat,omitempty"`
	Attacks     []cards.Attack  `json:"attacks,omitempty"`
	Abilities   []cards.Ability `json:"abilities,omitempty"`
	// Image is the TCGdex image base URL; append /low.webp or /high.webp.
	Image string `json:"image,omitempty"`
	Text  string `json:"text"`
}

func (s *Server) serveCard(w http.ResponseWriter, r *http.Request) {
	def := r.URL.Query().Get("def")
	if def == "" || s.M.DB == nil {
		http.Error(w, "unknown card", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	c, err := s.M.DB.Card(ctx, def)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=86400")
	json.NewEncoder(w).Encode(CardInfo{
		ID: c.ID, Name: c.Name, Category: c.Category, Stage: c.Stage,
		TrainerType: c.TrainerType, EnergyType: c.EnergyType, HP: c.HP,
		Types: c.Types, EvolveFrom: c.EvolveFrom, Retreat: c.Retreat,
		Attacks: c.Attacks, Abilities: c.Abilities, Image: c.Image, Text: c.Text(),
	})
}
