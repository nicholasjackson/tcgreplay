package game

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// pairDeck is testDeck with its Bands swapped for two copies of each print
// of Twin Peak (TST 10 and TST 11), a made-up pair-played Stadium.
const pairDeck = `Pokémon: 8
4 Riolu TST 1
4 Lucario TST 2

Trainer: 14
4 Ultra Ball TST 3
4 Iono TST 4
2 Arena TST 5
2 Twin Peak TST 10
2 Twin Peak TST 11

Energy: 38
38 Basic {F} Energy SVE 6

Total Cards: 60`

// pairCards are the cards in hand after pairGame: p1's four Twin Peaks
// (two of each print) and an Iono, and p2's Arena.
type pairCards struct {
	a, a2 state.IID // p1's Twin Peak tst-10
	b, b2 state.IID // p1's Twin Peak tst-11
	iono  state.IID // p1's Iono
	arena state.IID // p2's Arena
}

// pairGame starts a game from db with pairDeck for both seats, p1 going
// first, holding the cards in pairCards. The open action is closed.
func pairGame(t *testing.T, db cardstest.FakeDB) (*Game, pairCards) {
	t.Helper()
	m := NewManager(t.TempDir(), db)
	seed := uint64(1)
	m.Rand = func() *rand.Rand { seed++; return rand.New(rand.NewPCG(seed, 99)) }
	g, _, err := m.New(context.Background(), NewGameRequest{ID: "pairs", DeckP1: pairDeck, DeckP2: pairDeck})
	if err != nil {
		t.Fatal(err)
	}
	ofDef := func(seat state.Seat, def string) []state.IID {
		var out []state.IID
		st := g.State()
		for i := 1; i <= 60; i++ {
			iid := state.IID(fmt.Sprintf("%s-%d", seat, i))
			if st.Cards[iid].Def == def {
				out = append(out, iid)
			}
		}
		return out
	}
	tens, elevens := ofDef(state.P1, "tst-10"), ofDef(state.P1, "tst-11")
	if len(tens) != 2 || len(elevens) != 2 {
		t.Fatalf("p1 deck has Twin Peaks %v %v, want two of each print", tens, elevens)
	}
	c := pairCards{a: tens[0], a2: tens[1], b: elevens[0], b2: elevens[1],
		iono: ofDef(state.P1, "tst-4")[0], arena: ofDef(state.P2, "tst-5")[0]}
	toHand := func(seat state.Seat, iids ...state.IID) {
		for _, iid := range iids {
			if l, _ := g.State().Find(iid); l.Zone != state.Hand {
				must(t)(g.Move(seat, actorOf(seat), iid, state.Loc{Zone: state.Hand}, "", ""))
			}
		}
	}
	toHand(state.P1, "p1-1", c.a, c.a2, c.b, c.b2, c.iono)
	toHand(state.P2, "p2-1", c.arena)
	must(t)(g.PlaceActive(state.P1, state.ActorHuman, "p1-1"))
	must(t)(g.PlaceActive(state.P2, state.ActorClaude, "p2-1"))
	must(t)(g.StartGame(state.P1, state.ActorHuman))
	g.EndAction()
	return g, c
}

// moves decodes the move events among evs.
func moves(t *testing.T, evs []state.Event) []*state.MovePayload {
	t.Helper()
	var out []*state.MovePayload
	for i := range evs {
		if evs[i].Type != state.EvMove {
			continue
		}
		p, err := evs[i].Decode()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, p.(*state.MovePayload))
	}
	return out
}

// actionsOf is the distinct action ids of evs, in order.
func actionsOf(evs []state.Event) []string {
	var out []string
	for _, e := range evs {
		if !slices.Contains(out, e.Action) {
			out = append(out, e.Action)
		}
	}
	return out
}

func checkStadium(t *testing.T, g *Game, want ...state.IID) {
	t.Helper()
	if got := g.State().Stadium; !slices.Equal(got, want) {
		t.Fatalf("stadium = %v, want %v", got, want)
	}
}

