package state

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// pairState is a game in setup with 60 cards a side. p1 holds two halves
// of a pair (p1-1, p1-2) and a single Stadium (p1-3) in hand; p2 holds one
// half (p2-1). The state reads no card data, so names are only labels.
func pairState(t *testing.T) *State {
	t.Helper()
	var cards []Card
	for _, seat := range Seats {
		for i := 1; i <= DeckSize; i++ {
			name := "Filler"
			switch i {
			case 1, 2:
				name = "Twin Peak"
			case 3:
				name = "Arena"
			}
			cards = append(cards, Card{IID: IID(fmt.Sprintf("%s-%d", seat, i)), Def: fmt.Sprintf("tst-%d", i), Name: name, Owner: seat})
		}
	}
	s := New()
	apply(t, s, EvSetup, SetupPayload{BenchSize: 5, PrizeCount: 6, Cards: cards})
	for _, iid := range []IID{"p1-1", "p1-2", "p1-3", "p2-1"} {
		apply(t, s, EvMove, MovePayload{IID: iid, To: Loc{Zone: Hand}})
	}
	return s
}

func event(t *testing.T, s *State, typ EventType, payload any) *Event {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return &Event{ID: s.Head + 1, Parent: s.Head, Type: typ, Payload: b}
}

func apply(t *testing.T, s *State, typ EventType, payload any) {
	t.Helper()
	if err := s.Apply(event(t, s, typ, payload)); err != nil {
		t.Fatal(err)
	}
}

// tryMove applies a move to a clone and returns the result and the error.
func tryMove(t *testing.T, s *State, p MovePayload) (*State, error) {
	t.Helper()
	c := s.Clone()
	err := c.Apply(event(t, c, EvMove, p))
	return c, err
}

func clean(t *testing.T, s *State) {
	t.Helper()
	if problems := s.Check(); len(problems) > 0 {
		t.Errorf("invariants: %v", problems)
	}
}

func TestStadiumPairPlacedInOneEvent(t *testing.T) {
	s, err := tryMove(t, pairState(t), MovePayload{IID: "p1-1", Pair: "p1-2", To: Loc{Zone: Stadium}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Stadium, []IID{"p1-1", "p1-2"}) {
		t.Errorf("stadium = %v, want [p1-1 p1-2]", s.Stadium)
	}
	for _, iid := range []IID{"p1-1", "p1-2"} {
		if l, err := s.Find(iid); err != nil || l.Zone != Stadium {
			t.Errorf("Find(%s) = %v, %v; want the Stadium spot", iid, l, err)
		}
		if slices.Contains(s.Sides[P1].Hand, iid) {
			t.Errorf("%s is still in the hand", iid)
		}
	}
	clean(t, s)
}

func TestStadiumPairRole(t *testing.T) {
	tests := []struct {
		name    string
		spot    []IID // played singly or as a pair first
		move    MovePayload
		wantErr string
		want    []IID
	}{
		{"joins a lone card", []IID{"p1-1"}, MovePayload{IID: "p1-2", To: Loc{Zone: Stadium}, As: AsPair}, "", []IID{"p1-1", "p1-2"}},
		{"refused onto an empty spot", nil, MovePayload{IID: "p1-2", To: Loc{Zone: Stadium}, As: AsPair}, "only when it holds one other card", nil},
		{"refused onto a pair", []IID{"p1-1", "p1-2"}, MovePayload{IID: "p1-3", To: Loc{Zone: Stadium}, As: AsPair}, "only when it holds one other card", nil},
		{"plain move into an occupied spot refused", []IID{"p1-1"}, MovePayload{IID: "p1-3", To: Loc{Zone: Stadium}}, "stadium is occupied by p1-1", nil},
		{"plain move onto a pair refused", []IID{"p1-1", "p1-2"}, MovePayload{IID: "p1-3", To: Loc{Zone: Stadium}}, "stadium is occupied by p1-1", nil},
		{"pair into an occupied spot refused", []IID{"p1-3"}, MovePayload{IID: "p1-1", Pair: "p1-2", To: Loc{Zone: Stadium}}, "stadium is occupied by p1-3", nil},
		{"stack role ignored, as before", nil, MovePayload{IID: "p1-3", To: Loc{Zone: Stadium}, As: AsEnergy}, "", []IID{"p1-3"}},
		{"stack role into an occupied spot refused", []IID{"p1-1"}, MovePayload{IID: "p1-3", To: Loc{Zone: Stadium}, As: AsTool}, "stadium is occupied by p1-1", nil},
		{"plain move into an empty spot", nil, MovePayload{IID: "p1-3", To: Loc{Zone: Stadium}}, "", []IID{"p1-3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := pairState(t)
			switch len(tt.spot) {
			case 1:
				apply(t, s, EvMove, MovePayload{IID: tt.spot[0], To: Loc{Zone: Stadium}})
			case 2:
				apply(t, s, EvMove, MovePayload{IID: tt.spot[0], Pair: tt.spot[1], To: Loc{Zone: Stadium}})
			}
			got, err := tryMove(t, s, tt.move)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got.Stadium, tt.want) {
				t.Errorf("stadium = %v, want %v", got.Stadium, tt.want)
			}
			clean(t, got)
		})
	}
}

