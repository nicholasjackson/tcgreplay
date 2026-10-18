package webui

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// fakeDB serves a tiny card pool with set code TST (copied from the game
// package tests).
type fakeDB map[string]*cards.Card

func newFakeDB() fakeDB {
	return fakeDB{
		"tst-1":   {ID: "tst-1", Name: "Riolu", Category: "Pokemon", Stage: "Basic", HP: 70, Image: "https://assets.tcgdex.net/en/x/tst/1"},
		"tst-2":   {ID: "tst-2", Name: "Lucario", Category: "Pokemon", Stage: "Stage1", EvolveFrom: "Riolu", HP: 130},
		"tst-3":   {ID: "tst-3", Name: "Ultra Ball", Category: "Trainer", TrainerType: "Item"},
		"sve-006": {ID: "sve-006", Name: "Fighting Energy", Category: "Energy", EnergyType: "Normal"},
	}
}

func (f fakeDB) Card(_ context.Context, id string) (*cards.Card, error) {
	if c, ok := f[id]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("%s: %w", id, cards.ErrNotFound)
}
func (f fakeDB) Cached(id string) *cards.Card { return f[id] }
func (f fakeDB) ByName(_ context.Context, name string) (string, error) {
	for id, c := range f {
		if c.Name == name {
			return id, nil
		}
	}
	return "", cards.ErrNotFound
}
func (f fakeDB) Resolve(_ context.Context, set, num string) (string, error) {
	if set != "TST" {
		return "", cards.ErrNotFound
	}
	return "tst-" + num, nil
}

const testDeck = `Pokémon: 8
4 Riolu TST 1
4 Lucario TST 2
Trainer: 4
4 Ultra Ball TST 3
Energy: 48
48 Basic {F} Energy SVE 6`

type stateMsg struct {
	Type     string           `json:"type"`
	Reveal   bool             `json:"reveal"`
	Snapshot *game.UISnapshot `json:"snapshot"`
}

