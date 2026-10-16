package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/importer"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/store"
)

// ImportRequest describes a TCG Live battle log to import. It is saved
// beside the game (<id>.import.json) so the game can be rebuilt, e.g. with
// an assumed opponent list.
type ImportRequest struct {
	ID string `json:"id"`
	// Log is the battle log text as copied from TCG Live.
	Log string `json:"log"`
	// Deck is the log owner's decklist in TCG Live export format. If
	// empty, the deck from the previous import is used.
	Deck string `json:"deck"`
	// OppDeck optionally assumes the opponent's list for unrevealed cards.
	OppDeck string `json:"opp_deck,omitempty"`
	// Me is the log owner's player name; detected if empty.
	Me string `json:"me,omitempty"`
	// Seed makes the reconstruction reproducible; 0 picks one at random.
	Seed uint64 `json:"seed"`
}

// ImportResult is the outcome of an import.
type ImportResult struct {
	Game     *Game
	Report   importer.Report
	Warnings []string
}

var nonID = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Import replays a battle log into a new game file, in review mode, and
// makes it current. The owner sits at p1 and the opponent at p2.
func (m *Manager) Import(ctx context.Context, req ImportRequest) (*ImportResult, error) {
	var warnings []string
	lastDeck := filepath.Join(m.Dir, "last-deck.txt")
	if strings.TrimSpace(req.Deck) == "" {
		b, err := os.ReadFile(lastDeck)
		if err != nil {
			return nil, fmt.Errorf("no decklist given and no previous import to reuse one from")
		}
		req.Deck = string(b)
	}
	parse := func(text, who string) ([]deck.Entry, error) {
		entries, w, err := m.ParseDeck(ctx, text, who)
		warnings = append(warnings, w...)
		return entries, err
	}
	mine, err := parse(req.Deck, "deck")
	if err != nil {
		return nil, err
	}
	theirs, err := parse(req.OppDeck, "opponent deck")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(m.Dir, 0o755); err == nil {
		if err := os.WriteFile(lastDeck, []byte(req.Deck), 0o644); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not remember the decklist: %v", err))
		}
	}
	opts := importer.Options{Me: req.Me, MyDeck: mine, OppDeck: theirs, Seed: req.Seed}
	if m.DB != nil {
		opts.DefOf = func(name string) string {
			if id, ok := deck.BasicEnergyDef(name); ok {
				return id
			}
			id, _ := m.DB.ByName(ctx, name)
			return id
		}
	}
	res, err := importer.Import(req.Log, opts)
	if err != nil {
		return nil, err
	}

	if req.ID == "" {
		req.ID = "import-" + time.Now().Format("20060102-150405")
		if opp := nonID.ReplaceAllString(res.Report.Opponent, ""); opp != "" {
			req.ID += "-" + strings.ToLower(opp)
		}
	}
	p, err := m.path(req.ID)
	if err != nil {
		return nil, err
	}
	l, err := store.Create(p, store.Header{Game: req.ID, Created: time.Now().UTC(), Source: "import"})
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*ImportResult, error) {
		l.Close()
		os.Remove(p)
		return nil, err
	}
	for _, s := range res.Steps {
		if len(s.Events) == 0 {
			continue
		}
		a := store.Action{ID: l.NextActionID(), Label: s.Label, Kind: s.Kind, Seat: s.Seat, Actor: state.ActorImport}
		if err := l.PutAction(a); err != nil {
			return fail(err)
		}
		for i := range s.Events {
			s.Events[i].Action = a.ID
		}
		if err := l.Append(s.Events); err != nil {
			return fail(err)
		}
	}
	g, err := Load(req.ID, l, m.DB, m.rng())
	if err != nil {
		return fail(fmt.Errorf("imported events do not fold: %w", err))
	}
	req.Seed = res.Seed
	if b, err := json.Marshal(req); err == nil {
		if err := os.WriteFile(strings.TrimSuffix(p, ".jsonl")+".import.json", b, 0o644); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not save the import for rebuilding: %v", err))
		}
	}
	warnings = append(warnings, res.Report.Warnings...)
	warnings = append(warnings, g.st.Check()...)
	m.setCurrent(g)
	return &ImportResult{Game: g, Report: res.Report, Warnings: warnings}, nil
}

// ParseDeck parses a TCG Live export and resolves card definitions where
// possible; unresolved cards come back as warnings. Empty text is no list.
func (m *Manager) ParseDeck(ctx context.Context, text, who string) ([]deck.Entry, []string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil, nil
	}
	l, err := deck.Parse(text)
	if err != nil {
		return nil, nil, fmt.Errorf("%s decklist: %w", who, err)
	}
	var warnings []string
	if m.DB != nil {
		if err := l.Resolve(ctx, m.DB); err != nil {
			for _, e := range strings.Split(err.Error(), "\n") {
				warnings = append(warnings, fmt.Sprintf("%s: %s", who, e))
			}
		}
	}
	return l.Entries, warnings, nil
}

// SetOpponentDeck rebuilds an imported game with an assumed opponent list:
// the same log and seed, so the owner's side and everything the log shows
// come out the same, and every card the log never names is dealt at random
// from what the list has left. The game is replaced, so it refuses when
// branches other than main exist; pass newID to build a separate game.
func (m *Manager) SetOpponentDeck(ctx context.Context, id, oppDeck, newID string) (*ImportResult, error) {
	p, err := m.path(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(strings.TrimSuffix(p, ".jsonl") + ".import.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("game %s has no saved import to rebuild; import the log again with opp_deck", id)
	}
	if err != nil {
		return nil, err
	}
	var req ImportRequest
	if err := json.Unmarshal(b, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(oppDeck) == "" {
		return nil, errors.New("no opponent list given")
	}
	req.OppDeck = oppDeck
	if newID != "" && newID != id {
		req.ID = newID
		return m.Import(ctx, req)
	}

	// Replace in place: the old game's branches would not survive.
	g, err := m.Load(id)
	if err != nil {
		return nil, err
	}
	var extra []string
	for _, br := range g.Branches() {
		if br.Name != "main" {
			extra = append(extra, br.Name)
		}
	}
	if len(extra) > 0 {
		return nil, fmt.Errorf("game %s has branches %s that a rebuild would lose; pass new_id to build a separate game", id, strings.Join(extra, ", "))
	}
	m.mu.Lock()
	if m.current == g {
		m.current = nil
	}
	m.mu.Unlock()
	g.Close()
	old := p + ".old"
	if err := os.Rename(p, old); err != nil {
		return nil, err
	}
	res, err := m.Import(ctx, req)
	if err != nil {
		os.Rename(old, p) // keep the game we had
		return nil, err
	}
	os.Remove(old)
	return res, nil
}
