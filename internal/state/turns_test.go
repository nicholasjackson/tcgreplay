package state

import "testing"

func TestRound(t *testing.T) {
	cases := []struct {
		turn, want int
	}{
		{-1, 0},
		{0, 0},
		{1, 1},
		{2, 1},
		{3, 2},
		{4, 2},
		{5, 3},
		{6, 3},
	}
	for _, c := range cases {
		if got := Round(c.turn); got != c.want {
			t.Errorf("Round(%d) = %d, want %d", c.turn, got, c.want)
		}
	}
}

func TestTurnSeat(t *testing.T) {
	cases := []struct {
		name  string
		first Seat
		turn  int
		want  Seat
	}{
		{"p1 first, setup", P1, 0, ""},
		{"p1 first, turn 1", P1, 1, P1},
		{"p1 first, turn 2", P1, 2, P2},
		{"p1 first, turn 3", P1, 3, P1},
		{"p1 first, turn 4", P1, 4, P2},
		{"p2 first, setup", P2, 0, ""},
		{"p2 first, turn 1", P2, 1, P2},
		{"p2 first, turn 2", P2, 2, P1},
		{"p2 first, turn 3", P2, 3, P2},
		{"p2 first, turn 4", P2, 4, P1},
		{"not started, turn 1", "", 1, ""},
		{"not started, turn 2", "", 2, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := New()
			s.First = c.first
			if got := s.TurnSeat(c.turn); got != c.want {
				t.Errorf("TurnSeat(%d) with First=%q = %q, want %q", c.turn, c.first, got, c.want)
			}
		})
	}
}

func TestTurnOf(t *testing.T) {
	cases := []struct {
		name  string
		first Seat
		round int
		seat  Seat
		want  int
	}{
		{"p1 first, round 1 p1", P1, 1, P1, 1},
		{"p1 first, round 1 p2", P1, 1, P2, 2},
		{"p1 first, round 2 p1", P1, 2, P1, 3},
		{"p1 first, round 2 p2", P1, 2, P2, 4},
		{"p1 first, round 3 p1", P1, 3, P1, 5},
		{"p1 first, round 3 p2", P1, 3, P2, 6},
		{"p2 first, round 1 p2", P2, 1, P2, 1},
		{"p2 first, round 1 p1", P2, 1, P1, 2},
		{"p2 first, round 2 p2", P2, 2, P2, 3},
		{"p2 first, round 2 p1", P2, 2, P1, 4},
		{"p2 first, round 3 p2", P2, 3, P2, 5},
		{"p2 first, round 3 p1", P2, 3, P1, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := New()
			s.First = c.first
			got, err := s.TurnOf(c.round, c.seat)
			if err != nil {
				t.Fatalf("TurnOf(%d, %q): unexpected error: %v", c.round, c.seat, err)
			}
			if got != c.want {
				t.Errorf("TurnOf(%d, %q) with First=%q = %d, want %d", c.round, c.seat, c.first, got, c.want)
			}
		})
	}
}

// TestTurnRoundTrip converts per-player turns to a combined turn and seat
// and back, expecting the original per-player turn.
func TestTurnRoundTrip(t *testing.T) {
	for _, first := range []Seat{P1, P2} {
		for turn := 1; turn <= 6; turn++ {
			s := New()
			s.First = first
			round, seat := Round(turn), s.TurnSeat(turn)
			got, err := s.TurnOf(round, seat)
			if err != nil {
				t.Fatalf("First=%q turn %d: TurnOf(%d, %q): unexpected error: %v", first, turn, round, seat, err)
			}
			if got != turn {
				t.Errorf("First=%q: turn %d -> (%d, %q) -> %d, want %d", first, turn, round, seat, got, turn)
			}
		}
	}
}

func TestTurnOfErrors(t *testing.T) {
	cases := []struct {
		name  string
		first Seat
		round int
		seat  Seat
	}{
		{"game not started", "", 1, P1},
		{"round 0", P1, 0, P1},
		{"negative round", P1, -1, P2},
		{"empty seat", P1, 1, ""},
		{"unknown seat", P1, 1, "p3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := New()
			s.First = c.first
			got, err := s.TurnOf(c.round, c.seat)
			if err == nil {
				t.Fatalf("TurnOf(%d, %q) with First=%q = %d, want an error", c.round, c.seat, c.first, got)
			}
			if got != 0 {
				t.Errorf("TurnOf(%d, %q) returned %d alongside an error, want 0", c.round, c.seat, got)
			}
		})
	}
}