func checkIn(t *testing.T, g *Game, zone state.Zone, iids ...state.IID) {
	t.Helper()
	for _, iid := range iids {
		l, err := g.State().Find(iid)
		if err != nil {
			t.Fatal(err)
		}
		if l.Zone != zone || l.Seat != state.P1 {
			t.Fatalf("%s is in %v, want p1 %s", iid, l, zone)
		}
	}
}

// logLines is the log text after event since.
func logLines(t *testing.T, g *Game, since int) []string {
	t.Helper()
	lines, err := g.Log(since, "", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, l := range lines {
		out = append(out, l.Text)
	}
	return out
}

const lonePeak = "Twin Peak (%s) is played as a pair: play a second, different Twin Peak with it"

// Two different prints played together fill the Stadium spot as one
// Stadium in one action and one move event.
func TestStadiumPairPlayedTogether(t *testing.T) {
	g, c := pairGame(t, cardstest.New())
	head := headOf(g)
	res := must(t)(g.PlayTrainerWith(state.P1, state.ActorHuman, c.a, c.b))
	checkStadium(t, g, c.a, c.b)
	evs := eventsAfter(t, g, head)
	if acts := actionsOf(evs); len(acts) != 1 || acts[0] != res.ActionID || res.ActionID == "" {
		t.Fatalf("actions %v, result action %q: want one", acts, res.ActionID)
	}
	mv := moves(t, evs)
	if len(mv) != 1 || mv[0].IID != c.a || mv[0].Pair != c.b || mv[0].To.Zone != state.Stadium {
		t.Fatalf("moves %+v, want one move of %s + %s to the stadium", mv, c.a, c.b)
	}
	want := fmt.Sprintf("p1 moved %s Twin Peak + %s Twin Peak p1:hand → stadium", c.a, c.b)
	if lines := logLines(t, g, head); !slices.Contains(lines, want) {
		t.Fatalf("log %q lacks %q", lines, want)
	}
	if ws := g.Warnings(); hasWarning(ws, "is played as a pair") || hasWarning(ws, "same print") {
		t.Fatalf("a proper pair warns: %v", ws)
	}
}

// Only a Stadium is played as a pair.
func TestStadiumPairRejectsNonStadium(t *testing.T) {
	g, c := pairGame(t, cardstest.New())
	head := headOf(g)
	if _, err := g.PlayTrainerWith(state.P1, state.ActorHuman, c.iono, c.a); err == nil || !strings.Contains(err.Error(), "only a Stadium is played as a pair") {
		t.Fatalf("err = %v, want only a Stadium is played as a pair", err)
	}
	if headOf(g) != head {
		t.Fatal("a refused pair changed the log")
	}
}

// A lone half warns while it stands alone; its partner played while the
// half's play is open joins that action, so one undo returns both.
func TestStadiumPairCompletedInSameAction(t *testing.T) {
	g, c := pairGame(t, cardstest.New())
	first := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, c.a))
	checkStadium(t, g, c.a)
	lone := fmt.Sprintf(lonePeak, c.a)
	if !slices.Contains(first.Warnings, lone) || !slices.Contains(g.Warnings(), lone) {
		t.Fatalf("lone half: result %v, standing %v; want %q", first.Warnings, g.Warnings(), lone)
	}
	second := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, c.b))
	checkStadium(t, g, c.a, c.b)
	if second.ActionID != first.ActionID {
		t.Fatalf("completion opened action %q, want it to join %q", second.ActionID, first.ActionID)
	}
	if ws := g.Warnings(); hasWarning(ws, "is played as a pair") || hasWarning(ws, "Stadiums this turn") {
		t.Fatalf("completed pair still warns: %v", ws)
	}
	if _, err := g.Undo(1); err != nil {
		t.Fatal(err)
	}
	checkStadium(t, g)
	checkIn(t, g, state.Hand, c.a, c.b)
}