type resultMsg struct {
	Type  string `json:"type"`
	ID    int    `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func setup(t *testing.T) (*httptest.Server, *game.Manager) {
	t.Helper()
	ts, m, _ := setupServer(t, nil)
	return ts, m
}

// setupServer is setup with the playmat's public address (nil for the
// desktop), also returning the server so tests can inspect its hub.
func setupServer(t *testing.T, public *url.URL) (*httptest.Server, *game.Manager, *Server) {
	t.Helper()
	m := game.NewManager(t.TempDir(), newFakeDB())
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: "ui-1", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
		t.Fatal(err)
	}
	s := New(m, state.P1)
	s.Public = public
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartHub(ctx)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, m, s
}

func dial(t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

// next reads messages until one of type typ arrives.
func next(t *testing.T, c *websocket.Conn, typ string, out any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, b, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %s: %v", typ, err)
		}
		var head struct{ Type string }
		json.Unmarshal(b, &head)
		if head.Type == typ {
			if err := json.Unmarshal(b, out); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
}

func send(t *testing.T, c *websocket.Conn, id int, verb string, args map[string]any) resultMsg {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"id": id, "verb": verb, "args": args})
	if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
	var r resultMsg
	next(t, c, "result", &r)
	if r.ID != id {
		t.Fatalf("result id %d, want %d", r.ID, id)
	}
	return r
}

// TestNoGameOpen: with no game the state message has a null snapshot and
// no list of stored games, and the old stored-game verbs are refused.
func TestNoGameOpen(t *testing.T) {
	m := game.NewManager(t.TempDir(), newFakeDB())
	s := New(m, state.P1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartHub(ctx)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	c := dial(t, ts)

	var msg map[string]any
	next(t, c, "state", &msg)
	if snap, ok := msg["snapshot"]; !ok || snap != nil {
		t.Errorf("snapshot = %v (present %v), want null", snap, ok)
	}
	if games, ok := msg["games"]; ok {
		t.Errorf("state message has games: %v", games)
	}
	for i, verb := range []string{"load_game", "list_games"} {
		r := send(t, c, i+1, verb, map[string]any{"game": "old"})
		if r.OK || !strings.Contains(r.Error, "no game open") {
			t.Errorf("%s with no game: %+v, want a no game open error", verb, r)
		}
	}

	// With a game open they are unknown verbs.
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: "ui-1", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
		t.Fatal(err)
	}
	for i, verb := range []string{"load_game", "list_games"} {
		if r := send(t, c, i+10, verb, map[string]any{"game": "ui-1"}); r.OK || !strings.Contains(r.Error, "unknown verb") {
			t.Errorf("%s with a game open: %+v, want an unknown verb error", verb, r)
		}
	}
}

func TestWebsocketLoadAndAct(t *testing.T) {
	ts, m := setup(t)
	c := dial(t, ts)

	var st stateMsg
	next(t, c, "state", &st)
	if st.Snapshot == nil || st.Snapshot.Game != "ui-1" {
		t.Fatalf("no initial snapshot: %+v", st)
	}
	if n := len(st.Snapshot.Board.Sides[state.P1].Hand); n != 7 {
		t.Fatalf("human sees %d cards in hand", n)
	}
	if n := len(st.Snapshot.Board.Sides[state.P2].Hand); n != 0 {
		t.Fatalf("human sees %d of Claude's hand", n)
	}

	// Get a Riolu into hand, then make it active.
	g, _ := m.Current()
	if l, _ := g.State().Find("p1-1"); l.Zone != state.Hand {
		if r := send(t, c, 1, "move", map[string]any{"iid": "p1-1", "to": "hand"}); !r.OK {
			t.Fatal(r.Error)
		}
	}
	if r := send(t, c, 2, "place_active", map[string]any{"iid": "p1-1"}); !r.OK {
		t.Fatal(r.Error)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		next(t, c, "state", &st)
		if a := st.Snapshot.Board.Sides[state.P1].Active; a != nil && a.Pokemon[0].IID == "p1-1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no state push with the new active")
		}
	}

	// Errors come back as results, not dropped connections.
	if r := send(t, c, 3, "bench", map[string]any{"iid": "p1-1"}); r.OK || r.Error == "" {
		t.Fatalf("expected an error, got %+v", r)
	}
	// Acting for Claude's seat is refused in play mode; reveal too.
	if r := send(t, c, 4, "draw", map[string]any{"seat": "p2"}); r.OK {
		t.Fatal("acting as p2 allowed in play mode")
	}
	if r := send(t, c, 5, "set_reveal", map[string]any{"reveal": true}); r.OK {
		t.Fatal("reveal allowed in play mode")
	}

	// Claude's moves arrive with per-event frames for animation.
	if _, err := g.Draw(state.P2, state.ActorClaude, 2); err != nil {
		t.Fatal(err)
	}
	for {
		next(t, c, "state", &st)
		if len(st.Snapshot.Frames) > 0 {
			break
		}
	}
	if n := len(st.Snapshot.Frames); n != 2 || st.Snapshot.Frames[0].Actor != state.ActorClaude {
		t.Fatalf("frames: %d", n)
	}
	if strings.Contains(st.Snapshot.Frames[0].Text, "Riolu") || strings.Contains(st.Snapshot.Frames[0].Text, "Energy") {
		t.Fatalf("frame leaks Claude's draw: %s", st.Snapshot.Frames[0].Text)
	}

	// Review mode allows the reveal view.
	if r := send(t, c, 6, "set_mode", map[string]any{"mode": "review"}); !r.OK {
		t.Fatal(r.Error)
	}
	if r := send(t, c, 7, "set_reveal", map[string]any{"reveal": true}); !r.OK {
		t.Fatal(r.Error)
	}
	for {
		next(t, c, "state", &st)
		if st.Reveal {
			break
		}
	}
	if n := len(st.Snapshot.Board.Sides[state.P2].Hand); n != 9 {
		t.Fatalf("reveal view shows %d of p2's hand", n)
	}
}

func TestEndTurnCopiesPrompt(t *testing.T) {
	ts, m := setup(t)
	g, _ := m.Current()
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
	c := dial(t, ts)
	var st stateMsg
	next(t, c, "state", &st)
	b, _ := json.Marshal(map[string]any{"id": 1, "verb": "end_turn"})
	c.Write(context.Background(), websocket.MessageText, b)
	var r struct {
		resultMsg
		Data struct{ Prompt string } `json:"data"`
	}
	next(t, c, "result", &r)
	if !r.OK || !strings.Contains(r.Data.Prompt, "get_log since") || !strings.Contains(r.Data.Prompt, "turn 2") {
		t.Fatalf("prompt: %+v", r)
	}
}

func TestRejectsForeignHostAndOrigin(t *testing.T) {
	ts, _ := setup(t)
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.Host = "evil.example"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign host got %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err = websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
	})
	if err == nil {
		t.Fatal("foreign origin accepted")
	}

	resp, err = http.Get(ts.URL + "/api/card?def=tst-1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"name":"Riolu"`) {
		t.Fatalf("card api: %d %s", resp.StatusCode, body)
	}

	resp, err = http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "<title>tcgreplay</title>") {
		t.Fatalf("index: %d", resp.StatusCode)
	}
}

