// Package mcpserver exposes the playmat to Claude as MCP tools.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// Server binds MCP tools to a game manager.
type Server struct {
	M *game.Manager
	// ClaudeSeat is the seat Claude plays in play mode.
	ClaudeSeat state.Seat
	// HumanSeat is the player's seat, used by the coach view.
	HumanSeat state.Seat
	Version   string
	// BoardURL is the board's address in hosted mode. When set, the results
	// of the game-opening tools carry it as a "board" link and the server
	// instructions ask the assistant to share it. Empty on the desktop.
	BoardURL string
}

const actor = state.ActorClaude

// MCP builds the MCP server with every tool registered.
func (s *Server) MCP() *mcp.Server {
	if s.ClaudeSeat == "" {
		s.ClaudeSeat = state.P2
	}
	if s.HumanSeat == "" {
		s.HumanSeat = s.ClaudeSeat.Other()
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "tcgreplay", Version: s.Version}, &mcp.ServerOptions{
		Instructions: s.instructions(),
		InitializedHandler: func(_ context.Context, req *mcp.InitializedRequest) {
			if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
				s.M.SetAssistant(AssistantName(p.ClientInfo.Name))
			}
		},
	})
	s.register(srv)
	return srv
}

const instructions = `Virtual Pokémon TCG playmat. It is a playmat, not a rules engine: you police legality, the server only checks that cards are where you say they are.

- Reference cards by iid (e.g. "p2-12"), never by name. get_state lists iids.
- Locations: "active", "bench:N" (0-based), "hand", "deck", "deck:top", "deck:bottom", "discard", "lost", "prizes:N", "stadium". Prefix a seat to target the other side: "p1:active".
- In play mode you act as your own seat and only on your turn (promoting after a knockout is allowed any time). In review mode either seat may act at any time (pass seat; it defaults to the seat whose turn it is), though only the turn in progress can be ended.
- play_trainer, use_ability and attack open a compound action: following verbs join it until end_action, another opener, or end_turn. attach, bench, evolve and retreat join only if the card's text mentions doing so; otherwise they stand alone and close it. Effects on the opponent's side (the switch of Boss's Orders, Iono's draws) join it when you pass the opponent's seat.
- Every write returns the events and a delta, so you rarely need to re-read the board. Warnings are advisory, never blocking.
- Quote card text from card_text, not memory.`

// boardInstruction is added to the instructions when results carry a board link.
const boardInstruction = "\n- Results of import_log, new_game and set_opponent_deck carry a board link: give it to the player so they can watch and act on the live board."

// instructions returns the server instructions, with the board-link line
// in hosted mode.
func (s *Server) instructions() string {
	if s.BoardURL == "" {
		return instructions
	}
	return instructions + boardInstruction
}

// withBoard adds the board link to a game-opening result in hosted mode.
func (s *Server) withBoard(res map[string]any) map[string]any {
	if s.BoardURL != "" {
		res["board"] = s.BoardURL
	}
	return res
}

// AssistantName turns an MCP client's name ("claude-code",
// "codex-mcp-client", "gemini-cli-mcp-client") into a name for the playmat.
func AssistantName(client string) string {
	c := strings.ToLower(client)
	for _, known := range []string{"Claude", "Codex", "Gemini"} {
		if strings.Contains(c, strings.ToLower(known)) {
			return known
		}
	}
	return client
}

