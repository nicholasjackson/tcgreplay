package game

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// TestImportCorpus imports every corpus log and checks it becomes the
// current game with no invariant violations.
func TestImportCorpus(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	logs, _ := filepath.Glob("../../testdata/logs/*.txt")
	m := NewManager(t.TempDir(), nil)
	for _, path := range logs {
		t.Run(filepath.Base(path), func(t *testing.T) {
			text, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g-" + filepath.Base(path[:len(path)-4])})
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range res.Report.Problems {
				t.Errorf("line %d %q: %s", p.Line, p.Text, p.Err)
			}
			for _, w := range res.Game.State().Check() {
				t.Errorf("invariant: %s", w)
			}
			if res.Game.Mode() != ModeReview {
				t.Errorf("mode %s, want review", res.Game.Mode())
			}
			want := snapshot(t, res.Game)
			g, err := m.Current()
			if err != nil {
				t.Fatal(err)
			}
			if g != res.Game || g.ID != res.Game.ID {
				t.Errorf("current game %s, want the import %s", g.ID, res.Game.ID)
			}
			if got := snapshot(t, g); got != want {
				t.Error("current state differs from imported state")
			}
		})
	}
}

// TestJumpTo reuses a branch already at the event and names new ones
// base, base-2, ... without moving the branch left behind.
func TestJumpTo(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Game
	end := g.Head()
	jump := func(ev int, base, want string) {
		t.Helper()
		name, err := g.JumpTo(ev, base)
		if err != nil {
			t.Fatal(err)
		}
		if name != want || g.Branch() != want || g.Head() != ev {
			t.Fatalf("JumpTo(%d, %s) = %s on %s at %d, want %s at %d", ev, base, name, g.Branch(), g.Head(), want, ev)
		}
	}
	jump(20, "turn-1", "turn-1")
	jump(20, "turn-1", "turn-1") // reused, not turn-1-2
	jump(25, "turn-1", "turn-1-2")
	jump(20, "turn-1", "turn-1")
	if err := g.Checkout("main"); err != nil || g.Head() != end {
		t.Fatalf("main moved: head %d, want %d (%v)", g.Head(), end, err)
	}
	if _, err := g.JumpTo(end+100, "x"); err == nil {
		t.Error("jumped to a missing event")
	}
}

