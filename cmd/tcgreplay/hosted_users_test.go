package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/usergames"
)

// The two accounts the per-account tests sign in.
var (
	userA = hostauthtest.Account{ID: "sub-a", Email: "a@example.com"}
	userB = hostauthtest.Account{ID: "sub-b", Email: "b@example.com"}
)

// Two corpus games played with the test deck, one for each account.
const (
	testLogA = testLogPath                                         // NicJackson against LeDjo23
	testLogB = "../../testdata/logs/2026-10-11_lukiees_mirror.txt" // NicJackson against Lukiees
)

// signInAs signs a connector and a browser in as acct and returns the MCP
// session and the board cookie.
func (h *hostedServer) signInAs(t *testing.T, acct hostauthtest.Account) (*mcp.ClientSession, *http.Cookie) {
	t.Helper()
	h.g.SetAccount(acct)
	return h.connect(t), h.cookie(t)
}

// stateGame returns the id of the game get_state reads over cs.
func stateGame(t *testing.T, cs *mcp.ClientSession) string {
	t.Helper()
	var st struct {
		Game string `json:"game"`
	}
	out := callTool(t, cs, "get_state", nil)
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("get_state: %v: %s", err, out)
	}
	return st.Game
}

// boardSnapshot dials h's board with cookie from the public address and
// returns the live connection and its first snapshot.
func boardSnapshot(t *testing.T, h *hostedServer, cookie *http.Cookie) (*websocket.Conn, *game.UISnapshot) {
	t.Helper()
	c, _, err := dialBoard(t, h.ts, testPublic, cookie)
	if err != nil {
		t.Fatalf("board from %s: %v", testPublic, err)
	}
	// An imported game's snapshot is bigger than the default read limit.
	c.SetReadLimit(-1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var st boardState
	if err := readBoard(ctx, c, "state", &st); err != nil {
		t.Fatalf("initial board state: %v", err)
	}
	if st.Snapshot == nil {
		t.Fatal("initial board state has no snapshot")
	}
	return c, st.Snapshot
}

// boardFeed reads every state message from c in the background until the
// test ends. Reading with a deadline would close the connection, so the
// reads have no deadline and the test waits on the channel instead.
func boardFeed(t *testing.T, c *websocket.Conn) <-chan *game.UISnapshot {
	t.Helper()
	c.SetReadLimit(-1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch := make(chan *game.UISnapshot, 64)
	go func() {
		defer close(ch)
		for {
			var st boardState
			if err := readBoard(ctx, c, "state", &st); err != nil {
				return
			}
			select {
			case ch <- st.Snapshot:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// waitFor waits up to within for a snapshot on feed that ok accepts and
// returns it, failing the test otherwise.
func waitFor(t *testing.T, feed <-chan *game.UISnapshot, within time.Duration, what string, ok func(*game.UISnapshot) bool) *game.UISnapshot {
	t.Helper()
	timeout := time.After(within)
	for {
		select {
		case snap, open := <-feed:
			if !open {
				t.Fatalf("board connection closed waiting for %s", what)
			}
			if snap != nil && ok(snap) {
				return snap
			}
		case <-timeout:
			t.Fatalf("no board update with %s within %v", what, within)
		}
	}
}

// quiet drains feed until nothing has arrived for gap.
func quiet(feed <-chan *game.UISnapshot, gap time.Duration) {
	for {
		select {
		case <-feed:
		case <-time.After(gap):
			return
		}
	}
}

// firstHand returns the first card in seat's hand on snap's board.
func firstHand(t *testing.T, snap *game.UISnapshot, seat state.Seat) state.IID {
	t.Helper()
	hand := snap.Board.Sides[seat].Hand
	if len(hand) == 0 {
		t.Fatalf("%s has no cards in hand on the board", seat)
	}
	return hand[0].IID
}

// importFor imports the corpus log at logPath with the test deck over cs as
// game id, with the owner's name in the log changed to me if set, and
// returns the result.
func importFor(t *testing.T, cs *mcp.ClientSession, logPath, id, me string) map[string]any {
	t.Helper()
	logText := readTestdata(t, logPath)
	if me != "" {
		logText = strings.ReplaceAll(logText, "NicJackson", me)
	}
	return callObject(t, cs, "import_log", map[string]any{"log": logText, "deck": readTestdata(t, testDeckPath), "id": id})
}

// TestHostedIsolation: two accounts each import a different game; each
// reads only its own game over MCP and on its own board, and a write by one
// leaves the other's game exactly as it was.
func TestHostedIsolation(t *testing.T) {
	h := startHosted(t, testPublic)
	csA, cookieA := h.signInAs(t, userA)
	csB, cookieB := h.signInAs(t, userB)

	importFor(t, csA, testLogA, "game-a", "")
	importFor(t, csB, testLogB, "game-b", "")

	if got := stateGame(t, csA); got != "game-a" {
		t.Errorf("A's get_state reads game %q, want game-a", got)
	}
	if got := stateGame(t, csB); got != "game-b" {
		t.Errorf("B's get_state reads game %q, want game-b", got)
	}
	cA, snapA := boardSnapshot(t, h, cookieA)
	_, snapB := boardSnapshot(t, h, cookieB)
	if snapA.Game != "game-a" {
		t.Errorf("A's board shows game %q, want game-a", snapA.Game)
	}
	if snapB.Game != "game-b" {
		t.Errorf("B's board shows game %q, want game-b", snapB.Game)
	}
	if opp := snapA.Board.Sides[state.P2].Name; opp != "LeDjo23" {
		t.Errorf("A's board names the opponent %q, want LeDjo23 from A's import", opp)
	}
	if opp := snapB.Board.Sides[state.P2].Name; opp != "Lukiees" {
		t.Errorf("B's board names the opponent %q, want Lukiees from B's import", opp)
	}

	// A write by A leaves B's game unchanged.
	beforeB := callTool(t, csB, "get_state", map[string]any{"view": "reveal"})
	gB, err := h.manager(userB).Current()
	if err != nil {
		t.Fatal(err)
	}
	headB := gB.Head()
	gA, err := h.manager(userA).Current()
	if err != nil {
		t.Fatal(err)
	}
	headA := gA.Head()

	feedA := boardFeed(t, cA)
	iid := firstHand(t, snapA, state.P1)
	callTool(t, csA, "move", map[string]any{"seat": "p1", "iid": string(iid), "to": "discard"})
	waitFor(t, feedA, 2*time.Second, string(iid)+" in A's discard", func(s *game.UISnapshot) bool {
		return hasCard(s.Board.Sides[state.P1].Discard, iid)
	})
	if gA, _ := h.manager(userA).Current(); gA.Head() == headA {
		t.Errorf("A's head did not move after A's move (still %d)", headA)
	}

	if after := callTool(t, csB, "get_state", map[string]any{"view": "reveal"}); after != beforeB {
		t.Errorf("B's game changed after A's move:\nbefore %s\nafter  %s", beforeB, after)
	}
	if gB, _ := h.manager(userB).Current(); gB.ID != "game-b" || gB.Head() != headB {
		t.Errorf("B's game is %s at head %d after A's move, want game-b at %d", gB.ID, gB.Head(), headB)
	}
	if got := stateGame(t, csB); got != "game-b" {
		t.Errorf("B's get_state reads game %q after A's move, want game-b", got)
	}
}

// TestHostedSameAccountSessions: two connector sessions for one account
// share its game: a write over one shows in the other's next read and on
// the account's board, and a game opened over one replaces the game both
// sessions and the board see.
func TestHostedSameAccountSessions(t *testing.T) {
	h := startHosted(t, testPublic)
	h.g.SetAccount(userA)
	one, two, cookie := h.connect(t), h.connect(t), h.cookie(t)

	c, snap := openGameOn(t, h, one, cookie)
	if got := stateGame(t, two); got != "live-1" {
		t.Fatalf("second session reads game %q, want live-1 opened by the first", got)
	}
	feed := boardFeed(t, c)

	iid := firstHand(t, snap, state.P1)
	callTool(t, one, "move", map[string]any{"seat": "p1", "iid": string(iid), "to": "discard"})
	if !hasCard(claudeState(t, two).Sides[state.P1].Discard, iid) {
		t.Errorf("second session does not see %s in p1's discard after the first session's move", iid)
	}
	waitFor(t, feed, 2*time.Second, string(iid)+" in p1's discard", func(s *game.UISnapshot) bool {
		return hasCard(s.Board.Sides[state.P1].Discard, iid)
	})

	importFor(t, two, testLogA, "replaced", "")
	if got := stateGame(t, one); got != "replaced" {
		t.Errorf("first session reads game %q after the second's import, want replaced", got)
	}
	if got := stateGame(t, two); got != "replaced" {
		t.Errorf("second session reads game %q after its import, want replaced", got)
	}
	waitFor(t, feed, 2*time.Second, "game replaced", func(s *game.UISnapshot) bool { return s.Game == "replaced" })
}

// TestHostedBoardLinkOwnerOnly: an account's board link names its own
// board; another signed-in account opening it gets the "not your game"
// page and nothing of the game, and a signed-out browser is sent to sign
// in with nothing of the game either.
func TestHostedBoardLinkOwnerOnly(t *testing.T) {
	h := startHosted(t, testPublic)
	csA, cookieA := h.signInAs(t, userA)
	_, cookieB := h.signInAs(t, userB)

	res := importFor(t, csA, testLogA, "owner-only", "")
	handle := usergames.Handle(userA.ID)
	link, _ := res["board"].(string)
	if want := h.public + "/?board=" + handle; link != want {
		t.Fatalf("A's import_log board link %q, want %s", link, want)
	}
	if handle == usergames.Handle(userB.ID) {
		t.Fatal("A and B have the same board handle")
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	path := u.RequestURI()

	// The game's identifiers and card data that must not leak.
	leaks := []string{"owner-only", "LeDjo23", "NicJackson", "Lucario", `"snapshot"`, `"board"`, "<title>tcgreplay</title>", userA.Email}

	resp, body := getWith(t, h.ts.URL+path, cookieB, nil)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "Not your game") {
		t.Fatalf("B opening A's board link: %d %q, want 403 not your game", resp.StatusCode, body)
	}
	for _, leak := range leaks {
		if strings.Contains(body, leak) {
			t.Errorf("B's not-your-game page contains %q:\n%s", leak, body)
		}
	}

	resp, body = getWith(t, h.ts.URL+path, nil, nil)
	if want := "/auth/login?next=" + url.QueryEscape(path); resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != want {
		t.Errorf("A's board link signed out: %d to %q, want 302 to %s", resp.StatusCode, resp.Header.Get("Location"), want)
	}
	for _, leak := range leaks {
		if strings.Contains(body, leak) {
			t.Errorf("signed-out response contains %q:\n%s", leak, body)
		}
	}

	// The owner gets the board.
	if resp, body := getWith(t, h.ts.URL+path, cookieA, nil); resp.StatusCode != http.StatusOK || !strings.Contains(body, "<title>tcgreplay</title>") {
		t.Errorf("A opening A's board link: %d %q, want the board", resp.StatusCode, body)
	}
	// B's own board shows no game, not A's.
	cB, _, err := dialBoard(t, h.ts, testPublic, cookieB)
	if err != nil {
		t.Fatalf("B's board: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var st boardState
	if err := readBoard(ctx, cB, "state", &st); err != nil {
		t.Fatalf("B's board state: %v", err)
	}
	if st.Snapshot != nil && st.Snapshot.Game != "" {
		t.Errorf("B's board shows game %q, want none", st.Snapshot.Game)
	}
}

// TestHostedLiveUpdatesScoped: an account's open board gets its own
// account's writes within 2 seconds and nothing from another account's.
func TestHostedLiveUpdatesScoped(t *testing.T) {
	h := startHosted(t, testPublic)
	csA, cookieA := h.signInAs(t, userA)
	csB, cookieB := h.signInAs(t, userB)

	cA, snapA := openGameOn(t, h, csA, cookieA)
	cB, snapB := openGameOn(t, h, csB, cookieB)
	feedA, feedB := boardFeed(t, cA), boardFeed(t, cB)

	iidA := firstHand(t, snapA, state.P1)
	start := time.Now()
	callTool(t, csA, "move", map[string]any{"seat": "p1", "iid": string(iidA), "to": "discard"})
	waitFor(t, feedA, 2*time.Second, "A's move on A's board", func(s *game.UISnapshot) bool {
		return hasCard(s.Board.Sides[state.P1].Discard, iidA)
	})
	t.Logf("A's move reached A's board in %v", time.Since(start))
	quiet(feedA, 200*time.Millisecond)

	iidB := firstHand(t, snapB, state.P1)
	callTool(t, csB, "move", map[string]any{"seat": "p1", "iid": string(iidB), "to": "discard"})
	waitFor(t, feedB, 2*time.Second, "B's move on B's board", func(s *game.UISnapshot) bool {
		return hasCard(s.Board.Sides[state.P1].Discard, iidB)
	})
	select {
	case snap, open := <-feedA:
		if open {
			t.Fatalf("A's board got an update after B's move: game %s head %d", snap.Game, snap.Head)
		}
		t.Fatal("A's board connection closed after B's move")
	case <-time.After(500 * time.Millisecond):
	}
}

// TestHostedOwnName: each account's board names the player's side from
// that account's own import.
func TestHostedOwnName(t *testing.T) {
	h := startHosted(t, testPublic)
	csA, cookieA := h.signInAs(t, userA)
	csB, cookieB := h.signInAs(t, userB)

	importFor(t, csA, testLogA, "name-a", "")
	importFor(t, csB, testLogA, "name-b", "BeaPlays")

	_, snapA := boardSnapshot(t, h, cookieA)
	_, snapB := boardSnapshot(t, h, cookieB)
	if got := snapA.Board.Sides[state.P1].Name; got != "NicJackson" {
		t.Errorf("A's board names the player %q, want NicJackson from A's import", got)
	}
	if got := snapB.Board.Sides[state.P1].Name; got != "BeaPlays" {
		t.Errorf("B's board names the player %q, want BeaPlays from B's import", got)
	}
}

// TestHostedSessionOwnAccount: with two accounts signed in, each board
// cookie's /auth/session answers that account's own email and never the
// other's, so the name the board falls back to is the viewer's own.
func TestHostedSessionOwnAccount(t *testing.T) {
	h := startHosted(t, testPublic)
	_, cookieA := h.signInAs(t, userA)
	_, cookieB := h.signInAs(t, userB)

	for _, tc := range []struct {
		name        string
		cookie      *http.Cookie
		own, others string
	}{
		{"A", cookieA, userA.Email, userB.Email},
		{"B", cookieB, userB.Email, userA.Email},
	} {
		resp, body := getWith(t, h.ts.URL+"/auth/session", tc.cookie, nil)
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"email":"`+tc.own+`"`) {
			t.Errorf("%s's /auth/session: %d %q, want 200 with %s", tc.name, resp.StatusCode, body, tc.own)
		}
		if strings.Contains(body, tc.others) {
			t.Errorf("%s's /auth/session names the other account %s: %q", tc.name, tc.others, body)
		}
	}
}
