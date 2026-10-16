package game

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/store"
)

// fakeDB serves a tiny card pool with set code TST.
type fakeDB map[string]*cards.Card

func newFakeDB() fakeDB {
	return fakeDB{
		"tst-1":   {ID: "tst-1", Name: "Riolu", Category: "Pokemon", Stage: "Basic", HP: 70, Types: []string{"Fighting"}, Retreat: 1, Attacks: []cards.Attack{{Name: "Punch", Damage: "10"}}},
		"tst-2":   {ID: "tst-2", Name: "Lucario", Category: "Pokemon", Stage: "Stage1", EvolveFrom: "Riolu", HP: 130, Types: []string{"Fighting"}, Retreat: 2, Attacks: []cards.Attack{{Name: "Aura Sphere", Damage: "130"}}},
		"tst-3":   {ID: "tst-3", Name: "Ultra Ball", Category: "Trainer", TrainerType: "Item"},
		"tst-4":   {ID: "tst-4", Name: "Iono", Category: "Trainer", TrainerType: "Supporter"},
		"tst-5":   {ID: "tst-5", Name: "Arena", Category: "Trainer", TrainerType: "Stadium"},
		"tst-6":   {ID: "tst-6", Name: "Band", Category: "Trainer", TrainerType: "Tool"},
		"sve-006": {ID: "sve-006", Name: "Fighting Energy", Category: "Energy", EnergyType: "Normal"},
	}
}

func (f fakeDB) Card(_ context.Context, id string) (*cards.Card, error) {
	if c, ok := f[id]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("%s: %w", id, cards.ErrNotFound)
}
func (f fakeDB) Cached(id string) *cards.Card { return f[id] }
func (f fakeDB) ByName(_ context.Context, name string) (string, error) {
	for id, c := range f {
		if c.Name == name {
			return id, nil
		}
	}
	return "", cards.ErrNotFound
}
func (f fakeDB) Resolve(_ context.Context, set, num string) (string, error) {
	if set != "TST" {
		return "", cards.ErrNotFound
	}
	return "tst-" + num, nil
}

const testDeck = `Pokémon: 8
4 Riolu TST 1
4 Lucario TST 2

Trainer: 12
4 Ultra Ball TST 3
4 Iono TST 4
2 Arena TST 5
2 Band TST 6

Energy: 40
40 Basic {F} Energy SVE 6

Total Cards: 60`

func newTestGame(t *testing.T) (*Manager, *Game) {
	t.Helper()
	m := NewManager(t.TempDir(), newFakeDB())
	seed := uint64(1)
	m.Rand = func() *rand.Rand { seed++; return rand.New(rand.NewPCG(seed, 99)) }
	g, res, err := m.New(context.Background(), NewGameRequest{ID: "t1", DeckP1: testDeck, DeckP2: testDeck})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) > 0 {
		t.Fatalf("setup warnings: %v", res.Warnings)
	}
	return m, g
}

