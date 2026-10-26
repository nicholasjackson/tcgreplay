package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// waitOut is wait_for_turn's result.
type waitOut struct {
	Outcome  string `json:"outcome"`
	Seat     string `json:"seat"`
	Since    int    `json:"since"`
	Turn     int    `json:"turn"`
	TurnSeat string `json:"turn_seat"`
	Prompt   string `json:"prompt"`
	Log      string `json:"log"`
	Cards    []struct {
		IID  string `json:"iid"`
		Name string `json:"name"`
		Card string `json:"card"`
		Kind string `json:"kind"`
		Text string `json:"text"`
	} `json:"cards"`
	Reason string `json:"reason"`
	Next   string `json:"next"`
}

// waitGame opens a play-mode game through new_game and starts it with p1
// (the player) to move.
func waitGame(t *testing.T, cs *mcp.ClientSession, m *game.Manager) *game.Game {
	t.Helper()
	deck, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Fatal(err)
	}
	if out, isErr := call(t, cs, "new_game", map[string]any{"deck_p1": string(deck), "deck_p2": string(deck), "id": "wait-1"}); isErr {
		t.Fatal(out)
	}
	g, err := m.Current()
	if err != nil {
		t.Fatal(err)
	}
	for _, seat := range state.Seats {
		iid := state.IID(string(seat) + "-1")
		if l, _ := g.State().Find(iid); l.Zone != state.Hand {
			g.Move(seat, state.ActorSystem, iid, state.Loc{Zone: state.Hand}, "", "")
		}
		if _, err := g.PlaceActive(seat, state.ActorSystem, iid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := g.StartGame(state.P1, state.ActorHuman); err != nil {
		t.Fatal(err)
	}
	return g
}

// waitCall calls wait_for_turn, returning its result or its error text.
func waitCall(cs *mcp.ClientSession, args map[string]any) (waitOut, string) {
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "wait_for_turn", Arguments: args})
	if err != nil {
		return waitOut{}, err.Error()
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	if res.IsError {
		return waitOut{}, b.String()
	}
	var w waitOut
	if err := json.Unmarshal([]byte(b.String()), &w); err != nil {
		return waitOut{}, fmt.Sprintf("%v: %s", err, b.String())
	}
	return w, ""
}

func mustWait(t *testing.T, cs *mcp.ClientSession, args map[string]any) waitOut {
	t.Helper()
	w, errText := waitCall(cs, args)
	if errText != "" {
		t.Fatalf("wait_for_turn %v: %s", args, errText)
	}
	return w
}

// TestWaitForTurnHandOff: after the assistant ends its turn, a wait on the
// player's seat returns as soon as the player ends theirs, with the new
// turn, the hand-off prompt, the player's moves in the view get_log uses
// and a next line saying to play.
func TestWaitForTurnHandOff(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		m.Timing = game.Timing{WaitCap: 5 * time.Second, CheckIn: time.Minute}
		g := waitGame(t, cs, m)
		if _, err := g.EndTurn(state.P1, state.ActorHuman); err != nil {
			t.Fatal(err)
		}
		if out, isErr := call(t, cs, "end_turn", nil); isErr {
			t.Fatal(out)
		}
		since := g.TurnStartedAt(state.P1)

		done := make(chan waitOut, 1)
		errs := make(chan string, 1)
		go func() {
			w, e := waitCall(cs, nil)
			done <- w
			errs <- e
		}()
		time.Sleep(100 * time.Millisecond)
		if _, err := g.Draw(state.P1, state.ActorHuman, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := g.EndTurn(state.P1, state.ActorHuman); err != nil {
			t.Fatal(err)
		}
		ended := time.Now()
		var w waitOut
		select {
		case w = <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("wait did not return after the player ended their turn")
		}
		if e := <-errs; e != "" {
			t.Fatal(e)
		}
		if d := time.Since(ended); d > time.Second {
			t.Errorf("wait returned %v after the turn ended", d)
		}
		wantPrompt := fmt.Sprintf("Your turn in tcgreplay (game wait-1, branch main). Call get_log since %d and play turn 2 (p2's turn).", since)
		if w.Outcome != "turn_ended" || w.Seat != "p1" || w.Turn != 2 || w.TurnSeat != "p2" || w.Prompt != wantPrompt {
			t.Fatalf("result: %+v", w)
		}
		if w.Since != g.Head() {
			t.Errorf("since %d, want the head %d", w.Since, g.Head())
		}
		if w.Next != wantPrompt+" Play your turn now." {
			t.Errorf("next: %q", w.Next)
		}
		logOut, isErr := call(t, cs, "get_log", map[string]any{"since": since})
		if isErr {
			t.Fatal(logOut)
		}
		if w.Log != logOut || !strings.Contains(w.Log, "Draw 1") {
			t.Errorf("log\n%s\nwant get_log's\n%s", w.Log, logOut)
		}
	})
}