// out renders v as indented JSON text.
func out(v any) (*mcp.CallToolResult, any, error) {
	if s, ok := v.(string); ok {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}, nil, nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

func add[In any](srv *mcp.Server, name, desc string, h func(context.Context, In) (any, error)) {
	mcp.AddTool(srv, &mcp.Tool{Name: name, Description: desc}, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		v, err := h(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return out(v)
	})
}

// SeatArg lets review mode choose the acting seat.
type SeatArg struct {
	Seat string `json:"seat,omitempty" jsonschema:"acting seat, review mode only; defaults to the seat whose turn it is"`
}

// acting resolves the seat a verb acts for.
func (s *Server) acting(g *game.Game, override string) (state.Seat, error) {
	o := state.Seat(strings.ToLower(override))
	if o != "" && !o.Valid() {
		return "", fmt.Errorf("unknown seat %q", override)
	}
	if g.Mode() == game.ModePlay {
		if o != "" && o != s.ClaudeSeat {
			return "", fmt.Errorf("in play mode you act only as %s; set_mode review to act for %s", s.ClaudeSeat, o)
		}
		return s.ClaudeSeat, nil
	}
	if o != "" {
		return o, nil
	}
	st := g.State()
	if st.Phase != state.PhasePlaying {
		return "", errors.New("during setup in review mode, pass seat")
	}
	return st.Current, nil
}

// verb runs a write for the acting seat.
func (s *Server) verb(seatArg string, f func(g *game.Game, seat state.Seat) (*game.Result, error)) (any, error) {
	g, err := s.M.Current()
	if err != nil {
		return nil, err
	}
	seat, err := s.acting(g, seatArg)
	if err != nil {
		return nil, err
	}
	return f(g, seat)
}

func parseLoc(spec string, def state.Seat) (state.Loc, error) {
	return state.ParseLoc(spec, def)
}

func iid(s string) state.IID { return state.IID(strings.TrimSpace(s)) }

// Inputs.

type (
	Empty   struct{}
	NewGame struct {
		DeckP1 string `json:"deck_p1" jsonschema:"p1 decklist in Pokémon TCG Live export format"`
		DeckP2 string `json:"deck_p2" jsonschema:"p2 decklist in Pokémon TCG Live export format"`
		NameP1 string `json:"name_p1,omitempty"`
		NameP2 string `json:"name_p2,omitempty"`
		ID     string `json:"id,omitempty" jsonschema:"game id; generated if empty"`
	}
	ImportLog struct {
		Log     string `json:"log" jsonschema:"the battle log text copied from Pokémon TCG Live, or a game record returned earlier by import_log, set_opponent_deck or get_record (pass it unchanged) to restore that exact game"`
		Deck    string `json:"deck,omitempty" jsonschema:"the log owner's decklist in TCG Live export format; omit to reuse the deck from the previous import; omit when log is a record"`
		OppDeck string `json:"opp_deck,omitempty" jsonschema:"optional assumed opponent decklist; fills cards the log never reveals, tagged as reference; omit when log is a record"`
		Me      string `json:"me,omitempty" jsonschema:"the log owner's player name; detected from the log if omitted; omit when log is a record"`
		ID      string `json:"id,omitempty" jsonschema:"game id; generated from the date and opponent if empty"`
	}
	Sample struct {
		Deck string `json:"deck,omitempty" jsonschema:"assumed list for the seat in TCG Live export format; it is reconciled with what the log proves and stored for later samples on any branch. Omit to reuse the last list given"`
		Seat string `json:"seat,omitempty" jsonschema:"whose hidden cards to sample; defaults to the player's opponent"`
		Seed uint64 `json:"seed,omitempty" jsonschema:"makes the sample reproducible; omit for a fresh one"`
	}
	OppDeck struct {
		Deck            string `json:"deck" jsonschema:"the assumed opponent list in TCG Live export format, 60 cards; set codes optional"`
		DiscardBranches bool   `json:"discard_branches,omitempty" jsonschema:"rebuild even though the game has branches, which are lost; only after the player agrees"`
	}
	Undo struct {
		N int `json:"n,omitempty" jsonschema:"moves to undo, default 1"`
	}
	ProfileArg struct {
		Seat string `json:"seat,omitempty" jsonschema:"whose list; defaults to the player's opponent"`
	}
	Mode struct {
		Mode string `json:"mode" jsonschema:"play or review"`
	}
	GetState struct {
		View string `json:"view,omitempty" jsonschema:"seat (your seat in play mode; any seat in review mode), coach (the player's seat: their hand plus the public board) or reveal (everything, review mode only). Default seat"`
		Seat string `json:"seat,omitempty" jsonschema:"for the seat view in review mode"`
	}
	GetLog struct {
		Since int    `json:"since,omitempty" jsonschema:"only events after this id"`
		View  string `json:"view,omitempty" jsonschema:"seat, coach or reveal; default seat in play mode, reveal in review mode"`
	}
	CardText struct {
		Card string `json:"card" jsonschema:"TCGdex card id (e.g. sv01-113) or a card iid (e.g. p2-12)"`
	}
	Draw struct {
		SeatArg
		N int `json:"n,omitempty" jsonschema:"cards to draw, default 1"`
	}
	IIDArg struct {
		SeatArg
		IID string `json:"iid"`
	}
	BenchArg struct {
		SeatArg
		IID  string `json:"iid"`
		Slot *int   `json:"slot,omitempty" jsonschema:"bench slot, 0-based; first empty slot if omitted"`
	}
	StartGame struct {
		First string `json:"first" jsonschema:"seat that goes first: p1 or p2"`
	}
	Evolve struct {
		SeatArg
		IID    string `json:"iid" jsonschema:"evolution card in hand"`
		Target string `json:"target" jsonschema:"active or bench:N"`
	}
	Attach struct {
		SeatArg
		IID    string `json:"iid"`
		Target string `json:"target" jsonschema:"active or bench:N"`
		As     string `json:"as,omitempty" jsonschema:"energy or tool; inferred from the card if omitted"`
	}
	Ability struct {
		SeatArg
		Slot string `json:"slot" jsonschema:"active or bench:N"`
		Name string `json:"name" jsonschema:"ability name"`
	}
	Retreat struct {
		SeatArg
		ToSlot int      `json:"to_slot" jsonschema:"bench slot of the new Active Pokémon"`
		Energy []string `json:"energy_iids,omitempty" jsonschema:"Energy iids to discard as the Retreat Cost"`
	}
	Switch struct {
		SeatArg
		ToSlot int `json:"to_slot" jsonschema:"bench slot of the new Active Pokémon"`
	}
	Attack struct {
		SeatArg
		Name   string `json:"name"`
		Target string `json:"target,omitempty" jsonschema:"defaults to the opponent's active; a bare location means the opponent's side, prefix your seat for your own"`
		Damage int    `json:"damage" jsonschema:"damage to put on the target after Weakness and Resistance"`
	}
	TargetArg struct {
		SeatArg
		Target string `json:"target" jsonschema:"[seat:]active or [seat:]bench:N; your own side if no seat prefix"`
	}
	TakePrize struct {
		SeatArg
		Slot *int `json:"slot,omitempty" jsonschema:"prize slot 0-5; first remaining if omitted"`
	}
	Condition struct {
		SeatArg
		Target    string `json:"target"`
		Condition string `json:"condition" jsonschema:"asleep, burned, confused, paralyzed, poisoned, or all (with clear)"`
		Clear     bool   `json:"clear,omitempty"`
	}
	Damage struct {
		SeatArg
		Target string `json:"target"`
		Amount int    `json:"amount" jsonschema:"damage to add (10 per counter); negative heals"`
	}
	Search struct {
		SeatArg
		Filter string `json:"filter,omitempty" jsonschema:"words matching name, category, stage, trainer type or Pokémon type, e.g. 'basic fighting'"`
		Whose  string `json:"whose,omitempty" jsonschema:"whose deck; default the acting seat"`
	}
	LookTop struct {
		SeatArg
		N     int    `json:"n"`
		Whose string `json:"whose,omitempty"`
	}
	Shuffle struct {
		SeatArg
		Whose string `json:"whose,omitempty"`
	}
	Coin struct {
		SeatArg
		N int `json:"n,omitempty"`
	}
	Move struct {
		SeatArg
		IID  string `json:"iid"`
		To   string `json:"to" jsonschema:"destination location; the card always stays on its owner's side"`
		Face string `json:"face,omitempty" jsonschema:"up or down"`
		As   string `json:"as,omitempty" jsonschema:"role in an occupied stack: pokemon, energy or tool"`
	}
	Reveal struct {
		SeatArg
		IIDs []string `json:"iids"`
	}
	AddEffect struct {
		SeatArg
		Text      string `json:"text"`
		On        string `json:"on,omitempty" jsonschema:"seat affected"`
		Source    string `json:"source,omitempty" jsonschema:"iid of the card causing it"`
		UntilTurn int    `json:"until_turn,omitempty" jsonschema:"expires at the end of this turn number; 0 means until removed"`
	}
	EffectID struct {
		SeatArg
		ID string `json:"id"`
	}
	Rewind struct {
		EventID int    `json:"event_id" jsonschema:"the new branch's head: state as it was right after this event"`
		Name    string `json:"name,omitempty" jsonschema:"branch name; generated if empty"`
	}
	Checkout struct {
		Branch string `json:"branch"`
	}
	Annotate struct {
		EventID int    `json:"event_id"`
		Text    string `json:"text"`
	}
)

func (s *Server) register(srv *mcp.Server) {
	add(srv, "ping", "Check the server is alive.", func(ctx context.Context, _ Empty) (any, error) {
		return "pong", nil
	})

	// Game management.
	add(srv, "new_game", "Start a new game from two TCG Live decklists. Loads, shuffles and draws opening hands; then place actives (place_active, bench), mulligan if needed, and start_game.", func(ctx context.Context, in NewGame) (any, error) {
		names := map[state.Seat]string{state.P1: in.NameP1, state.P2: in.NameP2}
		g, res, err := s.M.New(ctx, game.NewGameRequest{ID: in.ID, DeckP1: in.DeckP1, DeckP2: in.DeckP2, Names: names})
		if err != nil {
			return nil, err
		}
		if errs := g.PrefetchCards(ctx); len(errs) > 0 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%d card definitions could not be fetched", len(errs)))
		}
		return s.withBoard(map[string]any{"game": g.ID, "result": compact(res)}), nil
	})
	add(srv, "import_log", "Import a Pokémon TCG Live battle log as a new game in review mode, owner at p1 and opponent at p2. The owner's side is exact. Pass opp_deck, a likely 60-card list for the opponent's archetype, so both decks are complete: cards the log never names (prizes, hand, deck) are dealt at random from what the list has left. Without it they stay unknown; set_opponent_deck can add a list later. Returns the game summary, report.observed (the minimum count of each opponent card the log proves), problems and warnings. Also restores a game from its record: pass the record as log, alone. The result's record holds the log, both lists, the player's name and the seed: give it to the player to keep; importing it later rebuilds the identical game (branches and notes are not kept).", func(ctx context.Context, in ImportLog) (any, error) {
		res, err := s.M.Import(ctx, game.ImportRequest{ID: in.ID, Log: in.Log, Deck: in.Deck, OppDeck: in.OppDeck, Me: in.Me})
		if err != nil {
			return nil, err
		}
		g := res.Game
		warnings := res.Warnings
		if errs := g.PrefetchCards(ctx); len(errs) > 0 {
			warnings = append(warnings, fmt.Sprintf("%d card definitions could not be fetched", len(errs)))
		}
		return s.withBoard(map[string]any{"game": g.ID, "head": g.Head(), "mode": g.Mode(), "report": res.Report, "warnings": warnings, "record": res.Record}), nil
	})
	add(srv, "set_opponent_deck", "Rebuild the current imported game with an assumed opponent list: same log and seed, so the player's side is unchanged, and every opponent card the log never names is dealt at random from what the list has left. The list is reconciled with what the log proves (counts raised, missing cards added) and warnings say what changed. Replaces the game; refuses when it has branches unless discard_branches is set (ask the player first: the branches are lost). Returns the updated record.", func(ctx context.Context, in OppDeck) (any, error) {
		res, err := s.M.SetOpponentDeck(ctx, in.Deck, in.DiscardBranches)
		if err != nil {
			return nil, err
		}
		g := res.Game
		warnings := res.Warnings
		if errs := g.PrefetchCards(ctx); len(errs) > 0 {
			warnings = append(warnings, fmt.Sprintf("%d card definitions could not be fetched", len(errs)))
		}
		return s.withBoard(map[string]any{"game": g.ID, "head": g.Head(), "observed": res.Report.Observed, "problems": res.Report.Problems, "warnings": warnings, "record": res.Record}), nil
	})
	add(srv, "get_record", "The current game's record: the battle log, the player's name, both decklists and the seed, as JSON the player can keep in a note and later pass to import_log to restore the identical game. Games started with new_game have none.", func(ctx context.Context, _ Empty) (any, error) {
		rec, err := s.M.Record()
		if err != nil {
			return nil, err
		}
		return rec, nil
	})
	add(srv, "set_mode", "Switch between play mode (you act only as your seat; no reveal view) and review mode (act for either seat; reveal view allowed).", func(ctx context.Context, in Mode) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		if err := g.SetMode(game.Mode(in.Mode)); err != nil {
			return nil, err
		}
		return "mode " + in.Mode, nil
	})

	// Reading.
	add(srv, "get_state", "Read the board through a view. Hidden cards show only their iid.", func(ctx context.Context, in GetState) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		viewer, err := s.viewer(g, in.View, in.Seat)
		if err != nil {
			return nil, err
		}
		v, err := g.View(viewer, "get_state:"+viewName(in.View), string(actor))
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"game": g.ID, "branch": g.Branch(), "mode": g.Mode(), "open_action": g.OpenAction(),
			"board": v,
		}, nil
	})
	add(srv, "get_log", "Read events on the current branch as text lines, redacted for the view.", func(ctx context.Context, in GetLog) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		view := in.View
		if view == "" && g.Mode() == game.ModeReview {
			view = "reveal"
		}
		viewer, err := s.viewer(g, view, "")
		if err != nil {
			return nil, err
		}
		lines, err := g.Log(in.Since, viewer, "get_log:"+viewName(view), string(actor))
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, l := range lines {
			b.WriteString(l.String())
			b.WriteByte('\n')
		}
		if b.Len() == 0 {
			return "no events", nil
		}
		return b.String(), nil
	})
	add(srv, "get_warnings", "Invariant violations and soft rule warnings for the current turn.", func(ctx context.Context, _ Empty) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		w := g.Warnings()
		if len(w) == 0 {
			return "no warnings", nil
		}
		return w, nil
	})
	add(srv, "card_text", "Card text from the TCGdex cache. Quote this rather than memory.", func(ctx context.Context, in CardText) (any, error) {
		def := strings.TrimSpace(in.Card)
		if g, err := s.M.Current(); err == nil {
			if c, ok := g.State().Cards[state.IID(def)]; ok {
				if c.Def == "" {
					return nil, fmt.Errorf("%s is hidden or unresolved (%s)", def, c.Name)
				}
				def = c.Def
			}
		}
		if s.M.DB == nil {
			return nil, errors.New("no card database")
		}
		c, err := s.M.DB.Card(ctx, def)
		if err != nil {
			return nil, err
		}
		return c.Text(), nil
	})

	// Setup verbs.
	add(srv, "mulligan", "Shuffle your hand into your deck and draw a new hand of 7.", func(ctx context.Context, in SeatArg) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) { return g.Mulligan(seat, actor) })
	})
	add(srv, "place_active", "Put a Basic Pokémon from hand into your empty Active Spot.", func(ctx context.Context, in IIDArg) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			return g.PlaceActive(seat, actor, iid(in.IID))
		})
	})
	add(srv, "start_game", "Deal prizes to both seats and start turn 1; the first player draws.", func(ctx context.Context, in StartGame) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		return g.StartGame(state.Seat(strings.ToLower(in.First)), actor)
	})

	// Turn verbs.
	add(srv, "draw", "Draw n cards (default 1).", func(ctx context.Context, in Draw) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) { return g.Draw(seat, actor, max(in.N, 1)) })
	})
	add(srv, "bench", "Put a Pokémon from hand onto the bench.", func(ctx context.Context, in BenchArg) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			slot := -1
			if in.Slot != nil {
				slot = *in.Slot
			}
			return g.Bench(seat, actor, iid(in.IID), slot)
		})
	})
	add(srv, "evolve", "Evolve a Pokémon in play with a card from hand. Clears Special Conditions.", func(ctx context.Context, in Evolve) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			l, err := parseLoc(in.Target, seat)
			if err != nil {
				return nil, err
			}
			return g.Evolve(seat, actor, iid(in.IID), l)
		})
	})
	add(srv, "attach", "Attach an Energy or Tool from hand to one of your Pokémon.", func(ctx context.Context, in Attach) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			l, err := parseLoc(in.Target, seat)
			if err != nil {
				return nil, err
			}
			return g.Attach(seat, actor, iid(in.IID), l, in.As)
		})
	})
	add(srv, "play_trainer", "Play an Item, Supporter or Stadium from hand (Tools use attach). Opens a compound action; follow with the verbs that resolve it, then end_action.", func(ctx context.Context, in IIDArg) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			return g.PlayTrainer(seat, actor, iid(in.IID))
		})
	})
	add(srv, "use_ability", "Declare an Ability. Opens a compound action; follow with the verbs that resolve it, then end_action.", func(ctx context.Context, in Ability) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			l, err := parseLoc(in.Slot, seat)
			if err != nil {
				return nil, err
			}
			return g.UseAbility(seat, actor, l, in.Name)
		})
	})
	add(srv, "retreat", "Discard Energy for the Retreat Cost and switch the Active Pokémon with a benched one. Clears Special Conditions.", func(ctx context.Context, in Retreat) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			es := make([]state.IID, len(in.Energy))
			for i, e := range in.Energy {
				es[i] = iid(e)
			}
			return g.Retreat(seat, actor, in.ToSlot, es)
		})
	})
	add(srv, "switch", "Switch the Active Pokémon with a benched one without paying a cost (e.g. Switch, Boss's Orders on yourself), or promote into an empty Active Spot after a knockout.", func(ctx context.Context, in Switch) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) { return g.Switch(seat, actor, in.ToSlot) })
	})
	add(srv, "attack", "Declare an attack and put damage on the target. Opens a compound action for side effects; then knock_out, take_prize and end_turn as needed.", func(ctx context.Context, in Attack) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			target := in.Target
			if target == "" {
				target = "active"
			}
			l, err := parseLoc(target, seat.Other())
			if err != nil {
				return nil, err
			}
			return g.Attack(seat, actor, in.Name, l, in.Damage)
		})
	})
	add(srv, "knock_out", "Move every card of a Pokémon to its owner's discard pile.", func(ctx context.Context, in TargetArg) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			l, err := parseLoc(in.Target, seat)
			if err != nil {
				return nil, err
			}
			return g.KnockOut(seat, actor, l)
		})
	})
	add(srv, "take_prize", "Take one of your prize cards into hand.", func(ctx context.Context, in TakePrize) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			slot := -1
			if in.Slot != nil {
				slot = *in.Slot
			}
			return g.TakePrize(seat, actor, slot)
		})
	})
	add(srv, "set_condition", "Add or clear a Special Condition.", func(ctx context.Context, in Condition) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			l, err := parseLoc(in.Target, seat)
			if err != nil {
				return nil, err
			}
			return g.SetCondition(seat, actor, l, strings.ToLower(in.Condition), !in.Clear)
		})
	})
	add(srv, "place_damage", "Add damage to a Pokémon (negative heals), e.g. damage counters from an effect or Pokémon Checkup.", func(ctx context.Context, in Damage) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			l, err := parseLoc(in.Target, seat)
			if err != nil {
				return nil, err
			}
			return g.PlaceDamage(seat, actor, l, in.Amount)
		})
	})
	add(srv, "end_turn", "End your turn. The opponent's turn starts and they draw a card.", func(ctx context.Context, in SeatArg) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) { return g.EndTurn(seat, actor) })
	})
	add(srv, "end_action", "Close the open compound action.", func(ctx context.Context, _ Empty) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		if id := g.EndAction(); id != "" {
			return "closed " + id, nil
		}
		return "no open action", nil
	})

	// Deck and randomness.
	add(srv, "search_deck", "List a deck's cards sorted by category and name (never in true order). Then move or bench what you take, and shuffle.", func(ctx context.Context, in Search) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		seat, err := s.acting(g, in.Seat)
		if err != nil {
			return nil, err
		}
		whose := seat
		if in.Whose != "" {
			whose = state.Seat(strings.ToLower(in.Whose))
		}
		return g.SearchDeck(whose, in.Filter, "search_deck", string(actor))
	})
	add(srv, "look_top", "Look at the top n cards of a deck, top first.", func(ctx context.Context, in LookTop) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		seat, err := s.acting(g, in.Seat)
		if err != nil {
			return nil, err
		}
		whose := seat
		if in.Whose != "" {
			whose = state.Seat(strings.ToLower(in.Whose))
		}
		return g.LookTop(whose, in.N, "look_top", string(actor))
	})
	add(srv, "shuffle", "Shuffle a deck (yours by default).", func(ctx context.Context, in Shuffle) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			whose := seat
			if in.Whose != "" {
				whose = state.Seat(strings.ToLower(in.Whose))
			}
			return g.Shuffle(seat, actor, whose)
		})
	})
	add(srv, "coin_flip", "Flip n coins (default 1). Results are recorded.", func(ctx context.Context, in Coin) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) { return g.CoinFlip(seat, actor, in.N) })
	})

	// Escape hatches.
	add(srv, "move", "Escape hatch: move any card to any location on its owner's side.", func(ctx context.Context, in Move) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			l, err := parseLoc(in.To, seat)
			if err != nil {
				return nil, err
			}
			return g.Move(seat, actor, iid(in.IID), l, in.Face, in.As)
		})
	})
	add(srv, "reveal", "Reveal cards to both players.", func(ctx context.Context, in Reveal) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			ids := make([]state.IID, len(in.IIDs))
			for i, x := range in.IIDs {
				ids[i] = iid(x)
			}
			return g.Reveal(seat, actor, ids)
		})
	})
	add(srv, "add_effect", "Record a lingering effect, e.g. 'Items locked' on p1 until the end of turn 5.", func(ctx context.Context, in AddEffect) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) {
			on := seat.Other()
			if in.On != "" {
				on = state.Seat(strings.ToLower(in.On))
			}
			return g.AddEffect(seat, actor, in.Text, on, iid(in.Source), in.UntilTurn)
		})
	})
	add(srv, "remove_effect", "Remove a lingering effect.", func(ctx context.Context, in EffectID) (any, error) {
		return s.verb(in.Seat, func(g *game.Game, seat state.Seat) (*game.Result, error) { return g.RemoveEffect(seat, actor, in.ID) })
	})

	// Branches.
	add(srv, "list_branches", "List branches with their head events.", func(ctx context.Context, _ Empty) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		return g.Branches(), nil
	})
	add(srv, "rewind", "Create a new branch whose head is event_id and check it out. Nothing is deleted; the old branch keeps its head.", func(ctx context.Context, in Rewind) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		name, err := g.Rewind(in.EventID, in.Name)
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("on branch %s at event %d", name, in.EventID), nil
	})
	add(srv, "undo", "Step the current branch back over its last n moves (default 1); its head moves to just before them. The events stay in the log. Only moves made on the playmat can be undone, never the imported game itself.", func(ctx context.Context, in Undo) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		head, err := g.Undo(in.N)
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("branch %s now at event %d", g.Branch(), head), nil
	})
	add(srv, "checkout", "Switch to an existing branch.", func(ctx context.Context, in Checkout) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		if err := g.Checkout(in.Branch); err != nil {
			return nil, err
		}
		return fmt.Sprintf("on branch %s at event %d", in.Branch, g.Head()), nil
	})

	// Opponent inference.
	add(srv, "get_profile", "What is known and assumed about a seat's list: the stored assumed list (with the minimum counts the whole log proves) and seen_so_far, the most copies of each card the player had seen at once by the current head. Use it to propose a list for sample_hidden.", func(ctx context.Context, in ProfileArg) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		seat, err := s.otherSeat(in.Seat)
		if err != nil {
			return nil, err
		}
		return g.Profile(seat)
	})
	add(srv, "sample_hidden", "Give the opponent's hidden cards (hand, deck order, prizes) plausible identities at the head of the current branch, to test a line against what they might hold. Cards the player can see keep their identities, and cards they saw that are now hidden stay out of the prizes. Rewind first, then sample; to try several hands, rewind to the same event again for each. Review mode only. Returns the sampled hand and prizes.", func(ctx context.Context, in Sample) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		seat, err := s.otherSeat(in.Seat)
		if err != nil {
			return nil, err
		}
		list, warnings, err := s.M.ParseDeck(ctx, in.Deck, "deck")
		if err != nil {
			return nil, err
		}
		res, err := g.SampleHidden(seat, actor, list, in.Seed)
		if err != nil {
			return nil, err
		}
		res.Warnings = append(warnings, res.Warnings...)
		return res, nil
	})

	// Coaching.
	add(srv, "annotate", "Attach a coaching note to an event; it shows on the timeline and as a toast.", func(ctx context.Context, in Annotate) (any, error) {
		g, err := s.M.Current()
		if err != nil {
			return nil, err
		}
		if err := g.Annotate(in.EventID, string(actor), in.Text); err != nil {
			return nil, err
		}
		return "noted", nil
	})
}

