package game

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/store"
)

// Manager owns the current game for this session, in memory only: it never
// reads or writes game files, so every session starts with no game open.
// The data directory holds only the assistant label.
type Manager struct {
	Dir string
	DB  cards.DB
	// Rand, if set, seeds every new game (for tests).
	Rand func() *rand.Rand

	mu      sync.Mutex
	current *Game
	onLoad  []func(*Game)
	onClear []func()
	// notice, when set, says why there is no game (it expired); it is
	// cleared when a game becomes current.
	notice    string
	assistant string
	// presence tracks the assistant's turn wait on the current game; it is
	// made on first use.
	presence *presence

	// Timing holds the turn wait's settings; zero fields use the defaults.
	Timing Timing
}

// NewManager returns a manager with no game open; dir holds the assistant
// label.
func NewManager(dir string, db cards.DB) *Manager {
	return &Manager{Dir: dir, DB: db}
}

// SetAssistant records which assistant is connected over MCP (Claude,
// Codex, Gemini), for the playmat's labels. It is kept in the data dir so
// the label survives restarts.
func (m *Manager) SetAssistant(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name == "" || name == m.assistant {
		return
	}
	m.assistant = name
	_ = os.WriteFile(filepath.Join(m.Dir, "assistant.txt"), []byte(name+"\n"), 0o644)
}

// Assistant is the connected assistant's display name, or "Assistant" if
// none has connected yet.
func (m *Manager) Assistant() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.assistant == "" {
		b, _ := os.ReadFile(filepath.Join(m.Dir, "assistant.txt"))
		m.assistant = strings.TrimSpace(string(b))
	}
	if m.assistant == "" {
		return "Assistant"
	}
	return m.assistant
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

// ErrExpired is wrapped by Current's error after the game expired (see
// Expire).
var ErrExpired = errors.New("game expired")

// expiredError is Current's error after Expire: its text is the notice,
// and it matches ErrExpired.
type expiredError struct{ notice string }

func (e expiredError) Error() string        { return e.notice }
func (e expiredError) Is(target error) bool { return target == ErrExpired }

// OnClear registers a callback run whenever the current game is dropped
// without a new one (Expire).
func (m *Manager) OnClear(f func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onClear = append(m.onClear, f)
}

// Expire drops the current game, if any, freeing it, and makes Current
// report notice (an error matching ErrExpired) until a new game or import
// replaces it. Listeners registered with OnClear are told. Only hosted
// mode calls it, when a game has been idle too long.
func (m *Manager) Expire(notice string) {
	m.mu.Lock()
	m.current = nil
	m.notice = notice
	hooks := slices.Clone(m.onClear)
	m.mu.Unlock()
	m.presenceRef().cleared()
	for _, f := range hooks {
		f()
	}
}

// Notice returns why there is no game (the expiry notice), or "" while a
// game is open or none has expired.
func (m *Manager) Notice() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil {
		return ""
	}
	return m.notice
}

// Current returns the loaded game or an error.
func (m *Manager) Current() (*Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil && m.notice != "" {
		return nil, expiredError{m.notice}
	}
	if m.current == nil {
		return nil, errors.New("no game open: import a battle log or a saved game record with import_log, or start one with new_game")
	}
	return m.current, nil
}

// Record returns the current game's record: an error if no game is open
// or the game has none.
func (m *Manager) Record() (Record, error) {
	g, err := m.Current()
	if err != nil {
		return Record{}, err
	}
	rec, ok := g.Record()
	if !ok {
		return Record{}, fmt.Errorf("game %s was started with new_game, not imported from a battle log, so it has no record", g.ID)
	}
	return rec, nil
}

func (m *Manager) setCurrent(g *Game) {
	m.mu.Lock()
	// The previous game, if any, is simply dropped.
	m.current = g
	m.notice = ""
	hooks := slices.Clone(m.onLoad)
	m.mu.Unlock()
	m.presenceRef().gameChanged(g)
	for _, f := range hooks {
		f(g)
	}
}

func (m *Manager) rng() *rand.Rand {
	if m.Rand != nil {
		return m.Rand()
	}
	return nil
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
	if req.Source == "" {
		req.Source = "play"
	}
	l := store.NewMemory(store.Header{Game: req.ID, Created: time.Now().UTC(), Source: req.Source})
	g, err := Load(req.ID, l, m.DB, m.rng())
	if err != nil {
		return nil, nil, err
	}
	res, err := g.Setup(cardList, Options{Names: req.Names})
	if err != nil {
		return nil, nil, err
	}
	res.Warnings = append(warnings, res.Warnings...)
	m.setCurrent(g)
	return g, res, nil
}