// TestWaitForTurnSeatRules: play mode waits only on the player's seat and
// refuses a fresh wait during the assistant's own turn once it has moved;
// review mode waits on the named seat or the seat whose turn it is.
func TestWaitForTurnSeatRules(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		m.Timing = game.Timing{WaitCap: 50 * time.Millisecond, CheckIn: time.Minute}
		g := waitGame(t, cs, m)

		if _, e := waitCall(cs, map[string]any{"seat": "p2"}); !strings.Contains(e, "only on the player's seat, p1") {
			t.Errorf("play-mode wait on p2: %q", e)
		}
		if w := mustWait(t, cs, nil); w.Outcome != "still_waiting" || w.Seat != "p1" {
			t.Errorf("play-mode wait with no seat: %+v", w)
		}

		// The assistant's turn, once it has moved: nothing is pending.
		if _, err := g.EndTurn(state.P1, state.ActorHuman); err != nil {
			t.Fatal(err)
		}
		if out, isErr := call(t, cs, "draw", nil); isErr {
			t.Fatal(out)
		}
		if _, e := waitCall(cs, nil); !strings.Contains(e, "it is p2's turn: end_turn first") {
			t.Errorf("fresh wait during the assistant's turn: %q", e)
		}

		if out, isErr := call(t, cs, "set_mode", map[string]any{"mode": "review"}); isErr {
			t.Fatal(out)
		}
		if w := mustWait(t, cs, nil); w.Outcome != "still_waiting" || w.Seat != "p2" {
			t.Errorf("review wait with no seat during p2's turn: %+v", w)
		}
		if w := mustWait(t, cs, map[string]any{"seat": "p1"}); w.Outcome != "still_waiting" || w.Seat != "p1" {
			t.Errorf("review wait on p1: %+v", w)
		}
	})
}

// TestWaitForTurnStillWaiting: at the cap the result says to call again
// with the same cursor and post nothing.
func TestWaitForTurnStillWaiting(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		m.Timing = game.Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
		g := waitGame(t, cs, m)
		since := g.TurnStartedAt(state.P1)
		start := time.Now()
		w := mustWait(t, cs, nil)
		if d := time.Since(start); d > time.Second {
			t.Errorf("call took %v with a 100ms cap", d)
		}
		want := fmt.Sprintf("Call wait_for_turn again with since %d. Post nothing in chat.", since)
		if w.Outcome != "still_waiting" || w.Since != since || w.Next != want {
			t.Fatalf("result: %+v", w)
		}
		logOut, isErr := call(t, cs, "get_log", map[string]any{"since": since})
		if isErr {
			t.Fatal(logOut)
		}
		if w.Log != logOut || w.Log == "" {
			t.Errorf("still_waiting log\n%s\nwant get_log's\n%s", w.Log, logOut)
		}
		w = mustWait(t, cs, map[string]any{"since": w.Since})
		if w.Outcome != "still_waiting" || w.Since != since {
			t.Fatalf("continued: %+v", w)
		}
	})
}

