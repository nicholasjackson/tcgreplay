package game

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/store"
)

// Manager owns the game files and the currently loaded game.
type Manager struct {
	Dir string
	DB  cards.DB
	// Rand, if set, seeds every new game (for tests).
	Rand func() *rand.Rand

	mu        sync.Mutex
	current   *Game
	onLoad    []func(*Game)
}

// NewManager stores games under dir.
func NewManager(dir string, db cards.DB) *Manager {
	return &Manager{Dir: dir, DB: db}
}

// DefaultDataDir returns $XDG_DATA_HOME/tcgreplay or ~/.local/share/tcgreplay.
func DefaultDataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "tcgreplay")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "tcgreplay")
	}
	return filepath.Join(home, ".local", "share", "tcgreplay")
}

// OnLoad registers a callback run whenever a game becomes current.
func (m *Manager) OnLoad(f func(*Game)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onLoad = append(m.onLoad, f)
}

// Current returns the loaded game or an error.
func (m *Manager) Current() (*Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return nil, errors.New("no game loaded: call new_game or load_game")
	}
	return m.current, nil
}

func (m *Manager) setCurrent(g *Game) {
	m.mu.Lock()
	if m.current != nil && m.current != g {
		m.current.Close()
	}
	m.current = g
	hooks := slices.Clone(m.onLoad)
	m.mu.Unlock()
	for _, f := range hooks {
		f(g)
	}
}

func (m *Manager) gamesDir() string { return filepath.Join(m.Dir, "games") }

var idRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func (m *Manager) path(id string) (string, error) {
	if !idRe.MatchString(id) {
		return "", fmt.Errorf("bad game id %q", id)
	}
	return filepath.Join(m.gamesDir(), id+".jsonl"), nil
}

// GameInfo summarises a stored game.
type GameInfo struct {
	ID       string    `json:"id"`
	Modified time.Time `json:"modified"`
}

// List returns stored games, newest first.
func (m *Manager) List() ([]GameInfo, error) {
	entries, err := os.ReadDir(m.gamesDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []GameInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, GameInfo{ID: strings.TrimSuffix(e.Name(), ".jsonl"), Modified: info.ModTime()})
	}
	slices.SortFunc(out, func(a, b GameInfo) int { return b.Modified.Compare(a.Modified) })
	return out, nil
}

func (m *Manager) rng() *rand.Rand {
	if m.Rand != nil {
		return m.Rand()
	}
	return nil
}

// Load opens a stored game and makes it current.
func (m *Manager) Load(id string) (*Game, error) {
	p, err := m.path(id)
	if err != nil {
		return nil, err
	}
	l, err := store.Open(p)
	if err != nil {
		return nil, err
	}
	g, err := Load(id, l, m.DB, m.rng())
	if err != nil {
		l.Close()
		return nil, err
	}
	m.setCurrent(g)
	return g, nil
}

// NewGameRequest describes a new game.
type NewGameRequest struct {
	ID     string
	DeckP1 string
	DeckP2 string
	Names  map[state.Seat]string
	Source string
}

// BuildCards turns decklists into card instances: p1-1..p1-N in list order.
// Resolution problems are returned as warnings; unresolved cards keep their
// names.
func BuildCards(ctx context.Context, db cards.DB, lists map[state.Seat]string) ([]state.Card, []string, error) {
	var out []state.Card
	var warnings []string
	for _, seat := range state.Seats {
		l, err := deck.Parse(lists[seat])
		if err != nil {
			return nil, nil, fmt.Errorf("%s decklist: %w", seat, err)
		}
		if l.Total() != state.DeckSize {
			warnings = append(warnings, fmt.Sprintf("%s decklist has %d cards", seat, l.Total()))
		}
		if db != nil {
			if err := l.Resolve(ctx, db); err != nil {
				for _, e := range strings.Split(err.Error(), "\n") {
					warnings = append(warnings, fmt.Sprintf("%s: %s", seat, e))
				}
			}
		}
		n := 0
		for _, e := range l.Entries {
			name := e.Name
			if e.Def != "" && db != nil {
				if c, err := db.Card(ctx, e.Def); err == nil {
					name = c.Name
				}
			}
			for range e.Count {
				n++
				out = append(out, state.Card{IID: state.IID(fmt.Sprintf("%s-%d", seat, n)), Def: e.Def, Name: name, Owner: seat})
			}
		}
	}
	return out, warnings, nil
}

// New creates a game from two decklists, sets it up and makes it current.
func (m *Manager) New(ctx context.Context, req NewGameRequest) (*Game, *Result, error) {
	cardList, warnings, err := BuildCards(ctx, m.DB, map[state.Seat]string{state.P1: req.DeckP1, state.P2: req.DeckP2})
	if err != nil {
		return nil, nil, err
	}
	if req.ID == "" {
		req.ID = "game-" + time.Now().Format("20060102-150405")
	}
	p, err := m.path(req.ID)
	if err != nil {
		return nil, nil, err
	}
	if req.Source == "" {
		req.Source = "play"
	}
	l, err := store.Create(p, store.Header{Game: req.ID, Created: time.Now().UTC(), Source: req.Source})
	if err != nil {
		return nil, nil, err
	}
	g, err := Load(req.ID, l, m.DB, m.rng())
	if err != nil {
		l.Close()
		return nil, nil, err
	}
	res, err := g.Setup(cardList, Options{Names: req.Names})
	if err != nil {
		l.Close()
		os.Remove(p)
		return nil, nil, err
	}
	res.Warnings = append(warnings, res.Warnings...)
	m.setCurrent(g)
	return g, res, nil
}