func TestPublicAddress(t *testing.T) {
	public, _ := url.Parse("https://playmat.example")
	ts, _, _ := setupServer(t, public)

	// The board loads for a host that is not loopback.
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.Host = "playmat.example"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "<title>tcgreplay</title>") {
		t.Fatalf("index on public host: %d", resp.StatusCode)
	}

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	dialFrom := func(origin string) (*websocket.Conn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
			HTTPHeader: http.Header{"Origin": []string{origin}},
		})
		if c != nil {
			t.Cleanup(func() { c.CloseNow() })
		}
		return c, err
	}

	// A page on the public address may open the live connection.
	c, err := dialFrom("https://playmat.example")
	if err != nil {
		t.Fatalf("public origin refused: %v", err)
	}
	var st stateMsg
	next(t, c, "state", &st)
	if st.Snapshot == nil || st.Snapshot.Game != "ui-1" {
		t.Fatalf("no initial snapshot: %+v", st)
	}

	// Another site may not.
	if _, err := dialFrom("https://evil.example"); err == nil {
		t.Fatal("foreign origin accepted")
	}
}

// rawWS is a bare websocket client that shows every frame the server sends,
// pings included, and answers pings only when told to.
type rawWS struct {
	conn net.Conn
	br   *bufio.Reader
}

func rawDial(t *testing.T, ts *httptest.Server) *rawWS {
	t.Helper()
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n", ts.Listener.Addr())
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake: %d", resp.StatusCode)
	}
	return &rawWS{conn: conn, br: br}
}