// TestSetOpponentDeck rebuilds the current import with an assumed opponent
// list: the owner's side is unchanged and every opponent card has an
// identity. It refuses with no game, with a game without a record, with a
// blank list, and with branches unless they may be discarded.
func TestSetOpponentDeck(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(ctx, ImportRequest{Log: string(text), Deck: string(deckText), ID: "g"}) // random seed
	if err != nil {
		t.Fatal(err)
	}
	before := res.Game.State()
	opp := `Pokémon: 12
4 Abra
3 Kadabra
3 Alakazam
2 Duskull

Trainer: 4
4 Buddy-Buddy Poffin

Energy: 44
44 Basic Psychic Energy`

	res, err = m.SetOpponentDeck(ctx, opp, false)
	if err != nil {
		t.Fatal(err)
	}
	after := res.Game.State()
	if len(res.Report.Problems) > 0 {
		t.Errorf("problems: %v", res.Report.Problems)
	}
	for iid, c := range after.Cards {
		b := before.Cards[iid]
		if c.Owner == state.P1 {
			if c.Name != b.Name {
				t.Errorf("%s was %s, now %s", iid, b.Name, c.Name)
			}
			continue
		}
		if c.Name == "" {
			t.Errorf("%s has no identity", iid)
		}
	}
	for _, seat := range state.Seats {
		bs, as := before.Sides[seat], after.Sides[seat]
		if !slices.Equal(bs.Hand, as.Hand) || !slices.Equal(bs.Prizes, as.Prizes) || !slices.Equal(bs.Deck, as.Deck) {
			t.Errorf("%s's cards moved in the rebuild", seat)
		}
	}

	if res.Game.ID != "g" {
		t.Errorf("rebuilt game id %s, want g", res.Game.ID)
	}
	if cur, err := m.Current(); err != nil || cur != res.Game {
		t.Errorf("rebuilt game is not current (%v)", err)
	}

	// A blank list is refused and the game is kept.
	if _, err := m.SetOpponentDeck(ctx, " \n", false); err == nil {
		t.Error("rebuilt with a blank list")
	}
	if cur, _ := m.Current(); cur != res.Game {
		t.Error("blank list replaced the game")
	}

	// A game with branches is not replaced unless they may be discarded.
	if _, err := res.Game.JumpTo(10, "look"); err != nil {
		t.Fatal(err)
	}
	withBranch := res.Game
	want := snapshot(t, withBranch)
	_, err = m.SetOpponentDeck(ctx, opp, false)
	if err == nil || !strings.Contains(err.Error(), "discard_branches") || !strings.Contains(err.Error(), "look") {
		t.Errorf("rebuild of a game with branches: err %v, want one naming the branch and discard_branches", err)
	}
	if cur, _ := m.Current(); cur != withBranch || snapshot(t, cur) != want || len(cur.Branches()) < 2 {
		t.Error("refused rebuild changed the current game")
	}
	res, err = m.SetOpponentDeck(ctx, opp, true)
	if err != nil {
		t.Fatalf("rebuild discarding branches: %v", err)
	}
	if res.Game == withBranch || res.Game.ID != "g" {
		t.Errorf("discard_branches did not rebuild game g: %s", res.Game.ID)
	}
	for _, br := range res.Game.Branches() {
		if br.Name != "main" {
			t.Errorf("branch %s survived the rebuild", br.Name)
		}
	}
	if cur, _ := m.Current(); cur != res.Game {
		t.Error("rebuilt game is not current")
	}

	// A new_game game has no record to rebuild from.
	ng, _, err := m.New(ctx, NewGameRequest{ID: "play", DeckP1: string(deckText), DeckP2: string(deckText)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetOpponentDeck(ctx, opp, true); err == nil || !strings.Contains(err.Error(), "cannot be rebuilt") {
		t.Errorf("rebuild of a new_game game: err %v, want cannot be rebuilt", err)
	}
	if cur, _ := m.Current(); cur != ng {
		t.Error("refused rebuild replaced the new_game game")
	}

	// No game open.
	if _, err := NewManager(t.TempDir(), nil).SetOpponentDeck(ctx, opp, true); err == nil || !strings.Contains(err.Error(), "no game open") {
		t.Errorf("rebuild with no game open: err %v, want no game open", err)
	}
}

// TestDeckList lists a whole deck with where each copy is: deck and prizes
// merge into hidden for the owner, and the reveal view places them.
func TestDeckList(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Game
	sum := func(rows []DeckListRow, where string) (total, at int) {
		for _, r := range rows {
			total += r.Count
			at += r.Where[where]
		}
		return
	}
	seat, err := g.DeckList(state.P1, state.P1, "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	if total, prizes := sum(seat, "prizes"); total != 60 || prizes != 0 {
		t.Errorf("seat view: %d cards, %d shown as prizes; want 60 and 0", total, prizes)
	}
	reveal, err := g.DeckList(state.P1, "", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	st := g.State()
	if _, prizes := sum(reveal, "prizes"); prizes != st.Sides[state.P1].PrizesLeft() {
		t.Errorf("reveal view shows %d prizes, want %d", prizes, st.Sides[state.P1].PrizesLeft())
	}
	for _, r := range reveal {
		if r.Name == "Meowth ex" && r.Count != 1 {
			t.Errorf("Meowth ex count %d", r.Count)
		}
		if r.Where["hidden"] != 0 {
			t.Errorf("reveal view hides %s", r.Name)
		}
	}
	g.SetMode(ModePlay)
	if _, err := g.DeckList(state.P1, "", "test", "test"); err == nil {
		t.Error("reveal deck list in play mode")
	}
}

// TestTurnDamage shows only damage put on a Pokémon since the turn began.
func TestTurnDamage(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_carlos0oz_dragapult.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Game
	// Mega Brave's damage on Dragapult ex, and the start of that turn.
	path, _ := g.log.PathTo(g.Head())
	brave, start := 0, 0
	for _, e := range path {
		if e.Type == state.EvTurn {
			if p, _ := e.Decode(); p.(*state.TurnPayload).Phase == "start" && brave == 0 {
				start = e.ID
			}
		}
		if e.Type == state.EvDamage && brave == 0 {
			p, _ := e.Decode()
			if dp := p.(*state.DamagePayload); dp.Target.Seat == state.P2 && dp.Value == 300 {
				brave = e.ID
			}
		}
	}
	if brave == 0 {
		t.Fatal("no 300 damage on p2 found")
	}
	active := func(ev int, seat state.Seat) *state.StackView {
		t.Helper()
		if _, err := g.JumpTo(ev, fmt.Sprintf("at-%d", ev)); err != nil {
			t.Fatal(err)
		}
		snap, err := g.UISnapshot("", nil)
		if err != nil {
			t.Fatal(err)
		}
		return snap.Board.Sides[seat].Active
	}
	if lucario := active(start, state.P1); lucario.Damage != 200 || lucario.DamageTurn != 0 {
		t.Errorf("turn start: Mega Lucario ex damage %d, this turn %d; want 200 and 0", lucario.Damage, lucario.DamageTurn)
	}
	if dragapult := active(brave, state.P2); dragapult.Damage != 300 || dragapult.DamageTurn != 300 {
		t.Errorf("after Mega Brave: Dragapult ex damage %d, this turn %d; want 300 and 300", dragapult.Damage, dragapult.DamageTurn)
	}
}

// TestReviewEitherSide lets either seat act at any time in review mode,
// but only the turn in progress can end; play mode keeps the turn lock.
func TestReviewEitherSide(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_carlos0oz_dragapult.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Game
	cur := g.State().Current
	other := cur.Other()
	if _, err := g.Draw(other, state.ActorHuman, 1); err != nil {
		t.Errorf("review: %s could not draw on %s's turn: %v", other, cur, err)
	}
	if _, err := g.EndTurn(other, state.ActorHuman); err == nil {
		t.Errorf("review: %s ended %s's turn", other, cur)
	}
	if _, err := g.EndTurn(cur, state.ActorHuman); err != nil {
		t.Errorf("review: %s could not end its turn: %v", cur, err)
	}
	g.SetMode(ModePlay)
	now := g.State().Current
	if _, err := g.Draw(now.Other(), state.ActorHuman, 1); err == nil {
		t.Error("play: drew on the other seat's turn")
	}
}

// TestReplay steps through a branch one action at a time.
func TestReplay(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Game
	head := g.Head()
	r, err := g.Replay("", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.To != head || len(r.Steps) == 0 {
		t.Fatalf("replay to %d with %d steps", r.To, len(r.Steps))
	}
	prev := 20
	for _, s := range r.Steps {
		if s.First != prev+1 || s.Last < s.First || len(s.Lines) != s.Last-s.First+1 {
			t.Fatalf("step %+v does not follow event %d", s, prev)
		}
		prev = s.Last
	}
	if prev != head {
		t.Errorf("steps end at %d, want %d", prev, head)
	}
	// The start board is the position right after event 20.
	if _, err := g.JumpTo(20, "at-20"); err != nil {
		t.Fatal(err)
	}
	snap, err := g.UISnapshot("", nil)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(snap.Board)
	b, _ := json.Marshal(r.Start)
	if string(a) != string(b) {
		t.Error("replay start differs from the board at event 20")
	}
	// A branch's own moves follow its fork point.
	if _, err := g.Draw(state.P1, state.ActorClaude, 1); err != nil {
		t.Fatal(err)
	}
	r, err = g.Replay("", 20, 0)
	if err != nil || len(r.Steps) != 1 || r.Steps[0].First != head+1 {
		t.Fatalf("branch replay: %+v, %v", r, err)
	}
	if _, err := g.Replay("", head, 0); err == nil {
		t.Error("replayed from an event on another branch")
	}
}

// TestUndo steps back over playmat moves but never into the import.
func TestUndo(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Game
	if _, err := g.Undo(1); err == nil {
		t.Fatal("undid part of the import")
	}
	if _, err := g.JumpTo(20, "try"); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, g)
	a, err := g.Draw(state.P1, state.ActorClaude, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Draw(state.P1, state.ActorClaude, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Draw("", state.ActorClaude, 1); err == nil {
		t.Error("drew for no seat")
	}
	head, err := g.Undo(1)
	if err != nil || head != a.Events[len(a.Events)-1] {
		t.Fatalf("undo 1: head %d, %v; want %d", head, err, a.Events[len(a.Events)-1])
	}
	if head, err = g.Undo(1); err != nil || head != 20 {
		t.Fatalf("undo 2: head %d, %v; want 20", head, err)
	}
	if snapshot(t, g) != before {
		t.Error("undo did not restore the position")
	}
	if g.Branch() != "try" {
		t.Errorf("on branch %s", g.Branch())
	}
}

// TestDrawsLikeMain shows a branch set to repeat main's draws draws the
// card the real game drew at the same point, even after a shuffle, and
// draws from the top with a warning when that card is not in the deck.
func TestDrawsLikeMain(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_carlos0oz_dragapult.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Game
	if err := g.SetDrawsLikeMain(true); err == nil {
		t.Error("main was set to repeat its own draws")
	}
	// Main's draw for p1 at the start of its second turn.
	path, err := g.log.PathTo(g.Head())
	if err != nil {
		t.Fatal(err)
	}
	var draw state.Event
	turns := 0
	for i, e := range path {
		if e.Type == state.EvTurn && e.Seat == state.P1 && i+1 < len(path) && isDraw(path[i+1], state.P1) {
			if turns++; turns == 2 {
				draw = path[i+1]
				break
			}
		}
	}
	if draw.ID == 0 {
		t.Fatal("no second turn-start draw for p1")
	}
	p, _ := draw.Decode()
	want := p.(*state.MovePayload).IID

	drawn := func(r *Result) state.IID {
		t.Helper()
		for _, id := range r.Events {
			if e := *g.log.Event(id); isDraw(e, state.P1) {
				p, _ := e.Decode()
				return p.(*state.MovePayload).IID
			}
		}
		t.Fatal("no draw")
		return ""
	}
	for i := 0; i < 5; i++ {
		if _, err := g.Rewind(draw.Parent, fmt.Sprintf("like-%d", i)); err != nil {
			t.Fatal(err)
		}
		if err := g.SetDrawsLikeMain(true); err != nil {
			t.Fatal(err)
		}
		if _, err := g.Shuffle(state.P1, state.ActorHuman, state.P1); err != nil {
			t.Fatal(err)
		}
		r, err := g.Draw(state.P1, state.ActorHuman, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := drawn(r); got != want {
			t.Errorf("branch %d drew %s, want main's %s", i, got, want)
		}
	}
	if b := g.Branches(); !slices.ContainsFunc(b, func(x BranchInfo) bool { return x.Name == "like-0" && x.DrawsLikeMain }) {
		t.Errorf("branches %+v do not show like-0 repeating main's draws", b)
	}

	// With main's card gone from the deck, the top card is drawn instead.
	if _, err := g.Rewind(draw.Parent, "gone"); err != nil {
		t.Fatal(err)
	}
	if err := g.SetDrawsLikeMain(true); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Move(state.P1, state.ActorHuman, want, state.Loc{Seat: state.P1, Zone: state.Discard}, "", ""); err != nil {
		t.Fatal(err)
	}
	top := g.State().Sides[state.P1].Deck[0]
	r, err := g.Draw(state.P1, state.ActorHuman, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := drawn(r); got != top {
		t.Errorf("drew %s, want the top card %s", got, top)
	}
	if !slices.ContainsFunc(r.Warnings, func(w string) bool { return strings.Contains(w, "not in p1's deck") }) {
		t.Errorf("no warning that main's card is gone; warnings %q", r.Warnings)
	}
}
