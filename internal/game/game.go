// Package game turns player verbs into primitive events, groups them into
// actions, persists them and keeps the folded state current. It checks
// locations, not rules: rule problems come back as warnings.
package game

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/store"
)

// Mode is play (hidden information respected) or review (reveal allowed,
// either seat may act).
type Mode string

const (
	ModePlay   Mode = "play"
	ModeReview Mode = "review"
)

// Result is returned by every write.
type Result struct {
	ActionID string   `json:"action_id,omitempty"`
	Events   []int    `json:"events"`
	Delta    []string `json:"delta"`
	Warnings []string `json:"warnings,omitempty"`
	Data     any      `json:"data,omitempty"`
}

// Update is sent to listeners after every change.
type Update struct {
	Head   int      `json:"head"`
	Branch string   `json:"branch"`
	Events []int    `json:"events"`
	Delta  []string `json:"delta"`
}

// Game is one game: its event log and current state.
type Game struct {
	mu   sync.Mutex
	ID   string
	log  *store.Log
	st   *state.State
	db   cards.DB
	rng  *rand.Rand
	mode Mode
	// open is the compound action following verbs join, if any.
	open *store.Action
	// openText is the effect text of the open action's card (nil if
	// unknown), which decides whether verbs with joinIf join it.
	openText  *string
	listeners []func(Update)
	// record is the game record it was imported from; nil for a game
	// started with new_game.
	record *Record
}

// Options configure a new game.
type Options struct {
	BenchSize  int
	PrizeCount int
	Names      map[state.Seat]string
	Rand       *rand.Rand
	Source     string
}

func newRand() *rand.Rand {
	var seed [32]byte
	crand.Read(seed[:])
	return rand.New(rand.NewChaCha8(seed))
}

