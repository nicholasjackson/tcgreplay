package usergames

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
)

// accountHeader names the test account a request is signed in as; the
// test front door stands in for hostauth.RequireSession.
const accountHeader = "X-Test-Account"

// lockedClock is a settable clock safe to read from server goroutines.
type lockedClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *lockedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *lockedClock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
	return c.t
}

// boardSite serves reg.Board() behind a stand-in for the session check
// that signs requests in by the accountHeader ("alice" or "bob"). Board
// verbs count as activity through reg.BoardLimit, as in hosted mode.
func boardSite(t *testing.T, clock *lockedClock) (*Registry, *httptest.Server) {
	t.Helper()
	db := newFakeDB()
	dir := t.TempDir()
	public, _ := url.Parse("https://playmat.example.com")
	var reg *Registry
	reg = New(Config{
		NewManager: func() *game.Manager { return game.NewManager(dir, db) },
		NewBoard: func(m *game.Manager) *webui.Server {
			ui := webui.New(m, state.P1)
			ui.Public = public
			ui.Limit = reg.BoardLimit(nil)
			return ui
		},
		Public: public,
		Now:    clock.Now,
	})
	board := reg.Board()
	accounts := map[string]hostauth.Account{"alice": alice, "bob": bob}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if acct, ok := accounts[r.Header.Get(accountHeader)]; ok {
			r = r.WithContext(hostauth.WithAccount(r.Context(), acct))
		}
		board.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	// Runs before ts.Close: end every entry, so its hub and live
	// connections stop.
	t.Cleanup(func() {
		reg.mu.Lock()
		defer reg.mu.Unlock()
		for _, e := range reg.entries {
			e.stop()
		}
	})
	return reg, ts
}

func newClock() *lockedClock {
	return &lockedClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
}

// get loads path on ts signed in as who ("" for nobody).
func get(t *testing.T, ts *httptest.Server, who, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if who != "" {
		req.Header.Set(accountHeader, who)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// liveConn is an open board connection whose messages are read in the
// background, so a test can wait for one with a timeout without the
// timeout closing the connection.
type liveConn struct {
	c    *websocket.Conn
	msgs chan []byte
	err  chan error
}

func dialBoard(t *testing.T, ts *httptest.Server, who string) *liveConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{accountHeader: {who}},
	})
	if err != nil {
		t.Fatalf("dial as %s: %v", who, err)
	}
	t.Cleanup(func() { c.CloseNow() })
	lc := &liveConn{c: c, msgs: make(chan []byte, 64), err: make(chan error, 1)}
	go func() {
		for {
			_, b, err := c.Read(context.Background())
			if err != nil {
				lc.err <- err
				close(lc.msgs)
				return
			}
			lc.msgs <- b
		}
	}()
	return lc
}

type boardState struct {
	Type     string           `json:"type"`
	Snapshot *game.UISnapshot `json:"snapshot"`
}