// readFrame reads one (unmasked, server-sent) frame.
func (r *rawWS) readFrame() (opcode byte, payload []byte, err error) {
	var h [2]byte
	if _, err := io.ReadFull(r.br, h[:]); err != nil {
		return 0, nil, err
	}
	n := uint64(h[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(r.br, b[:]); err != nil {
			return 0, nil, err
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(r.br, b[:]); err != nil {
			return 0, nil, err
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	payload = make([]byte, n)
	if _, err := io.ReadFull(r.br, payload); err != nil {
		return 0, nil, err
	}
	return h[0] & 0x0f, payload, nil
}

// pong answers a ping, echoing its payload in a masked frame.
func (r *rawWS) pong(payload []byte) error {
	mask := [4]byte{1, 2, 3, 4}
	f := []byte{0x8a, 0x80 | byte(len(payload))}
	f = append(f, mask[:]...)
	for i, b := range payload {
		f = append(f, b^mask[i%4])
	}
	_, err := r.conn.Write(f)
	return err
}

func clientCount(s *Server) int {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	return len(s.hub.clients)
}

func TestPing(t *testing.T) {
	// pingInterval is read when a client connects; set it before any server
	// goroutine exists and restore it after the servers are closed.
	oldInterval, oldTimeout := pingInterval, pingTimeout
	pingInterval, pingTimeout = 50*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { pingInterval, pingTimeout = oldInterval, oldTimeout })

	t.Run("open board is pinged at the interval", func(t *testing.T) {
		ts, _, s := setupServer(t, nil)
		ws := rawDial(t, ts)
		ws.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		start := time.Now()
		pings := 0
		for pings < 5 {
			op, payload, err := ws.readFrame()
			if err != nil {
				t.Fatalf("after %d pings: %v", pings, err)
			}
			if op != 0x9 {
				continue
			}
			pings++
			if err := ws.pong(payload); err != nil {
				t.Fatal(err)
			}
		}
		// Five pings at 50 ms take about 250 ms; at the 30 s default none
		// would arrive. Each ping waits for the previous pong, so the board
		// that answers is kept.
		if d := time.Since(start); d < 200*time.Millisecond || d > 3*time.Second {
			t.Fatalf("5 pings took %v at a 50ms interval", d)
		}
		if n := clientCount(s); n != 1 {
			t.Fatalf("answering board dropped: %d clients", n)
		}
	})

	t.Run("silent board is disconnected", func(t *testing.T) {
		// The server waits pingTimeout (200 ms here) for a pong before
		// giving up.
		ts, _, s := setupServer(t, nil)
		ws := rawDial(t, ts)
		ws.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		pinged := false
		var err error
		for {
			var op byte
			if op, _, err = ws.readFrame(); err != nil {
				break
			}
			if op == 0x9 {
				pinged = true // never answered
			}
		}
		if !pinged {
			t.Fatal("no ping before the connection ended")
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			t.Fatal("silent board was not disconnected within 5s")
		}
		deadline := time.Now().Add(5 * time.Second)
		for clientCount(s) != 0 {
			if time.Now().After(deadline) {
				t.Fatal("silent board still registered with the hub")
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

// limitKey marks the request context the limit test's server sees.
type limitKey struct{}

// TestLimitHook: with Limit set, every board action asks it first with the
// live connection's request context; a refusal comes back as the action's
// error, word for word, and the action does not run. Allowed, the action
// runs as before.
func TestLimitHook(t *testing.T) {
	m := game.NewManager(t.TempDir(), newFakeDB())
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: "ui-1", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
		t.Fatal(err)
	}
	s := New(m, state.P1)
	const refusal = "too many requests: this account is limited to 5 requests a minute; try again in 12s"
	var refuse atomic.Bool
	var calls, sawCtx atomic.Int32
	s.Limit = func(ctx context.Context) error {
		calls.Add(1)
		if ctx.Value(limitKey{}) == "live" {
			sawCtx.Add(1)
		}
		if refuse.Load() {
			return errors.New(refusal)
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartHub(ctx)
	h := s.Handler()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), limitKey{}, "live")))
	}))
	t.Cleanup(ts.Close)
	c := dial(t, ts)
	var st stateMsg
	next(t, c, "state", &st)

	g, _ := m.Current()
	before, _ := g.State().Find("p1-1")
	if before.Zone == state.Discard {
		t.Fatalf("p1-1 already in the discard")
	}

	refuse.Store(true)
	r := send(t, c, 1, "move", map[string]any{"iid": "p1-1", "to": "discard"})
	if r.OK || r.Error != refusal {
		t.Fatalf("refused move: %+v, want error %q", r, refusal)
	}
	if after, _ := g.State().Find("p1-1"); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("refused move changed the board: p1-1 %v, was %v", after, before)
	}

	refuse.Store(false)
	if r := send(t, c, 2, "move", map[string]any{"iid": "p1-1", "to": "discard"}); !r.OK {
		t.Fatalf("allowed move: %+v", r)
	}
	if after, _ := g.State().Find("p1-1"); after.Zone != state.Discard {
		t.Fatalf("allowed move left p1-1 at %+v", after)
	}
	if n, ctxN := calls.Load(), sawCtx.Load(); n != 2 || ctxN != 2 {
		t.Errorf("Limit called %d times (%d with the request context), want 2 and 2", n, ctxN)
	}
}

// TestNoLimitOnDesktop: the desktop board has no Limit, and many actions
// in a row are never refused.
func TestNoLimitOnDesktop(t *testing.T) {
	ts, _, s := setupServer(t, nil)
	if s.Limit != nil {
		t.Fatal("desktop board has a Limit")
	}
	c := dial(t, ts)
	var st stateMsg
	next(t, c, "state", &st)
	for i := 1; i <= 400; i++ {
		if r := send(t, c, i, "set_mode", map[string]any{"mode": "review"}); !r.OK {
			t.Fatalf("action %d refused on the desktop: %+v", i, r)
		}
	}
}

// rawState reads state messages until one satisfies ok, failing after a
// few seconds; it returns the message as a map so tests can tell an absent
// key from an empty one.
func rawState(t *testing.T, c *websocket.Conn, ok func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var msg map[string]any
		next(t, c, "state", &msg)
		if ok(msg) {
			return msg
		}
		if time.Now().After(deadline) {
			t.Fatalf("no matching state message; last %v", msg)
		}
	}
}

func noSnapshot(msg map[string]any) bool {
	snap, present := msg["snapshot"]
	return present && snap == nil
}