// A pair moved out of the spot puts each half on its owner's side, even
// when the halves have different owners.
func TestStadiumPairMovedOut(t *testing.T) {
	s := pairState(t)
	apply(t, s, EvMove, MovePayload{IID: "p1-1", To: Loc{Zone: Stadium}})
	apply(t, s, EvMove, MovePayload{IID: "p2-1", To: Loc{Zone: Stadium}, As: AsPair})
	got, err := tryMove(t, s, MovePayload{IID: "p1-1", Pair: "p2-1", From: &Loc{Zone: Stadium}, To: Loc{Seat: P1, Zone: Discard}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Stadium) != 0 {
		t.Errorf("stadium = %v, want empty", got.Stadium)
	}
	if !slices.Contains(got.Sides[P1].Discard, "p1-1") || !slices.Contains(got.Sides[P2].Discard, "p2-1") {
		t.Errorf("discards p1 %v, p2 %v; want each half in its owner's", got.Sides[P1].Discard, got.Sides[P2].Discard)
	}
	clean(t, got)

	// Back to the hand, too.
	got, err = tryMove(t, s, MovePayload{IID: "p2-1", Pair: "p1-1", To: Loc{Zone: Hand}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Sides[P1].Hand, "p1-1") || !slices.Contains(got.Sides[P2].Hand, "p2-1") {
		t.Errorf("hands p1 %v, p2 %v; want each half in its owner's", got.Sides[P1].Hand, got.Sides[P2].Hand)
	}
	clean(t, got)
}

func TestStadiumPairMoveRefusals(t *testing.T) {
	s := pairState(t)
	apply(t, s, EvMove, MovePayload{IID: "p1-10", To: Loc{Seat: P1, Zone: Active}})
	tests := []struct {
		name    string
		move    MovePayload
		wantErr string
	}{
		{"into a stack", MovePayload{IID: "p1-1", Pair: "p1-2", To: Loc{Seat: P1, Zone: Active}}, "a pair moves only"},
		{"with a role", MovePayload{IID: "p1-1", Pair: "p1-2", To: Loc{Zone: Stadium}, As: AsPair}, "takes no pair role"},
		{"into a bench slot", MovePayload{IID: "p1-1", Pair: "p1-2", To: Loc{Seat: P1, Zone: Bench, Slot: 0}}, "a pair moves only"},
		{"into a prize slot", MovePayload{IID: "p1-1", Pair: "p1-2", To: Loc{Seat: P1, Zone: Prizes, Slot: 0}}, "a pair moves only"},
		{"halves in different places", MovePayload{IID: "p1-1", Pair: "p1-20", To: Loc{Zone: Stadium}}, "a pair moves from one place"},
		{"its own pair", MovePayload{IID: "p1-1", Pair: "p1-1", To: Loc{Zone: Stadium}}, "its own pair"},
		{"unknown pair", MovePayload{IID: "p1-1", Pair: "p9-1", To: Loc{Zone: Stadium}}, "no card p9-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tryMove(t, s, tt.move); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

// The board view shows one Stadium with its second half nested in it;
// readers of stadium.iid and stadium.name still see the first half.
func TestStadiumPairView(t *testing.T) {
	s := pairState(t)
	apply(t, s, EvMove, MovePayload{IID: "p1-3", To: Loc{Zone: Stadium}})
	single := s.Project(ViewReveal, "", nil)
	if single.Stadium == nil || single.Stadium.IID != "p1-3" || single.Stadium.Pair != nil {
		t.Fatalf("single stadium view = %+v", single.Stadium)
	}
	b, _ := json.Marshal(single.Stadium)
	if got := string(b); got != `{"iid":"p1-3","name":"Arena","def":"tst-3"}` {
		t.Errorf("single stadium JSON = %s", got)
	}

	apply(t, s, EvMove, MovePayload{IID: "p1-3", To: Loc{Seat: P1, Zone: Discard}})
	apply(t, s, EvMove, MovePayload{IID: "p1-1", Pair: "p1-2", To: Loc{Zone: Stadium}})
	bv := s.Project(ViewSeat, P2, nil)
	if bv.Stadium == nil || bv.Stadium.IID != "p1-1" || bv.Stadium.Name != "Twin Peak" || bv.Stadium.Pair == nil || bv.Stadium.Pair.IID != "p1-2" {
		t.Fatalf("pair stadium view = %+v", bv.Stadium)
	}
	b, _ = json.Marshal(bv.Stadium)
	if got := string(b); got != `{"iid":"p1-1","name":"Twin Peak","def":"tst-1","pair":{"iid":"p1-2","name":"Twin Peak","def":"tst-2"}}` {
		t.Errorf("pair stadium JSON = %s", got)
	}
}

// A move event without the new parts decodes and folds as before.
func TestMoveWithoutPairUnchanged(t *testing.T) {
	var p MovePayload
	if err := json.Unmarshal([]byte(`{"iid":"p1-3","to":{"zone":"stadium"}}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.Pair != "" || p.As != "" {
		t.Fatalf("decoded %+v", p)
	}
	b, _ := json.Marshal(MovePayload{IID: "p1-3", To: Loc{Zone: Stadium}})
	if strings.Contains(string(b), "pair") {
		t.Errorf("a single move encodes a pair field: %s", b)
	}
	s, err := tryMove(t, pairState(t), p)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Stadium, []IID{"p1-3"}) {
		t.Errorf("stadium = %v", s.Stadium)
	}
}