type boardResult struct {
	Type  string `json:"type"`
	ID    int    `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// next waits up to d for a message of type typ, decoding it into out.
func (lc *liveConn) next(t *testing.T, typ string, out any, d time.Duration) {
	t.Helper()
	timeout := time.After(d)
	for {
		select {
		case b, ok := <-lc.msgs:
			if !ok {
				t.Fatalf("connection closed waiting for %s", typ)
			}
			var head struct{ Type string }
			json.Unmarshal(b, &head)
			if head.Type == typ {
				if err := json.Unmarshal(b, out); err != nil {
					t.Fatal(err)
				}
				return
			}
		case <-timeout:
			t.Fatalf("no %s message within %v", typ, d)
		}
	}
}

// drain discards whatever arrives within d.
func (lc *liveConn) drain(d time.Duration) {
	timeout := time.After(d)
	for {
		select {
		case <-lc.msgs:
		case <-timeout:
			return
		}
	}
}

func (lc *liveConn) send(t *testing.T, id int, verb string, args map[string]any) boardResult {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"id": id, "verb": verb, "args": args})
	if err := lc.c.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
	var r boardResult
	lc.next(t, "result", &r, 5*time.Second)
	if r.ID != id {
		t.Fatalf("result id %d, want %d", r.ID, id)
	}
	return r
}

func startGame(t *testing.T, m *game.Manager, id string) {
	t.Helper()
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: id, DeckP1: testDeck, DeckP2: testDeck}); err != nil {
		t.Fatal(err)
	}
}

// TestBoardShowsOnlyOwnGame: each signed-in browser's live connection
// carries its own account's game, and receives only that game's changes.
func TestBoardShowsOnlyOwnGame(t *testing.T) {
	reg, ts := boardSite(t, newClock())
	startGame(t, reg.Get(alice).M, "a-game")
	startGame(t, reg.Get(bob).M, "b-game")

	a, b := dialBoard(t, ts, "alice"), dialBoard(t, ts, "bob")
	var st boardState
	a.next(t, "state", &st, 5*time.Second)
	if st.Snapshot == nil || st.Snapshot.Game != "a-game" {
		t.Fatalf("alice's board opened on %+v, want a-game", st.Snapshot)
	}
	b.next(t, "state", &st, 5*time.Second)
	if st.Snapshot == nil || st.Snapshot.Game != "b-game" {
		t.Fatalf("bob's board opened on %+v, want b-game", st.Snapshot)
	}
	a.drain(200 * time.Millisecond)

	// A change to bob's game reaches bob and not alice.
	if r := b.send(t, 1, "move", map[string]any{"iid": "p1-1", "to": "discard"}); !r.OK {
		t.Fatalf("bob's move: %+v", r)
	}
	b.next(t, "state", &st, 2*time.Second)
	if st.Snapshot == nil || st.Snapshot.Game != "b-game" {
		t.Errorf("bob's board after his move shows %+v, want b-game", st.Snapshot)
	}
	startGame(t, reg.Get(bob).M, "b-game-2")
	select {
	case msg := <-a.msgs:
		t.Fatalf("alice's board received a message after changes to bob's game: %s", msg)
	case <-time.After(500 * time.Millisecond):
	}

	// A change to alice's game reaches alice.
	startGame(t, reg.Get(alice).M, "a-game-2")
	a.next(t, "state", &st, 2*time.Second)
	if st.Snapshot == nil || st.Snapshot.Game != "a-game-2" {
		t.Errorf("alice's board after her new game shows %+v, want a-game-2", st.Snapshot)
	}
}

// TestBoardWithoutAGame: an account with no game sees an empty board, not
// another account's game.
func TestBoardWithoutAGame(t *testing.T) {
	reg, ts := boardSite(t, newClock())
	startGame(t, reg.Get(alice).M, "a-game")

	b := dialBoard(t, ts, "bob")
	var st boardState
	b.next(t, "state", &st, 5*time.Second)
	if st.Snapshot != nil {
		t.Errorf("bob, with no game, sees %q", st.Snapshot.Game)
	}
}

// TestAnotherAccountsBoardLink: opening another account's board link gives
// the "not your game" page with 403 and nothing of the board app or game.
func TestAnotherAccountsBoardLink(t *testing.T) {
	reg, ts := boardSite(t, newClock())
	startGame(t, reg.Get(alice).M, "a-game")

	_, own := get(t, ts, "bob", "/")
	code, body := get(t, ts, "bob", "/?board="+url.QueryEscape(Handle(alice.ID)))
	if code != http.StatusForbidden {
		t.Errorf("status %d, want 403", code)
	}
	if !strings.Contains(body, "Not your game") {
		t.Errorf("page does not say Not your game:\n%s", body)
	}
	if body == own {
		t.Error("another account's link served the same page as one's own board")
	}
	for _, leak := range []string{"<script", "a-game", alice.Email, Handle(alice.ID)} {
		if strings.Contains(body, leak) {
			t.Errorf("not-your-game page contains %q:\n%s", leak, body)
		}
	}
}

// TestOwnBoard: the board with no link, or with one's own link, is one's
// own board.
func TestOwnBoard(t *testing.T) {
	_, ts := boardSite(t, newClock())
	for _, path := range []string{"/", "/?board=" + url.QueryEscape(Handle(alice.ID))} {
		code, body := get(t, ts, "alice", path)
		if code != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", path, code)
		}
		if strings.Contains(body, "Not your game") {
			t.Errorf("GET %s: served the not-your-game page", path)
		}
	}
}

// TestBoardNeedsAnAccount: a request with no signed-in account is refused.
func TestBoardNeedsAnAccount(t *testing.T) {
	_, ts := boardSite(t, newClock())
	for _, path := range []string{"/", "/ws", "/api/card?def=tst-1"} {
		if code, _ := get(t, ts, "", path); code != http.StatusUnauthorized {
			t.Errorf("GET %s with no account: status %d, want 401", path, code)
		}
	}
}

// TestBoardActivity: loading the board page and making a board move count
// as activity; an open idle live connection and card lookups do not.
func TestBoardActivity(t *testing.T) {
	clock := newClock()
	reg, ts := boardSite(t, clock)
	e := reg.Get(alice)
	startGame(t, e.M, "a-game")

	loaded := clock.Advance(time.Minute)
	if code, _ := get(t, ts, "alice", "/"); code != http.StatusOK {
		t.Fatalf("page load: status %d", code)
	}
	if got := reg.LastActive(e); !got.Equal(loaded) {
		t.Errorf("after a page load, LastActive = %v, want %v", got, loaded)
	}

	clock.Advance(time.Minute)
	if code, _ := get(t, ts, "alice", "/api/card?def=tst-1"); code != http.StatusOK {
		t.Fatalf("card lookup: status %d", code)
	}
	if got := reg.LastActive(e); !got.Equal(loaded) {
		t.Errorf("a card lookup changed LastActive to %v, want %v", got, loaded)
	}

	clock.Advance(time.Minute)
	a := dialBoard(t, ts, "alice")
	var st boardState
	a.next(t, "state", &st, 5*time.Second)
	a.drain(200 * time.Millisecond)
	if got := reg.LastActive(e); !got.Equal(loaded) {
		t.Errorf("an idle live connection changed LastActive to %v, want %v", got, loaded)
	}

	moved := clock.Advance(time.Minute)
	if r := a.send(t, 1, "move", map[string]any{"iid": "p1-1", "to": "discard"}); !r.OK {
		t.Fatalf("move: %+v", r)
	}
	if got := reg.LastActive(e); !got.Equal(moved) {
		t.Errorf("after a board move, LastActive = %v, want %v", got, moved)
	}
}

// TestBoardConnectionEndsWithEntry: ending an account's entry closes its
// open live connection.
func TestBoardConnectionEndsWithEntry(t *testing.T) {
	reg, ts := boardSite(t, newClock())
	e := reg.Get(alice)
	startGame(t, e.M, "a-game")

	a := dialBoard(t, ts, "alice")
	var st boardState
	a.next(t, "state", &st, 5*time.Second)

	e.stop()
	select {
	case <-a.err:
	case <-time.After(2 * time.Second):
		t.Fatal("alice's live connection stayed open after her entry ended")
	}
}