func hasSnapshot(msg map[string]any) bool { return msg["snapshot"] != nil }

// watchedCount is how many games the hub holds.
func watchedCount(s *Server) int {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	return len(s.hub.watched)
}

// TestExpiryNotice: when the game expires an open board switches to the
// notice without a reload, a newly loaded board shows it first, board
// actions report it, the hub lets the old game go, and a new game clears
// the notice.
func TestExpiryNotice(t *testing.T) {
	const notice = "ui-1 expired: import its saved record again"
	ts, m, s := setupServer(t, nil)
	c := dial(t, ts)
	if msg := rawState(t, c, hasSnapshot); msg["notice"] != nil {
		t.Errorf("state with a game open has notice %v", msg["notice"])
	}
	if n := watchedCount(s); n != 1 {
		t.Fatalf("hub watches %d games before expiry, want 1", n)
	}

	m.Expire(notice)

	// The open board switches to the notice.
	msg := rawState(t, c, noSnapshot)
	if msg["notice"] != notice {
		t.Errorf("open board notice = %v, want %q", msg["notice"], notice)
	}
	if n := watchedCount(s); n != 0 {
		t.Errorf("hub still watches %d games after expiry, want 0", n)
	}

	// A newly loaded board shows it in its first state.
	c2 := dial(t, ts)
	var first map[string]any
	next(t, c2, "state", &first)
	if !noSnapshot(first) || first["notice"] != notice {
		t.Errorf("new board's first state: snapshot %v notice %v, want null and %q", first["snapshot"], first["notice"], notice)
	}

	// Board actions report the notice.
	for i, verb := range []string{"draw", "move", "set_mode"} {
		r := send(t, c2, i+1, verb, map[string]any{"iid": "p1-1", "to": "hand", "mode": "review"})
		if r.OK || r.Error != notice {
			t.Errorf("%s after expiry: %+v, want the error %q", verb, r, notice)
		}
	}

	// Starting a game clears the notice and works as usual.
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: "ui-2", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
		t.Fatal(err)
	}
	for _, conn := range []*websocket.Conn{c, c2} {
		msg := rawState(t, conn, hasSnapshot)
		if n, present := msg["notice"]; present {
			t.Errorf("state after a new game has notice %v", n)
		}
		snap, _ := msg["snapshot"].(map[string]any)
		if snap["game"] != "ui-2" {
			t.Errorf("snapshot game = %v, want ui-2", snap["game"])
		}
	}
	if n := watchedCount(s); n != 1 {
		t.Errorf("hub watches %d games after a new game, want 1", n)
	}
	if r := send(t, c, 10, "set_mode", map[string]any{"mode": "review"}); !r.OK {
		t.Errorf("set_mode after a new game: %+v", r)
	}
}

// TestNoNoticeWithoutExpiry: a board with no game that never expired has
// no notice key, and board actions give the usual no game open error.
func TestNoNoticeWithoutExpiry(t *testing.T) {
	m := game.NewManager(t.TempDir(), newFakeDB())
	s := New(m, state.P1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartHub(ctx)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	c := dial(t, ts)
	var msg map[string]any
	next(t, c, "state", &msg)
	if !noSnapshot(msg) {
		t.Errorf("snapshot = %v, want null", msg["snapshot"])
	}
	if n, present := msg["notice"]; present {
		t.Errorf("state with no expiry has notice %v", n)
	}
	if r := send(t, c, 1, "draw", nil); r.OK || !strings.HasPrefix(r.Error, "no game open") {
		t.Errorf("draw with no game: %+v, want the no game open error", r)
	}
}

// TestHubWatchesOnlyCurrent: replacing the game leaves the hub holding only
// the new one.
func TestHubWatchesOnlyCurrent(t *testing.T) {
	_, m, s := setupServer(t, nil)
	for i := 2; i <= 4; i++ {
		if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: fmt.Sprintf("ui-%d", i), DeckP1: testDeck, DeckP2: testDeck}); err != nil {
			t.Fatal(err)
		}
		if n := watchedCount(s); n != 1 {
			t.Errorf("hub watches %d games after game %d, want 1", n, i)
		}
	}
	g, _ := m.Current()
	s.hub.mu.Lock()
	ok := s.hub.watched[g]
	s.hub.mu.Unlock()
	if !ok {
		t.Error("hub does not watch the current game")
	}
}