// A partner played after the half's play has closed completes the pair
// in its own action, and is not a second Stadium played that turn.
func TestStadiumPairCompletedLater(t *testing.T) {
	g, c := pairGame(t, cardstest.New())
	first := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, c.a))
	g.EndAction()
	head := headOf(g)
	second := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, c.b))
	checkStadium(t, g, c.a, c.b)
	if second.ActionID == "" || second.ActionID == first.ActionID {
		t.Fatalf("completion action %q, want its own (first %q)", second.ActionID, first.ActionID)
	}
	mv := moves(t, eventsAfter(t, g, head))
	if len(mv) != 1 || mv[0].IID != c.b || mv[0].As != state.AsPair {
		t.Fatalf("moves %+v, want one move of %s as pair", mv, c.b)
	}
	if hasWarning(second.Warnings, "Stadiums this turn") || hasWarning(g.Warnings(), "Stadiums this turn") {
		t.Fatalf("completion counted as a second Stadium: %v %v", second.Warnings, g.Warnings())
	}
	if hasWarning(g.Warnings(), "is played as a pair") {
		t.Fatalf("lone-half warning outlived the completion: %v", g.Warnings())
	}
	if _, err := g.Undo(1); err != nil {
		t.Fatal(err)
	}
	checkStadium(t, g, c.a)
	checkIn(t, g, state.Hand, c.b)
}

// A Stadium played over a pair sends both halves to their owner's discard
// in one move; undo puts both back, and the log has one line for them.
func TestStadiumReplacesPair(t *testing.T) {
	g, c := pairGame(t, cardstest.New())
	must(t)(g.PlayTrainerWith(state.P1, state.ActorHuman, c.a, c.b))
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	head := headOf(g)
	res := must(t)(g.PlayTrainer(state.P2, state.ActorClaude, c.arena))
	checkStadium(t, g, c.arena)
	checkIn(t, g, state.Discard, c.a, c.b)
	evs := eventsAfter(t, g, head)
	if acts := actionsOf(evs); len(acts) != 1 || acts[0] != res.ActionID {
		t.Fatalf("actions %v, want only %q", acts, res.ActionID)
	}
	var out []*state.MovePayload
	for _, mv := range moves(t, evs) {
		if mv.IID == c.a || mv.IID == c.b || mv.Pair == c.a || mv.Pair == c.b {
			out = append(out, mv)
		}
	}
	if len(out) != 1 || out[0].Pair == "" || out[0].To.Zone != state.Discard {
		t.Fatalf("pair moves %+v, want one paired move to the discard", out)
	}
	var peaks []string
	for _, l := range logLines(t, g, head) {
		if strings.Contains(l, "Twin Peak") {
			peaks = append(peaks, l)
		}
	}
	want := fmt.Sprintf("p2 moved %s Twin Peak + %s Twin Peak stadium → p1:discard", c.a, c.b)
	if len(peaks) != 1 || peaks[0] != want {
		t.Fatalf("log lines for the pair %q, want [%q]", peaks, want)
	}
	if _, err := g.Undo(1); err != nil {
		t.Fatal(err)
	}
	checkStadium(t, g, c.a, c.b)
	if l, _ := g.State().Find(c.arena); l.Zone != state.Hand {
		t.Fatalf("arena in %v after undo, want hand", l)
	}
}

// Discarding either half from the spot moves both in one logged action,
// and one undo returns both.
func TestStadiumPairDiscardedTogether(t *testing.T) {
	for _, half := range []string{"first", "second"} {
		t.Run(half, func(t *testing.T) {
			g, c := pairGame(t, cardstest.New())
			must(t)(g.PlayTrainerWith(state.P1, state.ActorHuman, c.a, c.b))
			g.EndAction()
			iid := c.a
			if half == "second" {
				iid = c.b
			}
			head := headOf(g)
			res := must(t)(g.Move(state.P1, state.ActorHuman, iid, state.Loc{Zone: state.Discard}, "", ""))
			checkStadium(t, g)
			checkIn(t, g, state.Discard, c.a, c.b)
			evs := eventsAfter(t, g, head)
			if acts := actionsOf(evs); len(acts) != 1 || acts[0] != res.ActionID {
				t.Fatalf("actions %v, want only %q", acts, res.ActionID)
			}
			if mv := moves(t, evs); len(mv) != 1 || mv[0].Pair == "" {
				t.Fatalf("moves %+v, want one paired move", mv)
			}
			if lines := logLines(t, g, head); len(lines) != 1 || !strings.Contains(lines[0], " + ") || !strings.HasSuffix(lines[0], "stadium → p1:discard") {
				t.Fatalf("log %q, want one line moving the pair to the discard", lines)
			}
			if _, err := g.Undo(1); err != nil {
				t.Fatal(err)
			}
			checkStadium(t, g, c.a, c.b)
		})
	}
}

