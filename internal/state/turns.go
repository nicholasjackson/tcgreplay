package state

import "fmt"

// Turns are counted two ways. Stored and rule-bearing data (State.Turn,
// Event.Turn, Effect.UntilTurn, Stack.EnteredTurn) count one turn per
// player: 1, 2, 3, … alternating seats. People and the assistant count
// turns the way players do: turn N is the first player's Nth turn and the
// second player's Nth turn. Only numbers shown to them or taken from them
// go through these helpers; everything else keeps the per-player count.

// Round returns the combined turn for a per-player turn; setup stays 0.
func Round(turn int) int {
	if turn <= 0 {
		return 0
	}
	return (turn + 1) / 2
}

// TurnSeat returns the seat that plays per-player turn: the first player on
// odd turns, the other on even. It is "" for setup or before the first turn
// has started.
func (s *State) TurnSeat(turn int) Seat {
	if turn <= 0 || s.First == "" {
		return ""
	}
	if turn%2 == 1 {
		return s.First
	}
	return s.First.Other()
}

// TurnOf returns the per-player turn for seat's half of combined turn round.
func (s *State) TurnOf(round int, seat Seat) (int, error) {
	if s.First == "" {
		return 0, fmt.Errorf("the game has not started")
	}
	if round < 1 {
		return 0, fmt.Errorf("turn %d: turns start at 1", round)
	}
	if !seat.Valid() {
		return 0, fmt.Errorf("unknown seat %q: use p1 or p2", seat)
	}
	if seat == s.First {
		return 2*round - 1, nil
	}
	return 2 * round, nil
}
