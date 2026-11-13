package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// testDisplayName is the Google display name the fake Google puts in
// its ID tokens for the usage tests. The playmat does not ask for it, so
// it must never reach a record or a log line.
const testDisplayName = "Nicola Testerton-Smythe"

// fakeGoogleAccessToken is the access token the fake Google issues.
const fakeGoogleAccessToken = "fake-google-access-token"

// googleTokens makes h's fake Google add testDisplayName to each ID token
// and returns a function giving every ID token it has issued since. The
// token is rebuilt from the claims exactly as the fake encodes it.
func googleTokens(h *hostedServer) func() []string {
	var mu sync.Mutex
	var issued []string
	h.g.Claims = func(claims map[string]any) {
		claims["name"] = testDisplayName
		head, _ := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
		body, _ := json.Marshal(claims)
		enc := base64.RawURLEncoding.EncodeToString
		mu.Lock()
		defer mu.Unlock()
		issued = append(issued, enc(head)+"."+enc(body)+".c2ln", enc(body))
	}
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), issued...)
	}
}

// bearer adds a playmat access token to each request.
type bearer struct {
	token string
	base  http.RoundTripper
}

func (b *bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// connectAs opens an MCP session to h as the named client (an assistant,
// such as "claude-ai"), carrying the playmat access token token.
func (h *hostedServer) connectAs(t *testing.T, client, token string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tr := &mcp.StreamableClientTransport{
		Endpoint:   strings.TrimSuffix(h.public, "/") + "/mcp",
		HTTPClient: &http.Client{Transport: &bearer{token: token, base: hostauthtest.Server(h.public, h.ts.URL).Transport}},
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: client}, nil).Connect(ctx, tr, nil)
	if err != nil {
		t.Fatalf("connect as %s: %v", client, err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// countEvents counts recs' records of event for player.
func countEvents(recs []map[string]string, event, player string) int {
	n := 0
	for _, r := range recs {
		if r["event"] == event && r["player"] == player {
			n++
		}
	}
	return n
}

// boardFail sends a board action over c that the playmat refuses and
// returns its error.
func boardFail(t *testing.T, c *websocket.Conn, id int, verb string, args map[string]any) string {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"id": id, "verb": verb, "args": args})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
	var r boardResult
	if err := readBoard(ctx, c, "result", &r); err != nil {
		t.Fatalf("%s result: %v", verb, err)
	}
	if r.ID != id || r.OK || r.Error == "" {
		t.Fatalf("%s result %+v, want an error for id %d", verb, r, id)
	}
	return r.Error
}

// TestHostedUsageSession: one player signs in to the board and through
// a named assistant (claude-ai), imports a game, starts a new game, makes
// a malformed import and a failing board move. The records hold a sign-in,
// a tool call per call naming the tool and the assistant, the import, the
// new game and an error for each failure, each with a time and the same
// player ID; and neither the records nor the log hold the player's email
// or display name, any Google or playmat token, any line of the battle log
// or any card name from the deck.
func TestHostedUsageSession(t *testing.T) {
	logs := captureLog(t)
	h := startHosted(t, testPublic)
	issued := googleTokens(h)
	h.g.SetAccount(userA)
	cookie := h.cookie(t)
	toks := hostauthtest.Connect(t, h.public, h.ts.URL, h.g)
	cs := h.connectAs(t, "claude-ai", toks.AccessToken)

	deckText, logText := readTestdata(t, testDeckPath), readTestdata(t, testLogA)
	callObject(t, cs, "import_log", map[string]any{"log": logText, "deck": deckText, "id": "usage-imp"})
	callObject(t, cs, "new_game", map[string]any{"deck_p1": deckText, "deck_p2": deckText, "id": "usage-new"})

	// A line before the log's Setup heading.
	const strayLine = "NicJackson played Mega Lucario ex before anything else."
	bad := strayLine + "\n" + logText
	if out, isErr := toolResult(t, cs, "import_log", map[string]any{"log": bad, "deck": deckText, "id": "usage-bad"}); !isErr {
		t.Fatalf("malformed import accepted: %s", out)
	}

	c, _ := boardSnapshot(t, h, cookie)
	boardFail(t, c, 1, "move", map[string]any{"iid": "p1-1", "to": "nowhere"})

	recs := eventRecords(t, h.events.String())
	for _, r := range recs {
		if r["player"] != "" && r["player"] != aID {
			t.Errorf("record %v names a player other than A (%s)", r, aID)
		}
	}
	wantEvent(t, recs, "sign_in", aID, nil)
	wantEvent(t, recs, "player_day", aID, nil)
	claude := map[string]string{"assistant": "Claude"}
	wantEvent(t, recs, "tool_call", aID, map[string]string{"tool": "import_log", "assistant": "Claude"})
	wantEvent(t, recs, "tool_call", aID, map[string]string{"tool": "new_game", "assistant": "Claude"})
	wantEvent(t, recs, "import", aID, claude)
	wantEvent(t, recs, "new_game", aID, claude)
	if n := countEvents(recs, "tool_call", aID); n != 3 {
		t.Errorf("%d tool_call records, want 3 (two imports and a new game):\n%s", n, h.events)
	}
	if n := countEvents(recs, "import", aID); n != 1 {
		t.Errorf("%d import records, want 1 (the malformed one is an error):\n%s", n, h.events)
	}
	if n := countEvents(recs, "player_day", aID); n != 1 {
		t.Errorf("%d player_day records, want 1:\n%s", n, h.events)
	}

	var toolErr, boardErr map[string]string
	for _, r := range recs {
		if r["event"] != "error" {
			continue
		}
		switch {
		case r["tool"] == "import_log":
			toolErr = r
		case r["action"] == "move":
			boardErr = r
		default:
			t.Errorf("unexpected error record %v", r)
		}
	}
	if toolErr == nil || toolErr["player"] != aID || toolErr["message"] == "" || toolErr["level"] != "error" {
		t.Errorf("import error record %v, want tool import_log, a message and A's ID", toolErr)
	}
	if boardErr == nil || boardErr["player"] != aID || boardErr["message"] == "" || boardErr["level"] != "error" {
		t.Errorf("board error record %v, want action move, a message and A's ID", boardErr)
	}

	// Nothing that identifies the player or holds their text.
	secrets := map[string]string{
		"email":                  userA.Email,
		"display name":           testDisplayName,
		"Google access token":    fakeGoogleAccessToken,
		"playmat access token":   toks.AccessToken,
		"playmat refresh token":  toks.RefreshToken,
		"board session cookie":   cookie.Value,
		"log player":             "NicJackson",
		"log opponent":           "LeDjo23",
		"malformed import line":  strayLine,
		"stray line's card name": "Mega Lucario ex",
	}
	ids := issued()
	if len(ids) == 0 {
		t.Fatal("the fake Google issued no ID token")
	}
	for i, tok := range ids {
		secrets["Google ID token "+string(rune('a'+i))] = tok
	}
	for _, l := range strings.Split(logText, "\n") {
		if l = strings.TrimSpace(l); l != "" && l != "Setup" {
			secrets["log line "+l] = l
		}
	}
	list, err := deck.Parse(deckText)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range list.Entries {
		secrets["card "+e.Name] = e.Name
	}
	for what, s := range secrets {
		if s == "" {
			t.Errorf("%s is empty; the check would prove nothing", what)
			continue
		}
		for name, out := range map[string]string{"records": h.events.String(), "log": logs.String()} {
			if strings.Contains(out, s) {
				t.Errorf("%s contain the %s (%q):\n%s", name, what, s, out)
			}
		}
	}
}

// TestHostedDailyPlayers: two players active in one day, A twice and B
// once, give exactly two player_day records for that day, one each,
// alongside their tool-call, import and new-game records.
func TestHostedDailyPlayers(t *testing.T) {
	clock := &idleClock{t: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	h := startHostedWith(t, testPublic, func(c *config) { c.now = clock.Now })
	deckText := readTestdata(t, testDeckPath)

	csA, _ := h.signInAs(t, userA)
	importFor(t, csA, testLogA, "a-imp", "")
	clock.Advance(3 * time.Hour)
	callObject(t, csA, "new_game", map[string]any{"deck_p1": deckText, "deck_p2": deckText, "id": "a-new"})

	clock.Advance(time.Hour)
	csB, _ := h.signInAs(t, userB)
	importFor(t, csB, testLogB, "b-imp", "")

	recs := eventRecords(t, h.events.String())
	var days []map[string]string
	for _, r := range recs {
		if r["event"] == "player_day" {
			days = append(days, r)
		}
	}
	if len(days) != 2 || countEvents(days, "player_day", aID) != 1 || countEvents(days, "player_day", bID) != 1 {
		t.Fatalf("player_day records %v, want exactly one for A and one for B", days)
	}
	for _, d := range days {
		if !strings.HasPrefix(d["time"], "2026-11-12T") {
			t.Errorf("player_day %v, want it on 2026-11-12", d)
		}
	}
	wantEvent(t, recs, "tool_call", aID, map[string]string{"tool": "import_log"})
	wantEvent(t, recs, "tool_call", aID, map[string]string{"tool": "new_game"})
	wantEvent(t, recs, "import", aID, nil)
	wantEvent(t, recs, "new_game", aID, nil)
	wantEvent(t, recs, "tool_call", bID, map[string]string{"tool": "import_log"})
	wantEvent(t, recs, "import", bID, nil)
}

// TestHostedRateLimitRecordedOnce: a tool call refused by the request
// limit is recorded once, as limited, and not as a tool call.
func TestHostedRateLimitRecordedOnce(t *testing.T) {
	h := startHostedWith(t, testPublic, func(c *config) { c.rateLimit = 1 })
	cs := h.connect(t) // initialize spends the one request
	if out, isErr := toolResult(t, cs, "ping", nil); !isErr {
		t.Fatalf("ping over a limit of 1 allowed: %s", out)
	}
	recs := eventRecords(t, h.events.String())
	if n := countEvents(recs, "limited", oneID); n != 1 {
		t.Errorf("%d limited records, want 1:\n%s", n, h.events)
	}
	wantEvent(t, recs, "limited", oneID, map[string]string{"reason": "rate"})
	for _, r := range recs {
		if r["event"] == "tool_call" || r["event"] == "error" {
			t.Errorf("refused call recorded as %v", r)
		}
	}
}
