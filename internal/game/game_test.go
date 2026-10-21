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
		"tst-1":   {ID: "tst-1", Name: "Riolu", Category: "Pokemon", Stage: "Basic", HP: 70, Types: []string{"Fighting"}, Retreat: 1, Attacks: []cards.Attack{{Name: "Punch", Damage: "10"}}, Abilities: []cards.Ability{{Name: "Focus", Effect: "Once during your turn, you may draw 2 cards."}}},
		"tst-2":   {ID: "tst-2", Name: "Lucario", Category: "Pokemon", Stage: "Stage1", EvolveFrom: "Riolu", HP: 130, Types: []string{"Fighting"}, Retreat: 2, Attacks: []cards.Attack{{Name: "Aura Sphere", Damage: "130"}}},
		"tst-3":   {ID: "tst-3", Name: "Ultra Ball", Category: "Trainer", TrainerType: "Item", Effect: "You can use this card only if you discard 2 other cards from your hand. Search your deck for a Pokémon, reveal it, and put it into your hand. Then, shuffle your deck."},
		"tst-4":   {ID: "tst-4", Name: "Iono", Category: "Trainer", TrainerType: "Supporter", Effect: "Each player shuffles their hand and puts it on the bottom of their deck. Then, each player draws a card for each of their remaining Prize cards."},
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
func (f fakeDB) ByName(_ context.Context, name string, _ ...string) (string, error) {
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

func TestFullGameRewindAndBranch(t *testing.T) {
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
	step(g.Shuffle(state.P1, hu, state.P1))
	if g.EndAction() != aid {
		t.Fatal("expected open action")
	}
	// Benching the Pokémon found is a play of its own.
	if r = step(g.Bench(state.P1, hu, found[0].IID, -1)); r.ActionID == aid {
		t.Fatalf("bench joined %s, whose card does not bench", aid)
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

	// The game stays current, in memory, with its branches.
	g2, err := m.Current()
	if err != nil {
		t.Fatal(err)
	}
	if g2 != g || g2.Head() != finalHead || snapshot(t, g2) != final {
		t.Fatal("current game differs")
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
	must(t)(g.AddEffect(state.P1, hu, "Items locked", state.P2, "p1-1", 1, state.P2))
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

// Abilities and attacks are recorded even when nothing else happens, and
// the moves that resolve an Ability join its action.
func TestAbilityAndAttackAreLogged(t *testing.T) {
	_, g := newTestGame(t)
	hu := state.ActorHuman
	must(t)(g.PlaceActive(state.P1, hu, g.State().Sides[state.P1].Hand[0]))
	must(t)(g.PlaceActive(state.P2, state.ActorClaude, g.State().Sides[state.P2].Hand[0]))
	must(t)(g.StartGame(state.P1, hu))
	active := state.Loc{Seat: state.P1, Zone: state.Active}
	r := must(t)(g.UseAbility(state.P1, hu, active, "Lunar Cycle"))
	if len(r.Events) != 1 {
		t.Fatalf("ability events %v", r.Events)
	}
	d := must(t)(g.Draw(state.P1, hu, 1))
	if d.ActionID != r.ActionID {
		t.Fatalf("draw did not join the ability: %s vs %s", d.ActionID, r.ActionID)
	}
	must(t)(g.EndTurn(state.P1, hu))
	must(t)(g.Attack(state.P2, state.ActorClaude, "Growl", state.Loc{Seat: state.P1, Zone: state.Active}, 0))
	lines, err := g.Log(0, "", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	var text []string
	for _, l := range lines {
		text = append(text, l.Text)
	}
	all := strings.Join(text, "\n")
	for _, want := range []string{"p1 used ability Lunar Cycle from p1:active (", "p2 used attack Growl from p2:active ("} {
		if !strings.Contains(all, want) {
			t.Errorf("log lacks %q:\n%s", want, all)
		}
	}
}

// Verbs join an open action only when its card's text calls for them: the
// turn's Energy attachment after an Ability stands alone, while the
// opponent's draws from Iono join it.
func TestJoiningFollowsCardText(t *testing.T) {
	_, g := newTestGame(t)
	hu := state.ActorHuman
	toHand := func(seat state.Seat, iid state.IID) {
		t.Helper()
		if l, _ := g.State().Find(iid); l.Zone != state.Hand {
			must(t)(g.Move(seat, hu, iid, state.Loc{Zone: state.Hand}, "", ""))
		}
	}
	toHand(state.P1, "p1-1")
	toHand(state.P2, "p2-1")
	must(t)(g.PlaceActive(state.P1, hu, "p1-1"))
	must(t)(g.PlaceActive(state.P2, hu, "p2-1"))
	must(t)(g.StartGame(state.P1, hu))
	find := func(def string) state.IID {
		s := g.State()
		for _, iid := range s.Sides[state.P1].Deck {
			if s.Cards[iid].Def == def {
				return iid
			}
		}
		t.Fatalf("no %s in deck", def)
		return ""
	}
	energy, iono := find("sve-006"), find("tst-4")
	toHand(state.P1, energy)
	toHand(state.P1, iono)
	g.EndAction()

	active := state.Loc{Seat: state.P1, Zone: state.Active}
	ab := must(t)(g.UseAbility(state.P1, hu, active, "Focus"))
	if d := must(t)(g.Draw(state.P1, hu, 2)); d.ActionID != ab.ActionID {
		t.Fatalf("the Ability's draw did not join it")
	}
	if a := must(t)(g.Attach(state.P1, hu, energy, active, "")); a.ActionID == ab.ActionID {
		t.Fatalf("the Energy attachment joined an Ability that does not attach")
	}
	if r := g.turnWarnings(); len(r) > 0 {
		t.Fatalf("unexpected turn warnings %v", r)
	}
	tr := must(t)(g.PlayTrainer(state.P1, hu, iono))
	if d := must(t)(g.Draw(state.P2, hu, 1)); d.ActionID != tr.ActionID {
		t.Fatalf("the opponent's Iono draw did not join it")
	}
}

// startedGame places an Active for each seat and starts the game with p1
// going first.
func startedGame(t *testing.T) *Game {
	t.Helper()
	_, g := newTestGame(t)
	hu, cl := state.ActorHuman, state.ActorClaude
	if l, _ := g.State().Find("p1-1"); l.Zone != state.Hand {
		must(t)(g.Move(state.P1, hu, "p1-1", state.Loc{Zone: state.Hand}, "", ""))
	}
	if l, _ := g.State().Find("p2-1"); l.Zone != state.Hand {
		must(t)(g.Move(state.P2, cl, "p2-1", state.Loc{Zone: state.Hand}, "", ""))
	}
	must(t)(g.PlaceActive(state.P1, hu, "p1-1"))
	must(t)(g.PlaceActive(state.P2, cl, "p2-1"))
	must(t)(g.StartGame(state.P1, hu))
	return g
}

// Moving an Active Pokémon to an empty Bench slot takes its Energy, Tools
// and damage with it, and clears its Special Conditions.
func TestMovePokemonToBenchTakesAttachments(t *testing.T) {
	g := startedGame(t)
	hu := state.ActorHuman
	fromDeck := func(def string) state.IID {
		t.Helper()
		s := g.State()
		for _, iid := range s.Sides[state.P1].Deck {
			if s.Cards[iid].Def == def {
				must(t)(g.Move(state.P1, hu, iid, state.Loc{Zone: state.Hand}, "", ""))
				return iid
			}
		}
		t.Fatalf("no %s in deck", def)
		return ""
	}
	energy, tool := fromDeck("sve-006"), fromDeck("tst-6")
	active := state.Loc{Seat: state.P1, Zone: state.Active}
	must(t)(g.Attach(state.P1, hu, energy, active, "energy"))
	must(t)(g.Attach(state.P1, hu, tool, active, "tool"))
	must(t)(g.PlaceDamage(state.P1, hu, active, 30))
	must(t)(g.SetCondition(state.P1, hu, active, "poisoned", true))

	must(t)(g.Move(state.P1, hu, "p1-1", state.Loc{Zone: state.Bench, Slot: 2}, "", ""))
	side := g.State().Sides[state.P1]
	if !side.Active.Empty() {
		t.Fatalf("active still holds %v", side.Active.Cards())
	}
	b := side.Bench[2]
	if b.Top() != "p1-1" || !slices.Equal(b.Energy, []state.IID{energy}) || !slices.Equal(b.Tools, []state.IID{tool}) {
		t.Fatalf("bench 2 = %+v", b)
	}
	if b.Damage != 30 || len(b.Conditions) > 0 {
		t.Fatalf("bench 2 damage %d conditions %v", b.Damage, b.Conditions)
	}
	if p := g.State().Check(); len(p) > 0 {
		t.Fatal(p)
	}
}

// The board counts turns the way players do: each player's Nth turn is
// turn N, while the stored state keeps counting one turn per player.
func TestBoardViewShowsCombinedTurn(t *testing.T) {
	g := startedGame(t)
	hu, cl := state.ActorHuman, state.ActorClaude
	check := func(stored, shown int, current state.Seat) {
		t.Helper()
		if st := g.State(); st.Turn != stored || st.Current != current {
			t.Fatalf("stored turn %d current %s, want %d %s", st.Turn, st.Current, stored, current)
		}
		for _, viewer := range state.Seats {
			v, err := g.View(viewer, "test", "test")
			if err != nil {
				t.Fatal(err)
			}
			if v.Turn != shown || v.Current != current {
				t.Fatalf("%s sees turn %d current %s, want %d %s", viewer, v.Turn, v.Current, shown, current)
			}
		}
	}
	check(1, 1, state.P1)
	must(t)(g.EndTurn(state.P1, hu))
	check(2, 1, state.P2) // the second player's first turn
	must(t)(g.EndTurn(state.P2, cl))
	check(3, 2, state.P1) // the first player's second turn
	must(t)(g.EndTurn(state.P1, hu))
	check(4, 2, state.P2)
}

// An effect's end turn is shown as the combined turn plus the player whose
// turn it ends on; the stored effect keeps the per-player turn.
func TestBoardViewShowsEffectEndAsCombinedTurn(t *testing.T) {
	g := startedGame(t)
	hu := state.ActorHuman
	// Ends on p2's second turn: per-player turn 4 with p1 going first.
	must(t)(g.AddEffect(state.P1, hu, "Items locked", state.P2, "p1-1", 2, state.P2))
	must(t)(g.AddEffect(state.P1, hu, "Until removed", state.P2, "p1-1", 0, ""))

	st := g.State()
	if len(st.Effects) != 2 {
		t.Fatalf("stored effects %+v", st.Effects)
	}
	if st.Effects[0].UntilTurn != 4 {
		t.Fatalf("stored until turn %d, want 4", st.Effects[0].UntilTurn)
	}
	if st.Effects[1].UntilTurn != 0 {
		t.Fatalf("stored until turn %d, want 0", st.Effects[1].UntilTurn)
	}

	v, err := g.View(state.P1, "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Effects) != 2 {
		t.Fatalf("view effects %+v", v.Effects)
	}
	e := v.Effects[0]
	if e.UntilTurn != 2 || e.UntilSeat != state.P2 {
		t.Fatalf("view effect until turn %d seat %q, want 2 p2", e.UntilTurn, e.UntilSeat)
	}
	if e.ID != st.Effects[0].ID || e.Text != "Items locked" || e.Seat != state.P2 || e.Source != "p1-1" {
		t.Fatalf("view effect fields not copied: %+v", e)
	}
	if e := v.Effects[1]; e.UntilTurn != 0 || e.UntilSeat != "" {
		t.Fatalf("lasting effect shows until turn %d seat %q, want 0 and none", e.UntilTurn, e.UntilSeat)
	}
	b, err := json.Marshal(v.Effects[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"until_turn":2`, `"until_seat":"p2"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("effect JSON %s lacks %s", b, want)
		}
	}

	// Projecting must not rewrite the stored effect.
	if got := g.State().Effects[0].UntilTurn; got != 4 {
		t.Fatalf("stored until turn after view %d, want 4", got)
	}
}

// tenTurnGame plays ten player turns, five each with p1 going first,
// leaving p2's fifth turn open. On the way it branches "alt" from the start
// of p1's third turn (per-player turn 5).
func tenTurnGame(t *testing.T) *Game {
	t.Helper()
	g := startedGame(t)
	hu, cl := state.ActorHuman, state.ActorClaude
	actor := map[state.Seat]state.Actor{state.P1: hu, state.P2: cl}
	altHead := 0
	for i := 0; i < 9; i++ {
		must(t)(g.EndTurn(g.State().Current, actor[g.State().Current]))
		if g.State().Turn == 5 {
			altHead = g.Head()
		}
	}
	if st := g.State(); st.Turn != 10 || st.Current != state.P2 {
		t.Fatalf("stored turn %d current %s, want 10 p2", st.Turn, st.Current)
	}
	mainHead := g.Head()
	if name, err := g.Rewind(altHead, "alt"); err != nil || name != "alt" {
		t.Fatalf("branching: %q %v", name, err)
	}
	if err := g.Checkout(store.MainBranch); err != nil {
		t.Fatal(err)
	}
	if g.Head() != mainHead {
		t.Fatalf("main head %d, want %d", g.Head(), mainHead)
	}
	return g
}

// combinedTurns maps each per-player turn of tenTurnGame to the turn and
// seat shown for it.
var combinedTurns = map[int]struct {
	turn int
	seat string
}{
	1: {1, "p1"}, 2: {1, "p2"}, 3: {2, "p1"}, 4: {2, "p2"}, 5: {3, "p1"},
	6: {3, "p2"}, 7: {4, "p1"}, 8: {4, "p2"}, 9: {5, "p1"}, 10: {5, "p2"},
}

// checkCombinedLog checks every line shows the combined turn and the seat
// of the per-player turn its event was stored in.
func checkCombinedLog(t *testing.T, g *Game, where string, lines []LogLine) {
	t.Helper()
	if len(lines) == 0 {
		t.Fatalf("%s: empty log", where)
	}
	maxTurn, setup := 0, 0
	seen := map[int]bool{}
	for _, l := range lines {
		e := g.log.Event(l.ID)
		if e == nil {
			t.Fatalf("%s: line %d has no event", where, l.ID)
		}
		if e.Turn == 0 {
			setup++
			if l.Turn != 0 || l.TurnSeat != "" {
				t.Errorf("%s: setup line %v shows turn %d seat %q", where, l, l.Turn, l.TurnSeat)
			}
			continue
		}
		want, ok := combinedTurns[e.Turn]
		if !ok {
			t.Fatalf("%s: line %d stored in unexpected turn %d", where, l.ID, e.Turn)
		}
		seen[e.Turn] = true
		if l.Turn != want.turn || l.TurnSeat != want.seat {
			t.Errorf("%s: line %v (per-player turn %d) shows turn %d %q, want %d %q",
				where, l, e.Turn, l.Turn, l.TurnSeat, want.turn, want.seat)
		}
		maxTurn = max(maxTurn, l.Turn)
	}
	if maxTurn != 5 {
		t.Errorf("%s: highest turn %d, want 5", where, maxTurn)
	}
	if setup == 0 {
		t.Errorf("%s: no setup lines", where)
	}
	if len(seen) != 10 {
		t.Errorf("%s: lines cover per-player turns %v, want all ten", where, seen)
	}
}

// The log, the playmat's log and the branch list count turns the way
// players do and name the player whose turn it is.
func TestLogAndBranchesShowCombinedTurn(t *testing.T) {
	g := tenTurnGame(t)
	lines, err := g.Log(0, "", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	checkCombinedLog(t, g, "get_log", lines)

	// Concrete values for the first three per-player turns.
	firstOf := map[int]LogLine{}
	for _, l := range lines {
		if e := g.log.Event(l.ID); e.Turn > 0 {
			if _, ok := firstOf[e.Turn]; !ok {
				firstOf[e.Turn] = l
			}
		}
	}
	for perPlayer, want := range map[int]struct {
		turn int
		seat string
	}{1: {1, "p1"}, 2: {1, "p2"}, 3: {2, "p1"}} {
		if l := firstOf[perPlayer]; l.Turn != want.turn || l.TurnSeat != want.seat {
			t.Errorf("per-player turn %d line %v shows %d %q, want %d %q", perPlayer, l, l.Turn, l.TurnSeat, want.turn, want.seat)
		}
	}

	snap, err := g.UISnapshot(state.P1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Log) != len(lines) {
		t.Fatalf("playmat log has %d lines, get_log %d", len(snap.Log), len(lines))
	}
	checkCombinedLog(t, g, "playmat", snap.Log)

	wantBranches := map[string]struct {
		turn int
		seat string
	}{store.MainBranch: {5, "p2"}, "alt": {3, "p1"}}
	for where, branches := range map[string][]BranchInfo{"list_branches": g.Branches(), "playmat": snap.Branches} {
		got := map[string]bool{}
		for _, b := range branches {
			if b.Turn > 5 {
				t.Errorf("%s: branch %s shows turn %d", where, b.Name, b.Turn)
			}
			want, ok := wantBranches[b.Name]
			if !ok {
				continue
			}
			got[b.Name] = true
			if b.Turn != want.turn || b.TurnSeat != want.seat {
				t.Errorf("%s: branch %s shows turn %d %q, want %d %q", where, b.Name, b.Turn, b.TurnSeat, want.turn, want.seat)
			}
		}
		if len(got) != len(wantBranches) {
			t.Errorf("%s: branches %+v lack main or alt", where, branches)
		}
	}
	b, err := json.Marshal(g.Branches())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"turn":5,"turn_seat":"p2"`) {
		t.Errorf("branch JSON %s lacks main's turn and seat", b)
	}
}

// Turn events read as the combined turn and the seat, and the action that
// starts a turn is labelled with the combined turn.
func TestTurnEventsReadAsCombinedTurn(t *testing.T) {
	g := tenTurnGame(t)
	lines, err := g.Log(0, "", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	var starts, ends []string
	var third *LogLine
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l.Text, "turn ") && strings.Contains(l.Text, " start "):
			starts = append(starts, l.Text)
		case strings.HasPrefix(l.Text, "turn ") && strings.Contains(l.Text, " end "):
			ends = append(ends, l.Text)
		}
		if l.Text == "turn 3 start (p2)" {
			third = &lines[i]
		}
	}
	wantStarts := []string{
		"turn 1 start (p1)", "turn 1 start (p2)", "turn 2 start (p1)", "turn 2 start (p2)", "turn 3 start (p1)",
		"turn 3 start (p2)", "turn 4 start (p1)", "turn 4 start (p2)", "turn 5 start (p1)", "turn 5 start (p2)",
	}
	if !slices.Equal(starts, wantStarts) {
		t.Errorf("turn starts\n  %q\nwant\n  %q", starts, wantStarts)
	}
	wantEnds := []string{
		"turn 1 end (p1)", "turn 1 end (p2)", "turn 2 end (p1)", "turn 2 end (p2)", "turn 3 end (p1)",
		"turn 3 end (p2)", "turn 4 end (p1)", "turn 4 end (p2)", "turn 5 end (p1)",
	}
	if !slices.Equal(ends, wantEnds) {
		t.Errorf("turn ends\n  %q\nwant\n  %q", ends, wantEnds)
	}

	// The second player's third turn.
	if third == nil {
		t.Fatal("no line reads turn 3 start (p2)")
	}
	if third.Turn != 3 || third.TurnSeat != "p2" || third.Label != "Turn 3" {
		t.Errorf("line %+v, want turn 3, seat p2, label Turn 3", *third)
	}
	s := third.String()
	for _, want := range []string{"T3 p2", "Turn 3]", "turn 3 start (p2)"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() %q lacks %q", s, want)
		}
	}
	b, err := json.Marshal(*third)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"turn":3,"turn_seat":"p2"`) {
		t.Errorf("line JSON %s lacks the turn and seat", b)
	}

	// Setup lines name no seat.
	setup := lines[0]
	if g.log.Event(setup.ID).Turn != 0 {
		t.Fatalf("first line %v is not setup", setup)
	}
	if s := setup.String(); !strings.HasPrefix(s, fmt.Sprintf("#%d T0 ", setup.ID)) || strings.Contains(s, "T0 p") {
		t.Errorf("setup line String() %q", s)
	}
}

// An effect's end is described as the combined turn and the seat it ends on.
func TestLogDescribesEffectEndAsCombinedTurn(t *testing.T) {
	g := startedGame(t)
	// Per-player turn 4 with p1 going first is p2's second turn.
	must(t)(g.AddEffect(state.P1, state.ActorHuman, "Items locked", state.P2, "p1-1", 2, state.P2))
	lines, err := g.Log(0, "", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last.Text, "Items locked (until end of turn 2 (p2))") {
		t.Errorf("effect line %q lacks until end of turn 2 (p2)", last.Text)
	}
	if last.Turn != 1 || last.TurnSeat != "p1" {
		t.Errorf("effect line shows turn %d %q, want 1 p1", last.Turn, last.TurnSeat)
	}
}

// An effect that ends on the second player's turn 4 lasts through the first
// player's turn 4 and ends with the second player's.
func TestEffectEndsOnNamedPlayersTurn(t *testing.T) {
	g := startedGame(t)
	hu, cl := state.ActorHuman, state.ActorClaude
	actor := map[state.Seat]state.Actor{state.P1: hu, state.P2: cl}
	must(t)(g.AddEffect(state.P1, hu, "Items locked", state.P2, "p1-1", 4, state.P2))
	if got := g.State().Effects[0].UntilTurn; got != 8 {
		t.Fatalf("stored until turn %d, want 8 (p2's turn 4 with p1 first)", got)
	}
	lines, err := g.Log(0, "", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	if last := lines[len(lines)-1]; !strings.Contains(last.Text, "until end of turn 4 (p2)") {
		t.Errorf("effect line %q lacks until end of turn 4 (p2)", last.Text)
	}

	// End every turn up to and including p1's turn 4 (per-player turn 7).
	for g.State().Turn < 8 {
		st := g.State()
		if len(st.Effects) != 1 {
			t.Fatalf("effect gone during turn %d (%s): %+v", state.Round(st.Turn), st.Current, st.Effects)
		}
		must(t)(g.EndTurn(st.Current, actor[st.Current]))
	}
	st := g.State()
	if st.Turn != 8 || st.Current != state.P2 {
		t.Fatalf("stored turn %d current %s, want 8 p2", st.Turn, st.Current)
	}
	if len(st.Effects) != 1 {
		t.Fatalf("effect gone after p1's turn 4 ended: %+v", st.Effects)
	}
	must(t)(g.EndTurn(state.P2, cl))
	if n := len(g.State().Effects); n != 0 {
		t.Fatalf("effect still on the board after p2's turn 4 ended: %+v", g.State().Effects)
	}
}

// An end turn without the seat it ends on is ambiguous and refused.
func TestEffectEndTurnNeedsSeat(t *testing.T) {
	g := startedGame(t)
	head := g.Head()
	_, err := g.AddEffect(state.P1, state.ActorHuman, "Items locked", state.P2, "p1-1", 2, "")
	if err == nil {
		t.Fatal("effect with until turn but no seat was recorded")
	}
	if !strings.Contains(err.Error(), "until_seat") || !strings.Contains(err.Error(), "seat whose turn") {
		t.Errorf("error %q does not ask for until_seat", err)
	}
	if g.Head() != head || len(g.State().Effects) != 0 {
		t.Fatal("refused effect changed the game")
	}
}

// Acting for the wrong player names the combined turn and whose it is.
func TestWrongSeatErrorNamesCombinedTurn(t *testing.T) {
	g := startedGame(t)
	hu, cl := state.ActorHuman, state.ActorClaude
	if g.Mode() != ModePlay {
		t.Fatalf("mode %q, want play", g.Mode())
	}
	must(t)(g.EndTurn(state.P1, hu))
	// p2's first turn.
	want := "it is p2's turn (turn 1), not p1's"
	if _, err := g.Draw(state.P1, hu, 1); err == nil || err.Error() != want {
		t.Errorf("p1 drawing on p2's turn 1: %v, want %q", err, want)
	}
	if _, err := g.EndTurn(state.P1, hu); err == nil || err.Error() != want {
		t.Errorf("p1 ending p2's turn 1: %v, want %q", err, want)
	}
	must(t)(g.EndTurn(state.P2, cl))
	must(t)(g.EndTurn(state.P1, hu))
	must(t)(g.EndTurn(state.P2, cl))
	// p1's third turn: per-player turn 5.
	if st := g.State(); st.Turn != 5 || st.Current != state.P1 {
		t.Fatalf("stored turn %d current %s, want 5 p1", st.Turn, st.Current)
	}
	want = "it is p1's turn (turn 3), not p2's"
	if _, err := g.Draw(state.P2, cl, 1); err == nil || err.Error() != want {
		t.Errorf("p2 drawing on p1's turn 3: %v, want %q", err, want)
	}
	// Review mode lifts the turn lock, but only the open turn can end.
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	if _, err := g.EndTurn(state.P2, cl); err == nil || err.Error() != want {
		t.Errorf("p2 ending p1's turn 3 in review: %v, want %q", err, want)
	}
}

// Opening-turn warnings follow each player's first turn: no Supporter or
// attack for the player going first, no evolving for either player.
func TestOpeningTurnWarnings(t *testing.T) {
	_, g := newTestGame(t)
	hu, cl := state.ActorHuman, state.ActorClaude
	actor := map[state.Seat]state.Actor{state.P1: hu, state.P2: cl}
	for _, seat := range state.Seats {
		for _, n := range []int{1, 2, 5, 6, 13, 14} {
			iid := state.IID(fmt.Sprintf("%s-%d", seat, n))
			if l, _ := g.State().Find(iid); l.Zone != state.Hand {
				must(t)(g.Move(seat, actor[seat], iid, state.Loc{Zone: state.Hand}, "", ""))
			}
		}
	}
	must(t)(g.PlaceActive(state.P1, hu, "p1-1"))
	must(t)(g.PlaceActive(state.P2, cl, "p2-1"))
	must(t)(g.StartGame(state.P1, hu))

	const (
		supporter = "the player going first cannot play a Supporter on their first turn"
		attack    = "the player going first cannot attack on their first turn"
		evolve    = "evolving on a player's first turn"
	)
	has := func(r *Result, w string) bool { return slices.Contains(r.Warnings, w) }
	active := func(s state.Seat) state.Loc { return state.Loc{Seat: s, Zone: state.Active} }
	bench := func(s state.Seat) state.Loc { return state.Loc{Seat: s, Zone: state.Bench, Slot: 0} }

	// p1's turn 1 (per-player 1).
	must(t)(g.Bench(state.P1, hu, "p1-2", -1))
	if r := must(t)(g.PlayTrainer(state.P1, hu, "p1-13")); !has(r, supporter) {
		t.Errorf("Supporter in p1's turn 1: warnings %v lack %q", r.Warnings, supporter)
	}
	if r := must(t)(g.Attack(state.P1, hu, "Punch", active(state.P2), 0)); !has(r, attack) {
		t.Errorf("attack in p1's turn 1: warnings %v lack %q", r.Warnings, attack)
	}
	if r := must(t)(g.Evolve(state.P1, hu, "p1-5", active(state.P1))); !has(r, evolve) {
		t.Errorf("evolving in p1's turn 1: warnings %v lack %q", r.Warnings, evolve)
	}
	must(t)(g.EndTurn(state.P1, hu))

	// p2's turn 1 (per-player 2).
	must(t)(g.Bench(state.P2, cl, "p2-2", -1))
	if r := must(t)(g.PlayTrainer(state.P2, cl, "p2-13")); has(r, supporter) {
		t.Errorf("Supporter in p2's turn 1 warned: %v", r.Warnings)
	}
	if r := must(t)(g.Attack(state.P2, cl, "Punch", active(state.P1), 0)); has(r, attack) {
		t.Errorf("attack in p2's turn 1 warned: %v", r.Warnings)
	}
	if r := must(t)(g.Evolve(state.P2, cl, "p2-5", active(state.P2))); !has(r, evolve) {
		t.Errorf("evolving in p2's turn 1: warnings %v lack %q", r.Warnings, evolve)
	}
	must(t)(g.EndTurn(state.P2, cl))

	// p1's turn 2 (per-player 3).
	if r := must(t)(g.Evolve(state.P1, hu, "p1-6", bench(state.P1))); has(r, evolve) {
		t.Errorf("evolving in p1's turn 2 warned: %v", r.Warnings)
	}
	if r := must(t)(g.PlayTrainer(state.P1, hu, "p1-14")); has(r, supporter) {
		t.Errorf("Supporter in p1's turn 2 warned: %v", r.Warnings)
	}
	if r := must(t)(g.Attack(state.P1, hu, "Aura Sphere", active(state.P2), 0)); has(r, attack) {
		t.Errorf("attack in p1's turn 2 warned: %v", r.Warnings)
	}
	must(t)(g.EndTurn(state.P1, hu))

	// p2's turn 2 (per-player 4).
	if r := must(t)(g.Evolve(state.P2, cl, "p2-6", bench(state.P2))); has(r, evolve) {
		t.Errorf("evolving in p2's turn 2 warned: %v", r.Warnings)
	}
}
