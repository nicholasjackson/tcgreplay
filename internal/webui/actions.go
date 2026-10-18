package webui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// request is one UI interaction.
type request struct {
	ID   int    `json:"id"`
	Verb string `json:"verb"`
	Args args   `json:"args"`
}

// args is the union of every verb's arguments.
type args struct {
	Seat      string   `json:"seat,omitempty"`
	IID       string   `json:"iid,omitempty"`
	IIDs      []string `json:"iids,omitempty"`
	Target    string   `json:"target,omitempty"`
	To        string   `json:"to,omitempty"`
	Face      string   `json:"face,omitempty"`
	As        string   `json:"as,omitempty"`
	Slot      *int     `json:"slot,omitempty"`
	ToSlot    int      `json:"to_slot,omitempty"`
	N         int      `json:"n,omitempty"`
	Energy    []string `json:"energy,omitempty"`
	Name      string   `json:"name,omitempty"`
	Damage    int      `json:"damage,omitempty"`
	Amount    int      `json:"amount,omitempty"`
	Condition string   `json:"condition,omitempty"`
	On        bool     `json:"on,omitempty"`
	Whose     string   `json:"whose,omitempty"`
	First     string   `json:"first,omitempty"`
	Filter    string   `json:"filter,omitempty"`
	Mode      string   `json:"mode,omitempty"`
	Branch    string   `json:"branch,omitempty"`
	Event     int      `json:"event,omitempty"`
	From      int      `json:"from,omitempty"`
	Until     int      `json:"until,omitempty"`
	Text      string   `json:"text,omitempty"`
	UntilTurn int      `json:"until_turn,omitempty"`
	Reveal    bool     `json:"reveal,omitempty"`
}

