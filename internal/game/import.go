package game

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/importer"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/store"
)

// ImportRequest describes a TCG Live battle log to import. Log may instead
// hold a game record's text, which supplies the deck, opponent list, name
// and seed itself.
type ImportRequest struct {
	ID string `json:"id"`
	// Log is the battle log text as copied from TCG Live, or a game
	// record (see Record) to restore.
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
	// Record is the game record of the game built: everything needed to
	// rebuild it exactly by importing the record.
	Record Record
}

var nonID = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// errRecordExtras refuses a record imported together with the values it
// already holds.
var errRecordExtras = errors.New("a game record already holds both decks, the player's name and the seed: import it on its own, then use set_opponent_deck to change the opponent's list")

// Import replays a battle log into a new in-memory game, in review mode, and
// makes it current, replacing any game open. The owner sits at p1 and the opponent at p2. When
// req.Log is a game record, the game it describes is rebuilt exactly.
func (m *Manager) Import(ctx context.Context, req ImportRequest) (*ImportResult, error) {
	rec, isRec, err := ParseRecord(req.Log)
	if err != nil {
		return nil, err
	}
	if isRec {
		if strings.TrimSpace(req.Deck) != "" || strings.TrimSpace(req.OppDeck) != "" || strings.TrimSpace(req.Me) != "" || req.Seed != 0 {
			return nil, errRecordExtras
		}
		req.Log, req.Deck, req.OppDeck, req.Me, req.Seed = rec.Log, rec.Deck, rec.OppDeck, rec.Me, rec.Seed
	}
	var warnings []string
	if strings.TrimSpace(req.Deck) == "" {
		m.mu.Lock()
		req.Deck = m.lastDeck
		m.mu.Unlock()
		if strings.TrimSpace(req.Deck) == "" {
			return nil, fmt.Errorf("no decklist given and no previous import to reuse one from")
		}
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
	l := store.NewMemory(store.Header{Game: req.ID, Created: time.Now().UTC(), Source: "import"})
	for _, s := range res.Steps {
		if len(s.Events) == 0 {
			continue
		}
		a := store.Action{ID: l.NextActionID(), Label: s.Label, Kind: s.Kind, Seat: s.Seat, Actor: state.ActorImport}
		if err := l.PutAction(a); err != nil {
			return nil, err
		}
		for i := range s.Events {
			s.Events[i].Action = a.ID
		}
		if err := l.Append(s.Events); err != nil {
			return nil, err
		}
	}
	g, err := Load(req.ID, l, m.DB, m.rng())
	if err != nil {
		return nil, fmt.Errorf("imported events do not fold: %w", err)
	}
	sealed := Record{Me: res.Report.Me, Seed: res.Seed, Deck: req.Deck, OppDeck: req.OppDeck, Log: req.Log}.Seal()
	g.record = &sealed
	warnings = append(warnings, res.Report.Warnings...)
	warnings = append(warnings, g.st.Check()...)
	m.mu.Lock()
	m.lastDeck = req.Deck
	m.mu.Unlock()
	m.setCurrent(g)
	return &ImportResult{Game: g, Report: res.Report, Warnings: warnings, Record: sealed}, nil
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

// SetOpponentDeck rebuilds the current imported game from its record with
// an assumed opponent list: the same log and seed, so the owner's side and
// everything the log shows come out the same, and every card the log never
// names is dealt at random from what the list has left. The game is
// replaced, so it refuses when branches other than main exist unless
// discardBranches is set. On failure the current game stays as it was.
func (m *Manager) SetOpponentDeck(ctx context.Context, oppDeck string, discardBranches bool) (*ImportResult, error) {
	g, err := m.Current()
	if err != nil {
		return nil, err
	}
	rec, ok := g.Record()
	if !ok {
		return nil, fmt.Errorf("game %s was not imported from a battle log, so it cannot be rebuilt with an opponent list", g.ID)
	}
	if strings.TrimSpace(oppDeck) == "" {
		return nil, errors.New("no opponent list given")
	}
	if !discardBranches {
		var extra []string
		for _, br := range g.Branches() {
			if br.Name != store.MainBranch {
				extra = append(extra, br.Name)
			}
		}
		if len(extra) > 0 {
			return nil, fmt.Errorf("game %s has branches %s that a rebuild would lose; ask the player, then pass discard_branches to rebuild anyway", g.ID, strings.Join(extra, ", "))
		}
	}
	return m.Import(ctx, ImportRequest{ID: g.ID, Log: rec.Log, Deck: rec.Deck, OppDeck: oppDeck, Me: rec.Me, Seed: rec.Seed})
}
