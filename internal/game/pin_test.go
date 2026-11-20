package game

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// swapSource is a card source whose current catalogue a test replaces, as
// the catalogue loader does when it picks up a publish.
type swapSource struct {
	mu    sync.Mutex
	cur   cards.DB
	calls int
	// after, when set, replaces cur with it once the first snapshot has
	// been taken: a publish picked up in the middle of an operation.
	after cards.DB
}

func (s *swapSource) Snapshot() cards.DB {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	db := s.cur
	if s.after != nil {
		s.cur, s.after = s.after, nil
	}
	return db
}

func (s *swapSource) swap(db cards.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = db
}

// twoVersions is the fake pool in an old and a new version: Lucario
// (tst-2) has a different HP and Arena (tst-5), a Stadium, lets each
// player bench 8 in the old version only.
func twoVersions() (old, updated cardstest.FakeDB) {
	old, updated = cardstest.New(), cardstest.New()
	old["tst-5"].Effect = "Each player can have 8 Pokémon on their Bench."
	updated["tst-5"].Effect = "Draw a card."
	updated["tst-2"].HP = 150
	return old, updated
}

// TestOpenGameKeepsItsCatalogue: a game started before a newer catalogue
// arrives keeps its card text, HP and the warnings its cards decide, and
// card lookups through the manager read its snapshot; a game started
// afterwards uses the new one; with no game open the manager gives the
// current catalogue.
func TestOpenGameKeepsItsCatalogue(t *testing.T) {
	old, updated := twoVersions()
	src := &swapSource{cur: old}
	m := NewManager(t.TempDir(), src)
	if db := m.CardDB(); db == nil || db.Cached("tst-5").Effect != old["tst-5"].Effect {
		t.Fatalf("no game open: CardDB is not the current catalogue")
	}
	seed := uint64(1)
	m.Rand = func() *rand.Rand { seed++; return rand.New(rand.NewPCG(seed, 99)) }
	g, _, err := m.New(context.Background(), NewGameRequest{ID: "before", DeckP1: testDeck, DeckP2: testDeck})
	if err != nil {
		t.Fatal(err)
	}
	src.swap(updated)

	// Card text and the game's own lookups keep the old version.
	if c, _ := m.CardDB().Card(context.Background(), "tst-5"); c == nil || c.Effect != old["tst-5"].Effect {
		t.Errorf("CardDB with the old game open: %+v", c)
	}
	if g.DB().Cached("tst-2").HP != 130 {
		t.Errorf("game's Lucario HP = %d, want the old 130", g.DB().Cached("tst-2").HP)
	}
	if lucario := iidOf(t, g, state.P1, "tst-2"); g.hp(g.State().Cards[lucario].Def, nil) != 130 {
		t.Errorf("hp() reads the new catalogue")
	}

	// Warnings follow the old Stadium text: with the old Arena in play a
	// sixth Benched Pokémon is allowed.
	hu, cl := state.ActorHuman, state.ActorClaude
	var pokemon []state.IID
	var arena state.IID
	for iid, c := range g.State().Cards {
		switch {
		case c.Owner == state.P1 && (c.Def == "tst-1" || c.Def == "tst-2") && len(pokemon) < 7:
			pokemon = append(pokemon, iid)
		case c.Owner == state.P1 && c.Def == "tst-5" && arena == "":
			arena = iid
		}
	}
	slices.Sort(pokemon)
	for _, iid := range append(slices.Clone(pokemon), arena) {
		if l, _ := g.State().Find(iid); l.Zone != state.Hand {
			must(t)(g.Move(state.P1, hu, iid, state.Loc{Zone: state.Hand}, "", ""))
		}
	}
	var p2 state.IID
	for iid, c := range g.State().Cards {
		if c.Owner == state.P2 && c.Def == "tst-1" {
			p2 = iid
			break
		}
	}
	if l, _ := g.State().Find(p2); l.Zone != state.Hand {
		must(t)(g.Move(state.P2, cl, p2, state.Loc{Zone: state.Hand}, "", ""))
	}
	must(t)(g.PlaceActive(state.P1, hu, pokemon[0]))
	must(t)(g.PlaceActive(state.P2, cl, p2))
	must(t)(g.StartGame(state.P1, hu))
	must(t)(g.PlayTrainer(state.P1, hu, arena))
	g.EndAction()
	var last *Result
	for _, iid := range pokemon[1:] {
		last = must(t)(g.Bench(state.P1, hu, iid, -1))
	}
	if slices.ContainsFunc(last.Warnings, func(w string) bool { return strings.Contains(w, "on the Bench, more than") }) {
		t.Errorf("sixth Benched Pokémon warned about under the old Arena: %v", last.Warnings)
	}

	// A game started after the swap uses the new catalogue.
	g2, _, err := m.New(context.Background(), NewGameRequest{ID: "after", DeckP1: testDeck, DeckP2: testDeck})
	if err != nil {
		t.Fatal(err)
	}
	if g2.DB().Cached("tst-2").HP != 150 {
		t.Errorf("new game's Lucario HP = %d, want 150", g2.DB().Cached("tst-2").HP)
	}
	if c, _ := m.CardDB().Card(context.Background(), "tst-5"); c == nil || c.Effect != "Draw a card." {
		t.Errorf("CardDB with the new game open: %+v", c)
	}
	// The old game, still held, kept its own.
	if g.DB().Cached("tst-2").HP != 130 {
		t.Error("the old game's catalogue changed")
	}
}