// response answers one request.
type response struct {
	Type     string   `json:"type"`
	ID       int      `json:"id"`
	OK       bool     `json:"ok"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Delta    []string `json:"delta,omitempty"`
	Data     any      `json:"data,omitempty"`
	// refresh asks the hub to push state even though no events happened.
	refresh bool
}

const human = state.ActorHuman

func ids(xs []string) []state.IID {
	out := make([]state.IID, len(xs))
	for i, x := range xs {
		out[i] = state.IID(strings.TrimSpace(x))
	}
	return out
}

func slotOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// dispatch runs one request for client c.
func (s *Server) dispatch(ctx context.Context, c *client, req request) response {
	res := response{Type: "result", ID: req.ID}
	if s.Limit != nil {
		if err := s.Limit(ctx); err != nil {
			res.Error = err.Error()
			return res
		}
	}
	data, gr, err := s.do(ctx, c, req, &res)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	res.Data = data
	if gr != nil {
		res.Warnings, res.Delta = gr.Warnings, gr.Delta
		if res.Data == nil {
			res.Data = gr.Data
		}
	}
	return res
}

func (s *Server) do(ctx context.Context, c *client, req request, res *response) (any, *game.Result, error) {
	a := req.Args
	g, err := s.M.Current()
	if err != nil {
		return nil, nil, err
	}
	// The UI acts as the human seat; review mode may act for either.
	seat := s.Human
	if a.Seat != "" {
		o := state.Seat(a.Seat)
		if !o.Valid() {
			return nil, nil, fmt.Errorf("unknown seat %q", a.Seat)
		}
		if o != s.Human && g.Mode() != game.ModeReview {
			return nil, nil, fmt.Errorf("you play %s; switch to review mode to act for %s", s.Human, o)
		}
		seat = o
	}
	loc := func(spec string) (state.Loc, error) { return state.ParseLoc(spec, seat) }

	switch req.Verb {
	case "set_reveal":
		if a.Reveal && g.Mode() != game.ModeReview {
			return nil, nil, errors.New("the reveal view is only available in review mode")
		}
		c.mu.Lock()
		c.reveal = a.Reveal
		c.mu.Unlock()
		res.refresh = true
		return nil, nil, nil
	case "set_mode":
		if err := g.SetMode(game.Mode(a.Mode)); err != nil {
			return nil, nil, err
		}
		res.refresh = true
		return nil, nil, nil
	case "checkout":
		return nil, nil, g.Checkout(a.Branch)
	case "replay":
		viewer := s.Human
		c.mu.Lock()
		if c.reveal {
			viewer = ""
		}
		c.mu.Unlock()
		v, err := g.Replay(viewer, a.From, a.Until)
		return v, nil, err
	case "rewind":
		base := a.Name
		if base == "" {
			base = fmt.Sprintf("at-%d", a.Event)
		}
		name, err := g.JumpTo(a.Event, base)
		return name, nil, err
	case "search_deck":
		v, err := g.SearchDeck(seatOf(a.Whose, seat), a.Filter, "ui:search_deck", string(human))
		return v, nil, err
	case "deck_list":
		viewer := s.Human
		c.mu.Lock()
		if c.reveal {
			viewer = ""
		}
		c.mu.Unlock()
		v, err := g.DeckList(seatOf(a.Whose, seat), viewer, "ui:deck_list", string(human))
		return v, nil, err
	case "look_top":
		v, err := g.LookTop(seatOf(a.Whose, seat), max(a.N, 1), "ui:look_top", string(human))
		return v, nil, err
	case "end_action":
		return g.EndAction(), nil, nil
	case "annotate":
		return nil, nil, errors.New("annotations come from the assistant")
	}

	var r *game.Result
	switch req.Verb {
	case "draw":
		r, err = g.Draw(seat, human, max(a.N, 1))
	case "mulligan":
		r, err = g.Mulligan(seat, human)
	case "place_active":
		r, err = g.PlaceActive(seat, human, state.IID(a.IID))
	case "bench":
		r, err = g.Bench(seat, human, state.IID(a.IID), slotOr(a.Slot, -1))
	case "start_game":
		r, err = g.StartGame(state.Seat(a.First), human)
	case "evolve":
		var l state.Loc
		if l, err = loc(a.Target); err == nil {
			r, err = g.Evolve(seat, human, state.IID(a.IID), l)
		}
	case "attach":
		var l state.Loc
		if l, err = loc(a.Target); err == nil {
			r, err = g.Attach(seat, human, state.IID(a.IID), l, a.As)
		}
	case "play_trainer":
		r, err = g.PlayTrainer(seat, human, state.IID(a.IID))
	case "use_ability":
		var l state.Loc
		if l, err = loc(a.Target); err == nil {
			r, err = g.UseAbility(seat, human, l, a.Name)
		}
	case "retreat":
		r, err = g.Retreat(seat, human, a.ToSlot, ids(a.Energy))
	case "switch":
		r, err = g.Switch(seat, human, a.ToSlot)
	case "attack":
		target := a.Target
		if target == "" {
			target = "active"
		}
		var l state.Loc
		if l, err = state.ParseLoc(target, seat.Other()); err == nil {
			r, err = g.Attack(seat, human, a.Name, l, a.Damage)
		}
	case "knock_out":
		var l state.Loc
		if l, err = loc(a.Target); err == nil {
			r, err = g.KnockOut(seat, human, l)
		}
	case "take_prize":
		r, err = g.TakePrize(seat, human, slotOr(a.Slot, -1))
	case "set_condition":
		var l state.Loc
		if l, err = loc(a.Target); err == nil {
			r, err = g.SetCondition(seat, human, l, a.Condition, a.On)
		}
	case "place_damage":
		var l state.Loc
		if l, err = loc(a.Target); err == nil {
			r, err = g.PlaceDamage(seat, human, l, a.Amount)
		}
	case "shuffle":
		r, err = g.Shuffle(seat, human, seatOf(a.Whose, seat))
	case "coin_flip":
		r, err = g.CoinFlip(seat, human, max(a.N, 1))
	case "move":
		var l state.Loc
		if l, err = loc(a.To); err == nil {
			r, err = g.Move(seat, human, state.IID(a.IID), l, a.Face, a.As)
		}
	case "reveal":
		r, err = g.Reveal(seat, human, ids(a.IIDs))
	case "add_effect":
		r, err = g.AddEffect(seat, human, a.Text, seatOf(a.Whose, seat.Other()), state.IID(a.IID), a.UntilTurn)
	case "remove_effect":
		r, err = g.RemoveEffect(seat, human, a.Name)
	case "end_turn":
		since := g.TurnStartedAt(seat)
		if r, err = g.EndTurn(seat, human); err == nil && g.Mode() == game.ModePlay {
			// Hand the turn to Claude; in review either side is played here.
			st := g.State()
			r.Data = map[string]any{"prompt": fmt.Sprintf(
				"Your turn in tcgreplay (game %s, branch %s). Call get_log since %d and play turn %d.",
				g.ID, g.Branch(), since, st.Turn)}
		}
	default:
		return nil, nil, fmt.Errorf("unknown verb %q", req.Verb)
	}
	if err != nil {
		return nil, nil, err
	}
	return nil, r, nil
}