// TestWaitForTurnCheckIn: with the player idle, the result asks for
// exactly "Still playing?" and how to carry on.
func TestWaitForTurnCheckIn(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		m.Timing = game.Timing{WaitCap: 5 * time.Second, CheckIn: 100 * time.Millisecond}
		g := waitGame(t, cs, m)
		since := g.TurnStartedAt(state.P1)
		w := mustWait(t, cs, nil)
		want := fmt.Sprintf("Ask the player exactly \"Still playing?\" and nothing else. When they reply, call wait_for_turn with since %d.", since)
		if w.Outcome != "check_in" || w.Since != since || w.Next != want {
			t.Fatalf("result: %+v", w)
		}
		if logOut, _ := call(t, cs, "get_log", map[string]any{"since": since}); w.Log != logOut {
			t.Errorf("check_in log\n%s\nwant get_log's\n%s", w.Log, logOut)
		}
	})
}

// TestWaitForTurnEnded: a game change ends the wait, naming the reason,
// saying to stop and carrying the player's moves while they are still on
// the board's line; a rewind leaves none to read.
func TestWaitForTurnEnded(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		m.Timing = game.Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
		g := waitGame(t, cs, m)
		since := g.TurnStartedAt(state.P1)
		w := mustWait(t, cs, nil)
		if _, err := g.Draw(state.P1, state.ActorHuman, 1); err != nil {
			t.Fatal(err)
		}
		if err := g.SetMode(game.ModeReview); err != nil {
			t.Fatal(err)
		}
		w = mustWait(t, cs, map[string]any{"since": w.Since})
		want := "Stop waiting: the game's mode was switched. Wait again only after you hand the player a turn."
		if w.Outcome != "ended" || w.Reason != "mode_changed" || w.Since != since || w.Next != want {
			t.Fatalf("result: %+v", w)
		}
		logOut, isErr := call(t, cs, "get_log", map[string]any{"since": since})
		if isErr {
			t.Fatal(logOut)
		}
		if w.Log != logOut || !strings.Contains(w.Log, "Draw 1") {
			t.Errorf("mode_changed log\n%s\nwant get_log's\n%s", w.Log, logOut)
		}

		// A wait on p1 in review, ended by a rewind to before its turn.
		w = mustWait(t, cs, map[string]any{"seat": "p1"})
		if _, err := g.Rewind(since-1, "before"); err != nil {
			t.Fatal(err)
		}
		w = mustWait(t, cs, map[string]any{"seat": "p1", "since": w.Since})
		if w.Outcome != "ended" || w.Reason != "rewound" || w.Log != "" ||
			w.Next != "Stop waiting: the board was rewound. Wait again only after you hand the player a turn." {
			t.Fatalf("rewound: %+v", w)
		}
	})
}

