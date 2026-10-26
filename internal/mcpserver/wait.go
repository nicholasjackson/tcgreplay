package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// waitResult is what wait_for_turn returns: the outcome, the cursor to
// carry on from, and in next what to do now.
type waitResult struct {
	Outcome  string     `json:"outcome"`
	Seat     state.Seat `json:"seat"`
	Since    int        `json:"since"`
	Turn     int        `json:"turn,omitempty"`
	TurnSeat state.Seat `json:"turn_seat,omitempty"`
	Prompt   string     `json:"prompt,omitempty"`
	Log      string     `json:"log,omitempty"`
	Cards    []usedCard `json:"cards,omitempty"`
	Reason   string     `json:"reason,omitempty"`
	Next     string     `json:"next"`
}

// usedCard is a card, Ability or attack the player used, with its text for
// the assistant to judge.
type usedCard struct {
	IID  state.IID `json:"iid"`
	Name string    `json:"name"`
	Card string    `json:"card,omitempty"`
	Kind string    `json:"kind"`
	Text string    `json:"text"`
}

// endReasons says in words why a wait ended.
var endReasons = map[string]string{
	game.EndModeChanged:    "the game's mode was switched",
	game.EndGameReplaced:   "another game was opened",
	game.EndGameClosed:     "no game is open",
	game.EndGameExpired:    "the game expired",
	game.EndBranchSwitched: "the board switched to another branch",
	game.EndRewound:        "the board was rewound",
}

// waitForTurn runs the manager's turn wait on the seat handed to the
// player and renders its outcome.
func (s *Server) waitForTurn(ctx context.Context, in WaitArg) (any, error) {
	m := s.m(ctx)
	seat := state.Seat(strings.ToLower(in.Seat))
	if seat != "" && !seat.Valid() {
		return nil, fmt.Errorf("unknown seat %q", in.Seat)
	}
	g, err := m.Current()
	if err == nil {
		switch {
		case g.Mode() == game.ModePlay:
			if seat != "" && seat != s.HumanSeat {
				return nil, fmt.Errorf("in play mode you wait only on the player's seat, %s", s.HumanSeat)
			}
			seat = s.HumanSeat
		case seat == "":
			if seat = g.State().Current; seat == "" {
				return nil, fmt.Errorf("no turn is in progress: pass seat")
			}
		}
	} else if seat == "" {
		seat = s.HumanSeat
	}
	o, err := m.Wait(ctx, game.WaitRequest{Seat: seat, Since: in.Since})
	if err != nil {
		return nil, err
	}
	r := waitResult{Outcome: o.Kind, Seat: o.Seat, Since: o.Next, Reason: o.Reason}
	// The player's moves since the cursor, from the game waited on. Only a
	// wait ended by a branch switch or rewind has none: its moves are no
	// longer on the board's line.
	if g != nil && g.OnCurrentBranch(o.Since) {
		if r.Log, err = s.waitLog(g, o.Since); err != nil {
			return nil, err
		}
	}
	switch o.Kind {
	case game.WaitTurnEnded:
		r.Turn, r.TurnSeat, r.Prompt = o.Turn, o.TurnSeat, o.Prompt
		r.Next = o.Prompt + " Play your turn now."
	case game.WaitRespond:
		for _, c := range o.Cards {
			r.Cards = append(r.Cards, usedCard{IID: c.IID, Name: c.Name, Card: c.Def, Kind: c.Kind, Text: c.Text})
		}
		mine := o.Seat.Other()
		r.Next = fmt.Sprintf("The player used the cards below. Read each card's text: if it asks your side (%s) to do something, do it now with your seat's verbs (pass seat %s in review mode), then call wait_for_turn with since %d. If nothing, call it again straight away.", mine, mine, o.Next)
	case game.WaitPromote:
		r.Next = fmt.Sprintf("Your Active Pokémon was Knocked Out: promote with switch (to_slot) if you have a Benched Pokémon, then call wait_for_turn with since %d.", o.Next)
	case game.WaitCheckIn:
		r.Next = fmt.Sprintf("Ask the player exactly \"Still playing?\" and nothing else. When they reply, call wait_for_turn with since %d.", o.Next)
	case game.WaitEnded:
		reason := endReasons[o.Reason]
		if reason == "" {
			reason = o.Reason
		}
		r.Next = fmt.Sprintf("Stop waiting: %s. Wait again only after you hand the player a turn.", reason)
	default:
		r.Next = fmt.Sprintf("Call wait_for_turn again with since %d. Post nothing in chat.", o.Next)
	}
	return r, nil
}

// waitLog renders the log after since in the view get_log would use: the
// assistant's seat in play mode, everything in review mode.
func (s *Server) waitLog(g *game.Game, since int) (string, error) {
	view := ""
	if g.Mode() == game.ModeReview {
		view = "reveal"
	}
	viewer, err := s.viewer(g, view, "")
	if err != nil {
		return "", err
	}
	lines, err := g.Log(since, viewer, "wait_for_turn", string(actor))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.String())
		b.WriteByte('\n')
	}
	return b.String(), nil
}
