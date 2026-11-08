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
	Seat string   `json:"seat,omitempty"`
	IID  string   `json:"iid,omitempty"`
	IIDs []string `json:"iids,omitempty"`
	// Pair is the second half of a Stadium played as a pair.
	Pair      string   `json:"pair_iid,omitempty"`
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
	// IgnorePrizes ends a turn with prize cards still owed (prize
	// reminders turned off on the board).
	IgnorePrizes bool   `json:"ignore_prizes,omitempty"`
	Filter       string `json:"filter,omitempty"`
	Mode         string `json:"mode,omitempty"`
	Branch       string `json:"branch,omitempty"`
	Event        int    `json:"event,omitempty"`
	From         int    `json:"from,omitempty"`
	Until        int    `json:"until,omitempty"`
	Text         string `json:"text,omitempty"`
	UntilTurn    int    `json:"until_turn,omitempty"`
	UntilSeat    string `json:"until_seat,omitempty"`
	Reveal       bool   `json:"reveal,omitempty"`
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
	// game is the game the request ran against, for redacting its delta.
	game *game.Game
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
	// Anything the player does on the board restarts the check-in clock.
	s.M.BoardActivity()
	data, gr, err := s.do(ctx, c, req, &res)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	res.Data = data
	if gr != nil {
		res.Warnings, res.Delta = gr.Warnings, s.boardDelta(c, res.game, gr)
		if res.Data == nil {
			res.Data = gr.Data
		}
	}
	return res
}

// boardDelta is a write's delta as the player may see it: redacted for the
// human seat, so it never names a card hidden from them (the opponent's
// hand and draws, including those an applied card makes it draw). With the
// reveal view on in review mode the board shows everything, and so does
// the delta. If redaction fails the delta is dropped rather than leaked.
func (s *Server) boardDelta(c *client, g *game.Game, gr *game.Result) []string {
	if g == nil {
		return nil
	}
	c.mu.Lock()
	reveal := c.reveal
	c.mu.Unlock()
	if reveal && g.Mode() == game.ModeReview {
		return gr.Delta
	}
	delta, err := g.DeltaFor(gr, s.Human)
	if err != nil {
		return nil
	}
	return delta
}

// lookVerbs are the board requests allowed while the assistant acts: reads,
// replays, the reveal toggle and switching mode (which also ends the
// assistant's wait).
var lookVerbs = map[string]bool{
	"set_reveal": true, "set_mode": true, "replay": true,
	"search_deck": true, "deck_list": true, "look_top": true,
}

var errOpponentActing = errors.New("the opponent is acting; wait for them to finish")

// decisionRefusals say what the board is waiting for when it holds an
// action that depends on the opponent's decision.
var decisionRefusals = map[string]error{
	game.DecidePromote:    errors.New("Waiting for your opponent to promote a new Active Pokémon; try again when they have."),
	game.DecideAnswerCard: errors.New("Waiting for your opponent to answer your card; try again when they have."),
}

// decisionVerbs are held under a decision hold whatever their arguments:
// they act on the result of the opponent's decision or move the board
// under it.
var decisionVerbs = map[string]bool{"attack": true, "end_turn": true, "checkout": true, "rewind": true}

// held is the board's one hold check, made before a request runs for
// seat. Under the assistant's full hold (its own turn) it refuses every
// request but looking and switching mode. Under a decision hold it
// refuses only requests that depend on the decision: attacking, ending
// the turn, moving the board back, anything aimed at the deciding side
// (acting as it, a target or destination on its side, one of its cards,
// a shuffle or effect on it), and a Trainer or Ability whose text names
// the opponent. Everything else goes through.
func (s *Server) held(g *game.Game, seat state.Seat, req request) error {
	switch s.M.Hold() {
	case game.HoldFull:
		if lookVerbs[req.Verb] {
			return nil
		}
		return errOpponentActing
	case game.HoldDecision:
	default:
		return nil
	}
	if lookVerbs[req.Verb] {
		return nil
	}
	refuse := decisionRefusals[s.M.Decision()]
	if refuse == nil {
		refuse = errOpponentActing
	}
	a := req.Args
	decider := s.M.Decider()
	if decider == "" {
		decider = seat.Other()
	}
	if decisionVerbs[req.Verb] || seat == decider {
		return refuse
	}
	for _, spec := range []string{a.Target, a.To} {
		if spec == "" {
			continue
		}
		if l, err := state.ParseLoc(spec, seat); err == nil && l.Seat == decider {
			return refuse
		}
	}
	st := g.State()
	for _, iid := range append([]string{a.IID, a.Pair}, a.IIDs...) {
		if c := st.Cards[state.IID(iid)]; c != nil && c.Owner == decider {
			return refuse
		}
	}
	switch req.Verb {
	case "shuffle":
		if seatOf(a.Whose, seat) == decider {
			return refuse
		}
	case "add_effect":
		if seatOf(a.Whose, seat.Other()) == decider {
			return refuse
		}
	case "play_trainer":
		if g.OpenerNamesOpponent(req.Verb, state.IID(a.IID), "") {
			return refuse
		}
	case "use_ability":
		l, err := state.ParseLoc(a.Target, seat)
		if err != nil {
			return nil // the verb reports the bad target
		}
		stack, err := st.Stack(l)
		if err != nil || stack.Top() == "" {
			return nil
		}
		if g.OpenerNamesOpponent(req.Verb, stack.Top(), a.Name) {
			return refuse
		}
	}
	return nil
}

func (s *Server) do(ctx context.Context, c *client, req request, res *response) (any, *game.Result, error) {
	a := req.Args
	g, err := s.M.Current()
	if err != nil {
		return nil, nil, err
	}
	res.game = g
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

	// While the assistant takes its turn, the player may look (reads,
	// replays, the reveal toggle) and switch mode, but not change the game
	// or the line; while it decides, only what depends on the decision is
	// held.
	if err := s.held(g, seat, req); err != nil {
		return nil, nil, err
	}

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
		// In play mode, going back is an undo on the game's own line: a
		// branch per step back only clutters the branch list. Review mode
		// keeps a branch per position, to explore lines side by side.
		if g.Mode() == game.ModePlay && a.Name == "" && g.OnCurrentBranch(a.Event) {
			return g.Branch(), nil, g.StepBackTo(a.Event)
		}
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
		id, warn := g.EndActionChecked()
		return id, &game.Result{Warnings: warn}, nil
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
		r, err = g.PlayTrainerWith(seat, human, state.IID(a.IID), state.IID(a.Pair))
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
		r, err = g.AddEffect(seat, human, a.Text, seatOf(a.Whose, seat.Other()), state.IID(a.IID), a.UntilTurn, state.Seat(a.UntilSeat))
	case "remove_effect":
		r, err = g.RemoveEffect(seat, human, a.Name)
	case "end_turn":
		since := g.TurnStartedAt(seat)
		waiting := s.M.Opponent() == "waiting"
		if r, err = g.EndTurnWith(seat, human, game.EndTurnOptions{IgnorePrizes: a.IgnorePrizes}); err == nil && g.Mode() == game.ModePlay {
			// Hand the turn to Claude; in review either side is played here.
			// A waiting assistant picks it up by itself, so the board need
			// not ask for the prompt to be pasted.
			data := map[string]any{"prompt": g.HandOff(since)}
			if waiting {
				data["waiting"] = true
			}
			r.Data = data
		}
	default:
		return nil, nil, fmt.Errorf("unknown verb %q", req.Verb)
	}
	if err != nil {
		return nil, nil, err
	}
	return nil, r, nil
}