func must(t *testing.T) func(*Result, error) *Result {
	return func(r *Result, err error) *Result {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
}

func snapshot(t *testing.T, g *Game) string {
	t.Helper()
	b, err := json.Marshal(g.State())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFullGameRewindAndReload(t *testing.T) {
	m, g := newTestGame(t)
	st := g.State()
	for _, seat := range state.Seats {
		if n := len(st.Sides[seat].Hand); n != 7 {
			t.Fatalf("%s hand %d", seat, n)
		}
	}
	if p := st.Check(); len(p) > 0 {
		t.Fatal(p)
	}
	snaps := map[int]string{g.Head(): snapshot(t, g)}
	step := func(r *Result, err error) *Result {
		t.Helper()
		r = must(t)(r, err)
		snaps[g.Head()] = snapshot(t, g)
		return r
	}

	// Deterministic openings: pull Riolus into hand during setup.
	hu := state.ActorHuman
	cl := state.ActorClaude
	for _, iid := range []state.IID{"p1-1", "p1-2", "p1-5"} {
		if l, _ := g.State().Find(iid); l.Zone != state.Hand {
			step(g.Move(state.P1, hu, iid, state.Loc{Zone: state.Hand}, "", ""))
		}
	}
	for _, iid := range []state.IID{"p2-1"} {
		if l, _ := g.State().Find(iid); l.Zone != state.Hand {
			step(g.Move(state.P2, cl, iid, state.Loc{Zone: state.Hand}, "", ""))
		}
	}
	step(g.PlaceActive(state.P1, hu, "p1-1"))
	step(g.Bench(state.P1, hu, "p1-2", -1))
	step(g.PlaceActive(state.P2, cl, "p2-1"))
	if _, err := g.PlaceActive(state.P2, cl, "p1-3"); err == nil {
		t.Fatal("placing p1's card for p2 should fail")
	}
	step(g.StartGame(state.P1, hu))
	st = g.State()
	if st.Turn != 1 || st.Current != state.P1 || st.Sides[state.P1].PrizesLeft() != 6 || st.Sides[state.P2].PrizesLeft() != 6 {
		t.Fatalf("bad start: turn %d current %s", st.Turn, st.Current)
	}
	if p := st.Check(); len(p) > 0 {
		t.Fatal(p)
	}

	// Turn lock.
	if _, err := g.Draw(state.P2, cl, 1); err == nil || !strings.Contains(err.Error(), "p1's turn") {
		t.Fatalf("expected turn lock, got %v", err)
	}

	// Two energy attachments warn but succeed.
	energy := func(seat state.Seat) []state.IID {
		var out []state.IID
		s := g.State()
		for _, iid := range s.Sides[seat].Deck {
			if s.Cards[iid].Def == "sve-006" {
				out = append(out, iid)
			}
		}
		return out
	}
	e := energy(state.P1)
	step(g.Move(state.P1, hu, e[0], state.Loc{Zone: state.Hand}, "", ""))
	step(g.Move(state.P1, hu, e[1], state.Loc{Zone: state.Hand}, "", ""))
	step(g.Attach(state.P1, hu, e[0], state.Loc{Seat: state.P1, Zone: state.Active}, ""))
	r := step(g.Attach(state.P1, hu, e[1], state.Loc{Seat: state.P1, Zone: state.Active}, ""))
	if !slices.ContainsFunc(r.Warnings, func(w string) bool { return strings.Contains(w, "2 Energy") }) {
		t.Fatalf("expected energy warning, got %v", r.Warnings)
	}

	// Ultra Ball as a compound action.
	if l, _ := g.State().Find("p1-9"); l.Zone != state.Hand {
		step(g.Move(state.P1, hu, "p1-9", state.Loc{Zone: state.Hand}, "", ""))
	}
	g.EndAction()
	r = step(g.PlayTrainer(state.P1, hu, "p1-9"))
	aid := r.ActionID
	hand := g.State().Sides[state.P1].Hand
	var discards []state.IID
	for _, iid := range hand {
		if len(discards) < 2 && iid != "p1-9" {
			discards = append(discards, iid)
		}
	}
	for _, iid := range discards {
		r = step(g.Move(state.P1, hu, iid, state.Loc{Zone: state.Discard}, "", ""))
		if r.ActionID != aid {
			t.Fatalf("move did not join %s: %s", aid, r.ActionID)
		}
	}
	found, err := g.SearchDeck(state.P1, "basic", "test", "test")
	if err != nil || len(found) == 0 {
		t.Fatalf("search: %v %v", found, err)
	}
	for _, f := range found {
		if f.Name != "Riolu" {
			t.Fatalf("filter returned %s", f.Name)
		}
	}
	step(g.Move(state.P1, hu, found[0].IID, state.Loc{Zone: state.Hand}, "", ""))
	r = step(g.Bench(state.P1, hu, found[0].IID, -1))
	if r.ActionID != aid {
		t.Fatalf("bench did not join %s", aid)
	}
	step(g.Shuffle(state.P1, hu, state.P1))
	if g.EndAction() != aid {
		t.Fatal("expected open action")
	}
	midHead := g.Head()

	// Lucario evolution on turn 1 warns.
	if l, _ := g.State().Find("p1-5"); l.Zone == state.Hand {
		r = step(g.Evolve(state.P1, hu, "p1-5", state.Loc{Seat: state.P1, Zone: state.Active}))
		if len(r.Warnings) == 0 {
			t.Fatal("expected evolution warnings")
		}
	}

	step(g.EndTurn(state.P1, hu))
	st = g.State()
	if st.Current != state.P2 || st.Turn != 2 {
		t.Fatalf("turn did not pass: %s %d", st.Current, st.Turn)
	}

	// p2 attacks for lethal, knocks out, takes a prize; p1 promotes on p2's turn.
	step(g.Attack(state.P2, cl, "Punch", state.Loc{Seat: state.P1, Zone: state.Active}, 200))
	step(g.KnockOut(state.P2, cl, state.Loc{Seat: state.P1, Zone: state.Active}))
	step(g.TakePrize(state.P2, cl, -1))
	step(g.Switch(state.P1, hu, 0))
	st = g.State()
	if st.Sides[state.P2].PrizesLeft() != 5 || st.Sides[state.P1].Active.Empty() {
		t.Fatal("knockout/promote did not apply")
	}
	if p := st.Check(); len(p) > 0 {
		t.Fatal(p)
	}
	finalHead := g.Head()
	final := snapshot(t, g)

	// Rewind to every recorded head reproduces that state exactly.
	for head, want := range snaps {
		if head == 0 {
			continue
		}
		if _, err := g.Rewind(head, ""); err != nil {
			t.Fatal(err)
		}
		if got := snapshot(t, g); got != want {
			t.Fatalf("rewind to %d differs", head)
		}
	}

	// A branch from mid-turn diverges without touching main.
	name, err := g.Rewind(midHead, "fix-1")
	if err != nil || name != "fix-1" {
		t.Fatal(err)
	}
	step(g.EndTurn(state.P1, hu))
	if err := g.Checkout(store.MainBranch); err != nil {
		t.Fatal(err)
	}
	if g.Head() != finalHead || snapshot(t, g) != final {
		t.Fatal("main changed after branching")
	}

	// Reload from disk.
	g2, err := m.Load("t1")
	if err != nil {
		t.Fatal(err)
	}
	if g2.Head() != finalHead || snapshot(t, g2) != final {
		t.Fatal("reloaded state differs")
	}
	if len(g2.Branches()) < 3 {
		t.Fatalf("branches lost: %v", g2.Branches())
	}
}

func TestViewsRedactHiddenInformation(t *testing.T) {
	_, g := newTestGame(t)
	v, err := g.View(state.P2, "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(v.Sides[state.P1].Hand); n != 0 {
		t.Fatalf("p2 sees %d of p1's hand", n)
	}
	if n := len(v.Sides[state.P2].Hand); n != 7 {
		t.Fatalf("p2 sees %d of own hand", n)
	}
	if _, err := g.View("", "test", "test"); err == nil {
		t.Fatal("reveal view allowed in play mode")
	}

	// A card that was public stays known in hand; one drawn does not.
	hand := g.State().Sides[state.P1].Hand
	must(t)(g.Move(state.P1, state.ActorHuman, hand[0], state.Loc{Zone: state.Discard}, "", ""))
	must(t)(g.Move(state.P1, state.ActorHuman, hand[0], state.Loc{Zone: state.Hand}, "", ""))
	v, _ = g.View(state.P2, "test", "test")
	if len(v.Sides[state.P1].Hand) != 1 || v.Sides[state.P1].Hand[0].IID != hand[0] {
		t.Fatalf("expected one known card in p1's hand: %+v", v.Sides[state.P1].Hand)
	}

	// The log redacts p1's draws for p2.
	lines, err := g.Log(0, state.P2, "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	var hidden, shown int
	for _, l := range lines {
		if strings.HasPrefix(l.Text, "p1 drew") {
			if strings.Contains(l.Text, "(hidden)") {
				hidden++
			} else {
				shown++
			}
		}
	}
	if hidden != 7 || shown != 0 {
		t.Fatalf("p1 draws: %d hidden, %d shown", hidden, shown)
	}

	g.SetMode(ModeReview)
	v, err = g.View("", "test", "test")
	if err != nil || len(v.Sides[state.P1].Hand) != 7 || len(v.Sides[state.P1].Deck) != 53 {
		t.Fatalf("reveal view: %v", err)
	}
}

func TestFailedVerbChangesNothing(t *testing.T) {
	_, g := newTestGame(t)
	before := snapshot(t, g)
	head := g.Head()
	// Knocking out an empty slot fails part way through validation.
	if _, err := g.KnockOut(state.P1, state.ActorHuman, state.Loc{Seat: state.P1, Zone: state.Active}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := g.Bench(state.P1, state.ActorHuman, "p1-60", 0); err == nil {
		t.Fatal("benching a card not in hand should fail")
	}
	if g.Head() != head || snapshot(t, g) != before {
		t.Fatal("state changed after failed verbs")
	}
}

func TestStadiumReplacesAndEffectsExpire(t *testing.T) {
	_, g := newTestGame(t)
	hu, cl := state.ActorHuman, state.ActorClaude
	for _, iid := range []state.IID{"p1-1", "p1-17"} {
		if l, _ := g.State().Find(iid); l.Zone != state.Hand {
			must(t)(g.Move(state.P1, hu, iid, state.Loc{Zone: state.Hand}, "", ""))
		}
	}
	for _, iid := range []state.IID{"p2-1", "p2-18"} {
		if l, _ := g.State().Find(iid); l.Zone != state.Hand {
			must(t)(g.Move(state.P2, cl, iid, state.Loc{Zone: state.Hand}, "", ""))
		}
	}
	must(t)(g.PlaceActive(state.P1, hu, "p1-1"))
	must(t)(g.PlaceActive(state.P2, cl, "p2-1"))
	must(t)(g.StartGame(state.P1, hu))
	must(t)(g.PlayTrainer(state.P1, hu, "p1-17"))
	must(t)(g.AddEffect(state.P1, hu, "Items locked", state.P2, "p1-1", 2))
	must(t)(g.EndTurn(state.P1, hu))
	if n := len(g.State().Effects); n != 1 {
		t.Fatalf("effect expired early: %d", n)
	}
	must(t)(g.PlayTrainer(state.P2, cl, "p2-18"))
	st := g.State()
	if st.Stadium != "p2-18" || !slices.Contains(st.Sides[state.P1].Discard, "p1-17") {
		t.Fatal("stadium not replaced into owner's discard")
	}
	must(t)(g.EndTurn(state.P2, cl))
	if n := len(g.State().Effects); n != 0 {
		t.Fatalf("effect did not expire: %d", n)
	}
}

func TestToolHP(t *testing.T) {
	basic := &cards.Card{Name: "Riolu", Category: "Pokemon", Stage: "Basic", HP: 70}
	stage1 := &cards.Card{Name: "N's Zoroark ex", Category: "Pokemon", Stage: "Stage1", HP: 280}
	for _, tc := range []struct {
		effect string
		p      *cards.Card
		want   int
	}{
		{"The Pokémon this card is attached to gets +100 HP.", stage1, 100}, // Hero's Cape
		{"The Basic Pokémon this card is attached to gets +50 HP.", basic, 50},
		{"The Basic Pokémon this card is attached to gets +50 HP.", stage1, 0},
		{"The Pokémon ex this card is attached to gets +60 HP.", stage1, 60},
		{"The Pokémon ex this card is attached to gets +60 HP.", basic, 0},
		{"The {G} Pokémon this card is attached to gets +30 HP.", basic, 0}, // condition not understood
		{"Attacks used by the Pokémon this card is attached to do 50 more damage to your opponent's Active Pokémon ex.", stage1, 0},
	} {
		if got := toolHP(tc.effect, tc.p); got != tc.want {
			t.Errorf("toolHP(%q, %s) = %d, want %d", tc.effect, tc.p.Name, got, tc.want)
		}
	}
}