// TestWaitUsesQuietResolver: wait_for_turn finds its game through
// ResolveQuiet when one is set, and through Resolve otherwise; other tools
// keep using Resolve.
func TestWaitUsesQuietResolver(t *testing.T) {
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			m := game.NewManager(t.TempDir(), nil)
			var mu sync.Mutex
			loud, quiet := 0, 0
			counts := func() (int, int) {
				mu.Lock()
				defer mu.Unlock()
				return loud, quiet
			}
			resolve := func(_ context.Context, _ *mcp.RequestExtra) (*game.Manager, string, error) {
				mu.Lock()
				defer mu.Unlock()
				loud++
				return m, "", nil
			}
			resolveQuiet := func(_ context.Context, _ *mcp.RequestExtra) (*game.Manager, string, error) {
				mu.Lock()
				defer mu.Unlock()
				quiet++
				return m, "", nil
			}
			for _, withQuiet := range []bool{true, false} {
				s := &Server{Version: "test", Resolve: resolve}
				if withQuiet {
					s.ResolveQuiet = resolveQuiet
				}
				cs, err := mcp.NewClient(&mcp.Implementation{Name: "codex-mcp-client"}, nil).Connect(context.Background(), tr.dial(t, s.MCP()), nil)
				if err != nil {
					t.Fatal(err)
				}
				l0, q0 := counts()
				w := mustWait(t, cs, nil) // no game open: ends at once
				if w.Outcome != "ended" || w.Reason != "game_closed" {
					t.Errorf("wait with no game: %+v", w)
				}
				l1, q1 := counts()
				if withQuiet && (l1 != l0 || q1 != q0+1) {
					t.Errorf("with ResolveQuiet: Resolve called %d, ResolveQuiet %d times; want 0 and 1", l1-l0, q1-q0)
				}
				if !withQuiet && (l1 != l0+1 || q1 != q0) {
					t.Errorf("without ResolveQuiet: Resolve called %d, ResolveQuiet %d times; want 1 and 0", l1-l0, q1-q0)
				}
				call(t, cs, "ping", nil)
				if l2, q2 := counts(); l2 != l1+1 || q2 != q1 {
					t.Errorf("ping: Resolve called %d, ResolveQuiet %d times; want 1 and 0", l2-l1, q2-q1)
				}
				cs.Close()
			}
		})
	}
}

// TestInstructionsTeachTheLoop: the standing instructions name the wait
// and the check-in question, and tell the assistant never to ask for "your
// turn" or "done".
func TestInstructionsTeachTheLoop(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
		ins := cs.InitializeResult().Instructions
		for _, want := range []string{
			"After handing the player a turn",
			"call wait_for_turn",
			`Never ask the player to say "your turn" or "done"`,
			`on a check-in ask only "Still playing?"`,
		} {
			if !strings.Contains(ins, want) {
				t.Errorf("instructions lack %q", want)
			}
		}
		if n := strings.Count(ins, `"your turn"`); n != 1 {
			t.Errorf(`instructions mention "your turn" %d times, want only in the never-ask line`, n)
		}
	})
}

// TestWaitForTurnRespondAndPromote: a card the player uses comes back as
// respond with the card and a next line asking the assistant to judge its
// text and wait again; a knock-out of the assistant's Active comes back as
// promote, telling it to promote and wait again.
func TestWaitForTurnRespondAndPromote(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		m.Timing = game.Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute}
		g := waitGame(t, cs, m)
		st := g.State()
		card := st.Sides[state.P1].Hand[0]

		if _, err := g.PlayTrainer(state.P1, state.ActorHuman, card); err != nil {
			t.Fatal(err)
		}
		w := mustWait(t, cs, nil)
		if w.Outcome != "respond" || len(w.Cards) != 1 {
			t.Fatalf("after a card: %+v", w)
		}
		c := w.Cards[0]
		if c.IID != string(card) || c.Kind != "trainer" || c.Card != st.Cards[card].Def || c.Name != st.Name(card) {
			t.Errorf("reported card %+v, want %s (%s)", c, card, st.Cards[card].Def)
		}
		want := fmt.Sprintf("The player used the cards below. Read each card's text: if it asks your side (p2) to do something, do it now with your seat's verbs (pass seat p2 in review mode), then call wait_for_turn with since %d. If nothing, call it again straight away.", w.Since)
		if w.Next != want {
			t.Errorf("respond next: %q", w.Next)
		}

		if _, err := g.KnockOut(state.P1, state.ActorHuman, state.Loc{Seat: state.P2, Zone: state.Active}); err != nil {
			t.Fatal(err)
		}
		w = mustWait(t, cs, map[string]any{"since": w.Since})
		want = fmt.Sprintf("Your Active Pokémon was Knocked Out: promote with switch (to_slot) if you have a Benched Pokémon, then call wait_for_turn with since %d.", w.Since)
		if w.Outcome != "promote" || w.Next != want || len(w.Cards) != 0 {
			t.Fatalf("after the knock-out: %+v", w)
		}
	})
}
