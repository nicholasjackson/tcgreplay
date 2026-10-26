package webui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// actingRefusal is the board's answer to a game action while the
// assistant acts.
const actingRefusal = "the opponent is acting; wait for them to finish"

// startPlay places both Actives and starts the game with p1 (the board's
// player) to move, in play mode.
func startPlay(t *testing.T, g *game.Game) {
	t.Helper()
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
}

// trainerInHand puts one of p1's Ultra Balls in p1's hand and returns it.
func trainerInHand(t *testing.T, g *game.Game) state.IID {
	t.Helper()
	st := g.State()
	side := st.Sides[state.P1]
	for _, iid := range append(append([]state.IID{}, side.Hand...), side.Deck...) {
		if st.Cards[iid].Def != "tst-3" {
			continue
		}
		if l, _ := st.Find(iid); l.Zone != state.Hand {
			if _, err := g.Move(state.P1, state.ActorSystem, iid, state.Loc{Zone: state.Hand}, "", ""); err != nil {
				t.Fatal(err)
			}
		}
		return iid
	}
	t.Fatal("p1 has no Ultra Ball")
	return ""
}

type waitDone struct {
	out game.WaitOutcome
	err error
}

// waitBG runs the assistant's turn wait in the background and returns once
// it is in flight (the board shows it waiting). Cancel ends it.
func waitBG(t *testing.T, m *game.Manager, req game.WaitRequest) (<-chan waitDone, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan waitDone, 1)
	go func() {
		out, err := m.Wait(ctx, req)
		done <- waitDone{out, err}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for m.Opponent() != "waiting" {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("the wait never showed as waiting")
		}
		time.Sleep(time.Millisecond)
	}
	return done, cancel
}

func waitOutcome(t *testing.T, done <-chan waitDone, kind string) game.WaitOutcome {
	t.Helper()
	select {
	case r := <-done:
		if r.err != nil || r.out.Kind != kind {
			t.Fatalf("wait got %+v, %v; want %s", r.out, r.err, kind)
		}
		return r.out
	case <-time.After(3 * time.Second):
		t.Fatalf("wait did not return %s", kind)
		return game.WaitOutcome{}
	}
}

// dataResult is a result with its data decoded as a map, so an absent key
// can be told from a false one.
type dataResult struct {
	resultMsg
	Data map[string]any `json:"data"`
}

func sendData(t *testing.T, c *websocket.Conn, id int, verb string, args map[string]any) dataResult {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"id": id, "verb": verb, "args": args})
	if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
	var r dataResult
	next(t, c, "result", &r)
	if r.ID != id {
		t.Fatalf("result id %d, want %d", r.ID, id)
	}
	return r
}

// lockByCard has p1 play a card under the assistant's wait, which reports
// it for an answer and locks the board. It returns the wait's outcome.
func lockByCard(t *testing.T, c *websocket.Conn, m *game.Manager, g *game.Game, id int) game.WaitOutcome {
	t.Helper()
	ball := trainerInHand(t, g)
	done, cancel := waitBG(t, m, game.WaitRequest{Seat: state.P1})
	defer cancel()
	if r := send(t, c, id, "play_trainer", map[string]any{"iid": string(ball)}); !r.OK {
		t.Fatalf("play_trainer: %s", r.Error)
	}
	out := waitOutcome(t, done, game.WaitRespond)
	if !m.Locked() {
		t.Fatal("the board is not locked while the assistant answers the card")
	}
	return out
}