// otherSeat parses a seat argument, defaulting to the seat opposite the player.
func (s *Server) otherSeat(arg string) (state.Seat, error) {
	if arg == "" {
		return s.HumanSeat.Other(), nil
	}
	seat := state.Seat(strings.ToLower(arg))
	if !seat.Valid() {
		return "", fmt.Errorf("unknown seat %q", arg)
	}
	return seat, nil
}

func viewName(v string) string {
	if v == "" {
		return "seat"
	}
	return v
}

// viewer maps a view name to the seat whose view it is ("" for reveal).
func (s *Server) viewer(g *game.Game, view, seatArg string) (state.Seat, error) {
	switch strings.ToLower(view) {
	case "", "seat":
		if g.Mode() == game.ModeReview && seatArg != "" {
			seat := state.Seat(strings.ToLower(seatArg))
			if !seat.Valid() {
				return "", fmt.Errorf("unknown seat %q", seatArg)
			}
			return seat, nil
		}
		if seatArg != "" && state.Seat(strings.ToLower(seatArg)) != s.ClaudeSeat {
			return "", fmt.Errorf("in play mode the seat view is %s's; use view coach for %s", s.ClaudeSeat, s.HumanSeat)
		}
		return s.ClaudeSeat, nil
	case "coach":
		return s.HumanSeat, nil
	case "reveal":
		if g.Mode() != game.ModeReview {
			return "", errors.New("the reveal view is only available in review mode")
		}
		return "", nil
	}
	return "", fmt.Errorf("unknown view %q", view)
}

// compact trims a result for setup, whose delta lists 100+ events.
func compact(r *game.Result) any {
	if len(r.Delta) <= 30 {
		return r
	}
	return map[string]any{
		"action_id": r.ActionID,
		"events":    fmt.Sprintf("%d..%d", r.Events[0], r.Events[len(r.Events)-1]),
		"delta":     fmt.Sprintf("%d events (use get_state to see the board)", len(r.Delta)),
		"warnings":  r.Warnings,
	}
}
