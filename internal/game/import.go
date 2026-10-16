package game

import (
	"context"
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

// ImportRequest describes a TCG Live battle log to import.
type ImportRequest struct {
	ID string
	// Log is the battle log text as copied from TCG Live.
	Log string
	// Deck is the log owner's decklist in TCG Live export format. If
	// empty, the deck from the previous import is used.
	Deck string
	// OppDeck optionally assumes the opponent's list for unrevealed cards.
	OppDeck string
	// Me is the log owner's player name; detected if empty.
	Me string
	// Seed makes the reconstruction reproducible; 0 picks one at random.
	Seed uint64
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
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		l, err := deck.Parse(text)
		if err != nil {
			return nil, fmt.Errorf("%s decklist: %w", who, err)
		}
		if m.DB != nil {
			if err := l.Resolve(ctx, m.DB); err != nil {
				for _, e := range strings.Split(err.Error(), "\n") {
					warnings = append(warnings, fmt.Sprintf("%s: %s", who, e))
				}
			}
		}
		return l.Entries, nil
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
	res, err := importer.Import(req.Log, importer.Options{Me: req.Me, MyDeck: mine, OppDeck: theirs, Seed: req.Seed})
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
	warnings = append(warnings, res.Report.Warnings...)
	warnings = append(warnings, g.st.Check()...)
	m.setCurrent(g)
	return &ImportResult{Game: g, Report: res.Report, Warnings: warnings}, nil
}