// TestLockRefusesGameActions: while the assistant answers the player's
// card, the board refuses game actions, closing an action, checkout and
// rewind with the opponent-acting message and leaves the game alone, but
// allows the reveal toggle, replays and reads; switching mode is allowed
// and releases the lock.
func TestLockRefusesGameActions(t *testing.T) {
	ts, m := setup(t)
	m.Timing = game.Timing{WaitCap: 5 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
	g, _ := m.Current()
	startPlay(t, g)
	rewindTo := g.Head()
	if _, err := g.Rewind(g.Head(), "alt"); err != nil {
		t.Fatal(err)
	}
	if err := g.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	c := dial(t, ts)
	var st stateMsg
	next(t, c, "state", &st)

	lockByCard(t, c, m, g, 1)
	hand := len(g.State().Sides[state.P1].Hand)
	head := g.Head()

	id := 10
	for _, tt := range []struct {
		verb string
		args map[string]any
	}{
		{"draw", nil},
		{"move", map[string]any{"iid": "p1-1", "to": "discard"}},
		{"end_action", nil},
		{"end_turn", nil},
		{"checkout", map[string]any{"branch": "alt"}},
		{"rewind", map[string]any{"event": rewindTo}},
	} {
		id++
		r := send(t, c, id, tt.verb, tt.args)
		if r.OK || r.Error != actingRefusal {
			t.Errorf("%s while the assistant acts: %+v, want error %q", tt.verb, r, actingRefusal)
		}
	}
	if n := len(g.State().Sides[state.P1].Hand); n != hand {
		t.Errorf("refused actions changed p1's hand from %d to %d cards", hand, n)
	}
	if b := g.Branch(); b != "main" || g.Head() != head {
		t.Errorf("refused actions moved the board to %s@%d, was main@%d", b, g.Head(), head)
	}

	for _, tt := range []struct {
		verb string
		args map[string]any
	}{
		{"set_reveal", map[string]any{"reveal": false}},
		{"replay", nil},
		{"deck_list", nil},
		{"search_deck", nil},
		{"look_top", map[string]any{"n": 1}},
	} {
		id++
		if r := send(t, c, id, tt.verb, tt.args); !r.OK {
			t.Errorf("%s while the assistant acts: %s, want allowed", tt.verb, r.Error)
		}
	}
	if !m.Locked() {
		t.Fatal("looking at the board released the lock")
	}

	id++
	if r := send(t, c, id, "set_mode", map[string]any{"mode": "review"}); !r.OK {
		t.Fatalf("set_mode while the assistant acts: %s", r.Error)
	}
	if m.Locked() {
		t.Error("switching mode left the board locked")
	}
}

// TestLockReleasesAfterFinish: the player's actions work again in the same
// turn once the assistant waits again after answering, or once it ends its
// own turn.
func TestLockReleasesAfterFinish(t *testing.T) {
	t.Run("waits again", func(t *testing.T) {
		ts, m := setup(t)
		m.Timing = game.Timing{WaitCap: 5 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
		g, _ := m.Current()
		startPlay(t, g)
		c := dial(t, ts)
		var st stateMsg
		next(t, c, "state", &st)

		out := lockByCard(t, c, m, g, 1)
		if r := send(t, c, 2, "draw", nil); r.OK || r.Error != actingRefusal {
			t.Fatalf("draw while answering: %+v, want refused", r)
		}
		done, cancel := waitBG(t, m, game.WaitRequest{Seat: state.P1, Since: &out.Next})
		defer func() { cancel(); <-done }()
		if r := send(t, c, 3, "draw", nil); !r.OK {
			t.Fatalf("draw after the assistant waits again: %s", r.Error)
		}
		if r := send(t, c, 4, "end_action", nil); !r.OK {
			t.Fatalf("end_action after the assistant waits again: %s", r.Error)
		}
	})
	t.Run("ends its turn", func(t *testing.T) {
		ts, m := setup(t)
		m.Timing = game.Timing{WaitCap: 5 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
		g, _ := m.Current()
		startPlay(t, g)
		c := dial(t, ts)
		var st stateMsg
		next(t, c, "state", &st)

		done, cancel := waitBG(t, m, game.WaitRequest{Seat: state.P1})
		defer cancel()
		if r := send(t, c, 1, "end_turn", nil); !r.OK {
			t.Fatalf("end_turn: %s", r.Error)
		}
		waitOutcome(t, done, game.WaitTurnEnded)
		if r := send(t, c, 2, "end_action", nil); r.OK || r.Error != actingRefusal {
			t.Fatalf("end_action during the assistant's turn: %+v, want refused", r)
		}
		// The assistant plays and ends its turn.
		if _, err := g.Draw(state.P2, state.ActorClaude, 1); err != nil {
			t.Fatal(err)
		}
		m.AssistantMoved(true)
		if _, err := g.EndTurn(state.P2, state.ActorClaude); err != nil {
			t.Fatal(err)
		}
		m.AssistantDone()
		if r := send(t, c, 3, "draw", nil); !r.OK {
			t.Fatalf("draw on the player's next turn: %s", r.Error)
		}
	})
}

// opponentIs matches a state message whose opponent status is o ("" for
// none).
func opponentIs(o string) func(map[string]any) bool {
	return func(msg map[string]any) bool {
		v, present := msg["opponent"]
		if o == "" {
			return !present && msg["snapshot"] != nil
		}
		return v == o
	}
}

// TestOpponentStatusPushed: open boards are told the assistant is waiting
// while it waits, lose the status straight after a check-in, are told it
// is acting after a hand-off, and that it stopped once the lock released
// itself, after which the board works again.
func TestOpponentStatusPushed(t *testing.T) {
	ts, m := setup(t)
	m.Timing = game.Timing{WaitCap: 5 * time.Second, CheckIn: 150 * time.Millisecond, LockRelease: 300 * time.Millisecond}
	g, _ := m.Current()
	startPlay(t, g)
	c := dial(t, ts)
	if msg := rawState(t, c, hasSnapshot); msg["opponent"] != nil {
		t.Fatalf("first state has opponent %v, want none", msg["opponent"])
	}

	done, cancel := waitBG(t, m, game.WaitRequest{Seat: state.P1})
	defer cancel()
	rawState(t, c, opponentIs("waiting"))
	out := waitOutcome(t, done, game.WaitCheckIn)
	rawState(t, c, opponentIs(""))
	if o := m.Opponent(); o != "" {
		t.Fatalf("Opponent() %q straight after a check-in, want none", o)
	}

	done, cancel2 := waitBG(t, m, game.WaitRequest{Seat: state.P1, Since: &out.Next})
	defer cancel2()
	rawState(t, c, opponentIs("waiting"))
	if r := send(t, c, 1, "end_turn", nil); !r.OK {
		t.Fatalf("end_turn: %s", r.Error)
	}
	waitOutcome(t, done, game.WaitTurnEnded)
	rawState(t, c, opponentIs("acting"))
	rawState(t, c, opponentIs("stopped"))
	if m.Locked() {
		t.Fatal("still locked after the assistant stopped responding")
	}
	if r := send(t, c, 2, "end_action", nil); !r.OK {
		t.Fatalf("end_action after the lock released itself: %s", r.Error)
	}
}

// TestNoLockInReviewWithoutWait: in review mode with no turn handed over,
// the assistant's moves never lock the board and boards show no opponent
// status.
func TestNoLockInReviewWithoutWait(t *testing.T) {
	ts, m := setup(t)
	m.Timing = game.Timing{LockRelease: time.Minute}
	g, _ := m.Current()
	startPlay(t, g)
	if err := g.SetMode(game.ModeReview); err != nil {
		t.Fatal(err)
	}
	rewindTo := g.Head()
	c := dial(t, ts)
	noOpponent := func(when string) {
		t.Helper()
		msg := rawState(t, c, hasSnapshot)
		if o, present := msg["opponent"]; present {
			t.Errorf("%s: state has opponent %v, want none", when, o)
		}
	}
	noOpponent("first state")

	id := 0
	for _, tt := range []struct {
		verb string
		args map[string]any
	}{
		{"draw", nil},
		{"draw", map[string]any{"seat": "p2"}},
		{"end_action", nil},
		{"end_turn", nil},
		{"rewind", map[string]any{"event": rewindTo, "name": "line"}},
		{"checkout", map[string]any{"branch": "main"}},
	} {
		// The assistant's review moves (tool calls that are not its own
		// play-mode turn) come between the player's.
		m.AssistantMoved(false)
		if _, err := g.Draw(state.P2, state.ActorClaude, 1); err != nil {
			t.Fatal(err)
		}
		m.AssistantMoved(false)
		noOpponent("after the assistant's move")
		id++
		if r := send(t, c, id, tt.verb, tt.args); !r.OK {
			t.Errorf("%s in review: %s, want allowed", tt.verb, r.Error)
		}
		// Not every verb pushes a state (end_action with nothing open
		// does not), so check what the next push would carry.
		if m.Locked() || m.Opponent() != "" {
			t.Errorf("after %s: Locked() %v, Opponent() %q; want false and none", tt.verb, m.Locked(), m.Opponent())
		}
	}
	if m.Locked() || m.Opponent() != "" {
		t.Errorf("Locked() %v, Opponent() %q in review with no wait; want false and none", m.Locked(), m.Opponent())
	}
}

// TestEndTurnReportsWaiting: ending the turn on the board says the
// assistant is waiting when it is, alongside the prompt, and says nothing
// of waiting when it is not.
func TestEndTurnReportsWaiting(t *testing.T) {
	t.Run("waiting", func(t *testing.T) {
		ts, m := setup(t)
		m.Timing = game.Timing{WaitCap: 5 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
		g, _ := m.Current()
		startPlay(t, g)
		c := dial(t, ts)
		var st stateMsg
		next(t, c, "state", &st)
		done, cancel := waitBG(t, m, game.WaitRequest{Seat: state.P1})
		defer cancel()
		r := sendData(t, c, 1, "end_turn", nil)
		if !r.OK {
			t.Fatalf("end_turn: %s", r.Error)
		}
		if r.Data["waiting"] != true {
			t.Errorf("end_turn data %v, want waiting true", r.Data)
		}
		if p, _ := r.Data["prompt"].(string); !strings.Contains(p, "play turn 1 (p2's turn)") {
			t.Errorf("end_turn prompt %q, want the hand-off", p)
		}
		waitOutcome(t, done, game.WaitTurnEnded)
	})
	t.Run("not waiting", func(t *testing.T) {
		ts, m := setup(t)
		g, _ := m.Current()
		startPlay(t, g)
		c := dial(t, ts)
		var st stateMsg
		next(t, c, "state", &st)
		r := sendData(t, c, 1, "end_turn", nil)
		if !r.OK {
			t.Fatalf("end_turn: %s", r.Error)
		}
		if w, present := r.Data["waiting"]; present {
			t.Errorf("end_turn with no wait has waiting %v, want none", w)
		}
		if p, _ := r.Data["prompt"].(string); !strings.Contains(p, "play turn 1 (p2's turn)") {
			t.Errorf("end_turn prompt %q, want the hand-off", p)
		}
	})
}

// TestBoardActivityRecorded: every board request restarts the check-in
// clock, so a wait does not check in while the player keeps using the
// board, and does once they stop.
func TestBoardActivityRecorded(t *testing.T) {
	ts, m := setup(t)
	m.Timing = game.Timing{WaitCap: 600 * time.Millisecond, CheckIn: 150 * time.Millisecond}
	g, _ := m.Current()
	startPlay(t, g)
	c := dial(t, ts)
	var st stateMsg
	next(t, c, "state", &st)

	done, cancel := waitBG(t, m, game.WaitRequest{Seat: state.P1})
	defer cancel()
	tick := time.NewTicker(40 * time.Millisecond)
	defer tick.Stop()
	var got waitDone
	for id := 1; ; id++ {
		select {
		case got = <-done:
		case <-tick.C:
			send(t, c, id, "deck_list", nil)
			continue
		}
		break
	}
	if got.err != nil || got.out.Kind != game.WaitStillWaiting {
		t.Fatalf("wait with board requests every 40ms (check-in 150ms): %+v, %v; want still_waiting", got.out, got.err)
	}

	done, cancel2 := waitBG(t, m, game.WaitRequest{Seat: state.P1, Since: &got.out.Next})
	defer cancel2()
	waitOutcome(t, done, game.WaitCheckIn)
}
