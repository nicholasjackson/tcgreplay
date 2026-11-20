package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	// Applied lists what the playmat did to the assistant's side as part
	// of the player's cards (Judge, Iono and the like), not reported before.
	Applied []appliedEffect `json:"applied,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	// KeepWaiting says whether the wait goes on after this result: false
	// only once the turn has ended or the wait was stopped.
	KeepWaiting bool `json:"keep_waiting"`
	// Action names the decision a decide outcome asks for: "promote" or
	// "answer_card"; Decision says it in one sentence, naming the seat.
	Action   string `json:"action,omitempty"`
	Decision string `json:"decision,omitempty"`
	// Choices are the Bench slots the assistant's seat may promote from.
	Choices []benchChoice `json:"choices,omitempty"`
	Wait    int           `json:"wait"`
	Waited  string        `json:"waited"`
	Next    string        `json:"next"`
}

// usedCard is a card, Ability or attack the player used, with its text for
// the assistant to judge.
type usedCard struct {
	IID  state.IID `json:"iid"`
	Name string    `json:"name"`
	Card string    `json:"card,omitempty"`
	Kind string    `json:"kind"`
	Text string    `json:"text"`
	// Need is "choose" (the card leaves your side a choice) or "read" (read
	// it and do what it asks of your side).
	Need string `json:"need"`
}

// benchChoice is a Benched Pokémon the assistant's seat may promote.
type benchChoice struct {
	Slot int       `json:"slot"`
	IID  state.IID `json:"iid"`
	Name string    `json:"name"`
}

// appliedEffect is an effect the playmat applied to the assistant's side:
// the card the player used and what happened.
type appliedEffect struct {
	IID  state.IID `json:"iid"`
	Name string    `json:"name"`
	Card string    `json:"card,omitempty"`
	What string    `json:"what"`
}

// endReasons says in words why a wait ended.
var endReasons = map[string]string{
	game.EndModeChanged:    "the game's mode was switched",
	game.EndGameReplaced:   "another game was opened",
	game.EndGameClosed:     "no game is open",
	game.EndGameExpired:    "the game expired",
	game.EndBranchSwitched: "the board switched to another branch",
	game.EndRewound:        "the board was rewound",
	game.EndGameOver:       "the game is over",
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
	waited := o.Waited.Round(time.Second).String()
	r := waitResult{Outcome: o.Kind, Seat: o.Seat, Since: o.Next, Reason: o.Reason, Wait: o.Polls, Waited: waited,
		KeepWaiting: o.Kind != game.WaitTurnEnded && o.Kind != game.WaitEnded}
	// The player's moves since the cursor, from the game waited on. Only a
	// wait ended by a branch switch or rewind has none: its moves are no
	// longer on the board's line.
	if g != nil && g.OnCurrentBranch(o.Since) {
		if r.Log, err = s.waitLog(g, o.Since); err != nil {
			return nil, err
		}
	}
	var appliedNames []string
	for _, a := range o.Applied {
		r.Applied = append(r.Applied, appliedEffect{IID: a.IID, Name: a.Name, Card: a.Def, What: a.What})
		appliedNames = append(appliedNames, a.Name)
	}
	switch o.Kind {
	case game.WaitTurnEnded:
		r.Turn, r.TurnSeat, r.Prompt = o.Turn, o.TurnSeat, o.Prompt
		r.Next = o.Prompt + " Play your turn now."
		if len(appliedNames) > 0 {
			r.Next += " The playmat applied its side of: " + strings.Join(appliedNames, ", ") + "; see applied."
		}
	case game.WaitDecide:
		mine := o.Seat.Other()
		r.Action = o.Action
		also := ""
		if len(appliedNames) > 0 {
			also = " The playmat has already applied its side of " + strings.Join(appliedNames, ", ") + " to your side (see applied); do not do that part again."
		}
		if o.Action == game.DecidePromote {
			var slots []string
			for _, c := range o.Choices {
				r.Choices = append(r.Choices, benchChoice{Slot: c.Slot, IID: c.IID, Name: c.Name})
				slots = append(slots, fmt.Sprint(c.Slot))
			}
			r.Decision = fmt.Sprintf("Your Active Pokémon (%s) was Knocked Out: choose a Benched Pokémon to promote.", mine)
			r.Next = fmt.Sprintf("Decision needed for your seat %[1]s. "+
				"1. Promote: call switch with seat %[1]s and to_slot set to one of the choices (slots %[2]s). "+
				"2. Do nothing else: do not take prizes, end the turn or play cards, it is still the player's turn. "+
				"3. Hand the turn back: call wait_for_turn with seat %[3]s and since %[4]d, in this same reply, and keep calling it until the turn ends. "+
				"The player keeps playing meanwhile.%[5]s Post nothing in chat.", mine, strings.Join(slots, ", "), o.Seat, o.Next, also)
			break
		}
		var names []string
		for _, c := range o.Cards {
			r.Cards = append(r.Cards, usedCard{IID: c.IID, Name: c.Name, Card: c.Def, Kind: c.Kind, Text: c.Text, Need: c.Need})
			names = append(names, c.Name)
		}
		r.Decision = fmt.Sprintf("The player used %s, which involves your side (%s): answer it.", strings.Join(names, ", "), mine)
		r.Next = fmt.Sprintf("Decision needed for your seat %[1]s: the player used the cards below, which involve your side. "+
			"1. For each card: if need is choose, make the choice it leaves you with your seat's verbs, passing seat %[1]s; if need is read, read its text and do exactly what it asks your side to do, or nothing if it asks nothing. "+
			"2. Do nothing else: do not end the turn or play cards of your own, it is still the player's turn. "+
			"3. Hand the turn back: call wait_for_turn with seat %[2]s and since %[3]d, in this same reply, and keep calling it until the turn ends. "+
			"The player keeps playing meanwhile.%[4]s Post nothing in chat.", mine, o.Seat, o.Next, also)
	case game.WaitCheckIn:
		r.Next = fmt.Sprintf("Ask the player exactly \"Still playing?\" and nothing else. When they reply, call wait_for_turn with since %d and keep waiting until the turn ends.", o.Next)
	case game.WaitEnded:
		reason := endReasons[o.Reason]
		if reason == "" {
			reason = o.Reason
		}
		r.Next = fmt.Sprintf("Stop waiting: %s. Wait again only after you hand the player a turn.", reason)
		if o.Reason == game.EndGameOver && g != nil {
			winner, why := g.State().Winner()
			r.Next = fmt.Sprintf("Stop waiting: the game is over, %s wins (%s). Tell the player the result and offer a review of the game; do not wait again.", winner, why)
		}
	default:
		r.Next = fmt.Sprintf("The player is still taking their turn (wait %d, %s so far). This is not the end of the wait, however many times you see it: call wait_for_turn again now with since %d, in this same reply, and keep calling it until the turn ends. Post nothing in chat.", o.Polls, waited, o.Next)
		if len(appliedNames) > 0 {
			r.Next += " The playmat applied its side of " + strings.Join(appliedNames, ", ") + " to your side (see applied)."
		}
	}
	if o.Kind == game.WaitEnded && len(appliedNames) > 0 {
		r.Next += " Before it ended, the playmat applied its side of " + strings.Join(appliedNames, ", ") + " to your side (see applied)."
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
