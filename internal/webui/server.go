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
	"net/url"
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
	// Public is the address the playmat is reached at in hosted mode. When
	// set, the loopback Host check is skipped and pages on this address may
	// open the live connection. Nil (the desktop) keeps both loopback-only.
	Public *url.URL
	// Limit, when set (hosted mode), is called before each board action
	// with the live connection's request context; an error refuses the
	// action with its message. Nil (the desktop) never refuses.
	Limit func(ctx context.Context) error
	// Images, when set (hosted mode with the shared store only), decides
	// where the board loads each card image from. Nil (the desktop, the
	// daemon, dev and the board test server) keeps the TCGdex address.
	Images ImageSource
	hub    *hub
}

// ImageSource decides where the board loads a card image from. Image is
// given the card's TCGdex image base address and returns the base address
// to load (to which the board appends /low.webp or /high.webp), when that
// differs from src the address to fall back to, and the content version of
// the image at that address ("" when there is none), which changes
// whenever its bytes do. Implementations
// must never block or touch the network: the card endpoint answers on the
// board's critical path.
type ImageSource interface {
	Image(src string) (image, fallback, version string)
}

// New returns a server for m with the UI playing human.
func New(m *game.Manager, human state.Seat) *Server {
	s := &Server{M: m, Human: human}
	s.hub = newHub(s)
	m.OnLoad(s.hub.watch)
	m.OnClear(s.hub.clear)
	m.OnPresence(s.hub.wake)
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
	if s.Public != nil {
		return mux
	}
	return localOnly(mux)
}

// borrowedArt is the art for a card TCGdex has no image for (a new promo,
// say): the image of the print of the same name and moves that the card
// database prefers, which is one with art. Only the picture is borrowed;
// the card keeps its identity. "" if there is none.
func (s *Server) borrowedArt(ctx context.Context, c *cards.Card) string {
	if c.Category == "Energy" {
		return "" // Basic Energy art is borrowed by the card database
	}
	var moves []string
	for _, a := range c.Attacks {
		moves = append(moves, a.Name)
	}
	for _, a := range c.Abilities {
		moves = append(moves, a.Name)
	}
	id, err := s.M.DB.ByName(ctx, c.Name, moves...)
	if err != nil || id == "" || id == c.ID {
		return ""
	}
	alt, err := s.M.DB.Card(ctx, id)
	if err != nil || !cards.SameName(alt.Name, c.Name) {
		return ""
	}
	return alt.Image
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
	// Image is the base address to load the card's image from (the CDN
	// copy in hosted mode with the shared store, else TCGdex); append
	// /low.webp or /high.webp.
	Image string `json:"image,omitempty"`
	// ImageFallback is the TCGdex base address, set only when Image
	// differs from it; the board loads it if Image fails.
	ImageFallback string `json:"image_fallback,omitempty"`
	// ImageVersion is the content version of the copy at Image, set only
	// when Image is a copy. The board appends ?v=<image_version> to the
	// Image address with its size (never to the fallback), so a browser or
	// CDN holding an old copy fetches changed art again.
	ImageVersion string `json:"image_version,omitempty"`
	Text         string `json:"text"`
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
	img, fb, ver := c.Image, "", ""
	if img == "" {
		img = s.borrowedArt(ctx, c)
	}
	if src := img; s.Images != nil && src != "" {
		img, fb, ver = s.Images.Image(src)
		if fb == img {
			fb = ""
		}
		if img == src {
			ver = "" // a version only ever names a copy
		}
	}
	w.Header().Set("Content-Type", "application/json")
	// Revalidate: what is served for a card (e.g. borrowed Energy art) can
	// change between builds, and the server is local.
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(CardInfo{
		ID: c.ID, Name: c.Name, Category: c.Category, Stage: c.Stage,
		TrainerType: c.TrainerType, EnergyType: c.EnergyType, HP: c.HP,
		Types: c.Types, EvolveFrom: c.EvolveFrom, Retreat: c.Retreat,
		Attacks: c.Attacks, Abilities: c.Abilities, Image: img, ImageFallback: fb, ImageVersion: ver, Text: c.Text(),
	})
}