// Two copies of one print still go in as a pair, with a warning that
// stands only while they are in the spot.
func TestStadiumPairIdenticalPrintsWarn(t *testing.T) {
	g, c := pairGame(t, cardstest.New())
	res := must(t)(g.PlayTrainerWith(state.P1, state.ActorHuman, c.a, c.a2))
	checkStadium(t, g, c.a, c.a2)
	want := "the Stadium pair Twin Peak is two copies of the same print (tst-10): a pair needs two different prints"
	if !slices.Contains(res.Warnings, want) || !slices.Contains(g.Warnings(), want) {
		t.Fatalf("result %v, standing %v; want %q", res.Warnings, g.Warnings(), want)
	}
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	must(t)(g.PlayTrainer(state.P2, state.ActorClaude, c.arena))
	if hasWarning(g.Warnings(), "same print") {
		t.Fatalf("identical-print warning outlived the pair: %v", g.Warnings())
	}
}

// A lone half's warning ends when it leaves the spot, not only when its
// pair is completed.
func TestStadiumLoneHalfWarningEndsWhenReplaced(t *testing.T) {
	g, c := pairGame(t, cardstest.New())
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, c.a))
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	if !slices.Contains(g.Warnings(), fmt.Sprintf(lonePeak, c.a)) {
		t.Fatalf("lone half not warned on the next turn: %v", g.Warnings())
	}
	must(t)(g.PlayTrainer(state.P2, state.ActorClaude, c.arena))
	checkStadium(t, g, c.arena)
	if hasWarning(g.Warnings(), "is played as a pair") {
		t.Fatalf("lone-half warning outlived the half: %v", g.Warnings())
	}
}

// A Stadium played while one of the same name is in play warns and still
// replaces it.
func TestStadiumPairOverSameNameWarns(t *testing.T) {
	g, c := pairGame(t, cardstest.New())
	must(t)(g.PlayTrainerWith(state.P1, state.ActorHuman, c.a, c.b))
	res := must(t)(g.PlayTrainerWith(state.P1, state.ActorHuman, c.a2, c.b2))
	if !slices.Contains(res.Warnings, "a Stadium named Twin Peak is already in play") {
		t.Fatalf("warnings %v, want the same-name warning", res.Warnings)
	}
	checkStadium(t, g, c.a2, c.b2)
	checkIn(t, g, state.Discard, c.a, c.b)
	// Two pairs played are two Stadiums played (unlike a completion).
	if !slices.Contains(g.Warnings(), "p1 played 2 Stadiums this turn") {
		t.Fatalf("warnings %v, want two Stadiums played this turn", g.Warnings())
	}
}

// The pair rule comes from the card data: without the mark Twin Peak is an
// ordinary Stadium, and a second one replaces the first.
func TestStadiumPairRuleIsData(t *testing.T) {
	db := cardstest.New()
	db["tst-10"].PlayedAsPair = false
	db["tst-11"].PlayedAsPair = false
	g, c := pairGame(t, db)
	first := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, c.a))
	if hasWarning(first.Warnings, "is played as a pair") || hasWarning(g.Warnings(), "is played as a pair") {
		t.Fatalf("unmarked Stadium warned as a lone half: %v %v", first.Warnings, g.Warnings())
	}
	g.EndAction()
	second := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, c.b))
	checkStadium(t, g, c.b)
	checkIn(t, g, state.Discard, c.a)
	if second.ActionID == first.ActionID {
		t.Fatal("unmarked second Stadium joined the first's action")
	}
	if !slices.Contains(second.Warnings, "a Stadium named Twin Peak is already in play") {
		t.Fatalf("warnings %v, want the same-name warning", second.Warnings)
	}
}