// anyDB answers every lookup in one catalogue version: each id it gives
// is prefixed with the version, so a game's defs show which version
// resolved them.
type anyDB struct{ ver string }

func (d anyDB) card(id string) *cards.Card {
	name := id
	if _, n, ok := strings.Cut(id, "|"); ok {
		name = n
	}
	return &cards.Card{ID: id, Name: name, Category: "Pokemon", HP: 100, Effect: d.ver}
}
func (d anyDB) Card(_ context.Context, id string) (*cards.Card, error) {
	if !strings.HasPrefix(id, d.ver+"|") {
		return nil, fmt.Errorf("%s: %w", id, cards.ErrNotFound)
	}
	return d.card(id), nil
}
func (d anyDB) Cached(id string) *cards.Card {
	c, _ := d.Card(context.Background(), id)
	return c
}
func (d anyDB) Resolve(_ context.Context, set, num string) (string, error) {
	return d.ver + "|" + set + "-" + num, nil
}
func (d anyDB) ByName(_ context.Context, name string, _ ...string) (string, error) {
	return d.ver + "|" + name, nil
}

// versionsOf lists the catalogue versions a game's defs came from (Basic
// Energy defs, fixed by the importer, aside).
func versionsOf(g *Game) []string {
	seen := map[string]bool{}
	for _, c := range g.State().Cards {
		if ver, _, ok := strings.Cut(c.Def, "|"); ok {
			seen[ver] = true
		}
	}
	vs := make([]string, 0, len(seen))
	for v := range seen {
		vs = append(vs, v)
	}
	slices.Sort(vs)
	return vs
}

// TestImportResolvesAgainstOneCatalogue: an import takes one snapshot and
// resolves the decklists and the log against it alone, even when a newer
// catalogue arrives during it, and the game keeps it; reopening the game
// from its record after the swap uses the new catalogue.
func TestImportResolvesAgainstOneCatalogue(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	src := &swapSource{cur: anyDB{"v1"}, after: anyDB{"v2"}}
	m := NewManager(t.TempDir(), src)
	res, err := m.Import(context.Background(), ImportRequest{ID: "g", Log: logText, Deck: deckText, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if src.calls != 1 {
		t.Errorf("import took %d snapshots, want 1", src.calls)
	}
	if got := versionsOf(res.Game); !slices.Equal(got, []string{"v1"}) {
		t.Fatalf("imported game's defs come from %v, want only v1", got)
	}
	if res.Game.DB().(anyDB).ver != "v1" {
		t.Errorf("imported game pinned %v", res.Game.DB())
	}

	// The source has moved on to v2: reopening from the record uses it.
	again, err := m.Import(context.Background(), ImportRequest{Log: res.Record.JSON()})
	if err != nil {
		t.Fatal(err)
	}
	if got := versionsOf(again.Game); !slices.Equal(got, []string{"v2"}) {
		t.Errorf("reopened game's defs come from %v, want only v2", got)
	}
	if got := versionsOf(res.Game); !slices.Equal(got, []string{"v1"}) {
		t.Errorf("the first game's defs changed to %v", got)
	}
}

// TestOpponentDeckRebuildTakesTheCurrentCatalogue: rebuilding an imported
// game with an opponent list is a reopen, so it takes the current
// catalogue.
func TestOpponentDeckRebuildTakesTheCurrentCatalogue(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	src := &swapSource{cur: anyDB{"v1"}}
	m := NewManager(t.TempDir(), src)
	if _, err := m.Import(context.Background(), ImportRequest{ID: "g", Log: logText, Deck: deckText, Seed: 1}); err != nil {
		t.Fatal(err)
	}
	src.swap(anyDB{"v2"})
	res, err := m.SetOpponentDeck(context.Background(), deckText, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := versionsOf(res.Game); !slices.Equal(got, []string{"v2"}) {
		t.Errorf("rebuilt game's defs come from %v, want only v2", got)
	}
}

// TestNoSourceNoCatalogue: a manager without a card source resolves
// nothing and has no card database, as before.
func TestNoSourceNoCatalogue(t *testing.T) {
	m := NewManager(t.TempDir(), nil)
	if m.CardDB() != nil {
		t.Error("CardDB without a source is not nil")
	}
	g, _, err := m.New(context.Background(), NewGameRequest{ID: "none", DeckP1: testDeck, DeckP2: testDeck})
	if err != nil {
		t.Fatal(err)
	}
	if g.DB() != nil || m.CardDB() != nil {
		t.Error("a game without a source has a card database")
	}
}