// Load wraps an existing log, folding it to the current head.
func Load(id string, log *store.Log, db cards.DB, rng *rand.Rand) (*Game, error) {
	if rng == nil {
		rng = newRand()
	}
	g := &Game{ID: id, log: log, db: db, rng: rng, mode: ModePlay}
	if log.Header.Source == "import" {
		g.mode = ModeReview
	}
	if err := g.refold(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *Game) refold() error {
	path, err := g.log.PathTo(g.log.Head())
	if err != nil {
		return err
	}
	st, err := state.Fold(path)
	if err != nil {
		return err
	}
	g.st = st
	return nil
}

// Record returns a copy of the game record the game was imported from,
// and false for a game started from two decklists, which has none.
func (g *Game) Record() (Record, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.record == nil {
		return Record{}, false
	}
	return *g.record, true
}

// Subscribe registers a listener for updates.
func (g *Game) Subscribe(f func(Update)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.listeners = append(g.listeners, f)
}

// Mode returns the current mode.
func (g *Game) Mode() Mode {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.mode
}

// SetMode switches between play and review.
func (g *Game) SetMode(m Mode) error {
	if m != ModePlay && m != ModeReview {
		return fmt.Errorf("unknown mode %q", m)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mode = m
	return nil
}

// State returns a copy of the current state.
func (g *Game) State() *state.State {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.st.Clone()
}

// Branch returns the current branch name.
func (g *Game) Branch() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.log.Current()
}

// tx accumulates events against a scratch copy of the state.
type tx struct {
	g      *Game
	st     *state.State
	actor  state.Actor
	seat   state.Seat
	action string
	events []state.Event
	data   any
	warn   []string
	// effect is an opener's effect text, when its card is known.
	effect *string
}

func (t *tx) emit(seat state.Seat, typ state.EventType, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	parent := t.g.log.Head()
	if n := len(t.events); n > 0 {
		parent = t.events[n-1].ID
	}
	e := state.Event{
		ID:      t.g.log.NextID() + len(t.events),
		Parent:  parent,
		Action:  t.action,
		Actor:   t.actor,
		Seat:    seat,
		Type:    typ,
		Payload: b,
	}
	if err := t.st.Apply(&e); err != nil {
		return err
	}
	e.Turn = t.st.Turn
	t.events = append(t.events, e)
	return nil
}

func (t *tx) move(iid state.IID, to state.Loc, as, face string) error {
	from, err := t.st.Find(iid)
	if err != nil {
		return err
	}
	return t.emit(t.seat, state.EvMove, state.MovePayload{IID: iid, From: &from, To: to, As: as, Face: face})
}

func (t *tx) draw(seat state.Seat, n int) error {
	if _, ok := t.st.Sides[seat]; !ok {
		return fmt.Errorf("unknown seat %q", seat)
	}
	for i := 0; i < n; i++ {
		deck := t.st.Sides[seat].Deck
		if len(deck) == 0 {
			t.warn = append(t.warn, fmt.Sprintf("%s's deck is empty; drew %d of %d", seat, i, n))
			return nil
		}
		if err := t.emit(seat, state.EvMove, state.MovePayload{
			IID: deck[0], From: &state.Loc{Seat: seat, Zone: state.Deck, Pos: new(state.Top)}, To: state.Loc{Seat: seat, Zone: state.Hand},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (t *tx) shuffle(seat state.Seat) error {
	order := slices.Clone(t.st.Sides[seat].Deck)
	t.g.rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	return t.emit(seat, state.EvShuffle, state.ShufflePayload{Seat: seat, Order: order})
}

// actionSpec describes the action a verb creates or joins.
type actionSpec struct {
	label  string
	kind   string
	source state.IID
	// opener starts a compound action that later verbs join.
	opener bool
	// anyTurn skips the turn lock.
	anyTurn bool
	// system marks game-management events that never join a compound action.
	system bool
	// joinIf, if set, joins an open action only when its card's effect text
	// mentions this word: an Energy attached by hand after Lunar Cycle is the
	// turn's attachment, not part of the Ability.
	joinIf string
	// crossSeat joins an open action of the other seat, for effects on the
	// opponent's side: the switch of Boss's Orders, the draws of Iono.
	crossSeat bool
}

// run executes a verb atomically: either every event applies and is
// persisted, or nothing changes.
func (g *Game) run(seat state.Seat, actor state.Actor, spec actionSpec, fn func(*tx) error) (*Result, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	res, err := g.runLocked(seat, actor, spec, fn)
	if err != nil {
		return nil, err
	}
	g.notify(res)
	return res, nil
}

func (g *Game) runLocked(seat state.Seat, actor state.Actor, spec actionSpec, fn func(*tx) error) (*Result, error) {
	if seat != "" && !seat.Valid() {
		return nil, fmt.Errorf("unknown seat %q", seat)
	}
	joined := !spec.system && !spec.opener && g.open != nil &&
		(g.open.Seat == seat || spec.crossSeat) && g.joins(spec.joinIf)
	// The opponent's part of an effect (Iono's draws) happens on the turn of
	// the player whose action it joins.
	if !spec.anyTurn && !(joined && g.open.Seat != seat) {
		if err := g.checkTurn(seat); err != nil {
			return nil, err
		}
	}
	var action store.Action
	switch {
	case joined:
		action = *g.open
	case !spec.system:
		action = store.Action{
			ID: g.log.NextActionID(), Label: spec.label, Kind: spec.kind,
			Seat: seat, Actor: actor, Source: spec.source,
		}
	}
	t := &tx{g: g, st: g.st.Clone(), actor: actor, seat: seat, action: action.ID}
	if err := fn(t); err != nil {
		return nil, err
	}
	if spec.opener && !spec.system {
		g.open, g.openText = &action, t.effect
	} else if !joined && !spec.system && g.open != nil {
		// A standalone verb from either seat closes any open action.
		g.open = nil
	}
	res := &Result{Events: []int{}, Delta: []string{}, Data: t.data, Warnings: t.warn}
	if len(t.events) > 0 {
		if action.ID != "" && g.log.Action(action.ID) == nil {
			if err := g.log.PutAction(action); err != nil {
				return nil, err
			}
		}
		before := g.st
		if err := g.log.Append(t.events); err != nil {
			// The file and memory may now disagree; reload from disk truth.
			g.refold()
			return nil, err
		}
		g.st = t.st
		for _, e := range t.events {
			res.Events = append(res.Events, e.ID)
			res.Delta = append(res.Delta, describe(&e, before, g.st, ""))
		}
		res.ActionID = action.ID
	} else if spec.opener {
		res.ActionID = action.ID
	}
	res.Warnings = append(res.Warnings, g.turnWarnings()...)
	return res, nil
}

// joins reports whether a verb with joinIf word joins the open action. If
// the open action's text is unknown it joins, as every verb used to.
func (g *Game) joins(word string) bool {
	if word == "" || g.openText == nil {
		return true
	}
	return strings.Contains(strings.ToLower(*g.openText), word)
}

func (g *Game) notify(res *Result) {
	if len(res.Events) == 0 {
		return
	}
	u := Update{Head: g.log.Head(), Branch: g.log.Current(), Events: res.Events, Delta: res.Delta}
	for _, f := range g.listeners {
		f(u)
	}
}

// checkTurn is the turn lock: a seat's verbs only on its turn. Review mode
// is a sandbox for exploring lines, where either side may act at any time
// (soft rule warnings still apply).
func (g *Game) checkTurn(seat state.Seat) error {
	switch g.st.Phase {
	case state.PhaseEmpty:
		return errors.New("no game set up")
	case state.PhaseSetup:
		return nil
	}
	if g.mode == ModeReview {
		return nil
	}
	if !g.st.TurnOpen {
		return fmt.Errorf("no turn is in progress")
	}
	if g.st.Current != seat {
		return fmt.Errorf("it is %s's turn (turn %d), not %s's", g.st.Current, g.st.Turn, seat)
	}
	return nil
}

// card returns a card's definition if it can be found, else nil.
func (g *Game) card(st *state.State, iid state.IID) *cards.Card {
	c, ok := st.Cards[iid]
	if !ok || c.Def == "" || g.db == nil {
		return nil
	}
	if cc := g.db.Cached(c.Def); cc != nil {
		return cc
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cc, err := g.db.Card(ctx, c.Def)
	if err != nil {
		return nil
	}
	return cc
}

// hp is a Pokémon's maximum HP: printed HP plus what its Tools add.
func (g *Game) hp(def string, tools []string) int {
	if g.db == nil || def == "" {
		return 0
	}
	c := g.db.Cached(def)
	if c == nil || c.HP == 0 {
		return 0
	}
	hp := c.HP
	for _, t := range tools {
		if tc := g.db.Cached(t); tc != nil {
			hp += toolHP(tc.Effect, c)
		}
	}
	return hp
}

// toolHPRe matches Tool text like "The Pokémon this card is attached to
// gets +100 HP" (Hero's Cape), optionally naming which Pokémon.
var toolHPRe = regexp.MustCompile(`(?i)the ([\w ]*?)pok[eé]mon( ex)? this card is attached to gets \+(\d+) hp`)

// toolHP is the HP a Tool with effect text gives Pokémon p. A condition
// the code does not understand counts as unmet rather than guessed at.
func toolHP(effect string, p *cards.Card) int {
	m := toolHPRe.FindStringSubmatch(effect)
	if m == nil {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(m[1])) {
	case "":
	case "basic":
		if !p.IsBasic() {
			return 0
		}
	default:
		return 0
	}
	if m[2] != "" && !strings.HasSuffix(p.Name, " ex") {
		return 0
	}
	n, _ := strconv.Atoi(m[3])
	return n
}

// EndAction closes the open compound action.
func (g *Game) EndAction() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open == nil {
		return ""
	}
	id := g.open.ID
	g.open = nil
	return id
}

// OpenAction returns the open compound action id, if any.
func (g *Game) OpenAction() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open == nil {
		return ""
	}
	return g.open.ID
}
