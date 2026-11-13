package telemetry_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// TestRedact covers the rules on hand-written messages.
func TestRedact(t *testing.T) {
	sensitive := []string{
		"LeDjo23 played Buddy-Buddy Poffin.",
		"NicJackson attached Basic Fighting Energy to Solrock in the Active Spot.",
		"Lillie's Determination",
		"Abra",
	}
	for _, tc := range []struct{ msg, want string }{
		// Quoted segments, with escaped quotes, go whatever they hold.
		{`line 1: "NicJackson drew Riolu." before the Setup section`, `line 1: "…" before the Setup section`},
		{`bad line "say \"hi\" Abra" here`, `bad line "…" here`},
		{`unclosed "Buddy-Buddy Poffin and more`, `unclosed "…`},
		// The longest run found in the sensitive text goes as one.
		{`no Buddy-Buddy Poffin for LeDjo23 in hand`, `no … for … in hand`},
		{`the decklist has only 3 Lillie's Determination`, `the decklist has only 3 …`},
		{`NicJackson has no Solrock in play in the active`, `… has no … in play in the active`},
		// Separately found words next to each other collapse into one.
		{`players [NicJackson LeDjo23]`, `players […]`},
		// Runs of common words stay, even though the sensitive text holds them.
		{`Energy in the Active Spot`, `Energy in the Active …`},
		{`is not on the current branch`, `is not on the current branch`},
		// Matches are word-bounded and case-sensitive.
		{`Abracadabra and abra`, `Abracadabra and abra`},
		{`no such tool`, `no such tool`},
		{``, ``},
	} {
		if got := telemetry.Redact(tc.msg, sensitive); got != tc.want {
			t.Errorf("Redact(%q)\n got %q\nwant %q", tc.msg, got, tc.want)
		}
	}
}

// TestSensitiveFromJSON walks every string value, splits lines and adds
// each decklist line's card name.
func TestSensitiveFromJSON(t *testing.T) {
	raw := json.RawMessage(`{"log":"Setup\nLeDjo23 played Abra.\n\n","deck":"Pokémon: 1\n3 Mega Lucario ex MEG 179\n4 Lillie's Clefairy ex\n","me":"NicJackson","seed":1,"nested":[{"x":"Iono"}]}`)
	got := map[string]bool{}
	for _, s := range telemetry.SensitiveFromJSON(raw) {
		got[s] = true
	}
	for _, want := range []string{"Setup", "LeDjo23 played Abra.", "3 Mega Lucario ex MEG 179", "Mega Lucario ex", "Lillie's Clefairy ex", "NicJackson", "Iono"} {
		if !got[want] {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	if got[""] {
		t.Error("an empty string is listed")
	}
	if telemetry.SensitiveFromJSON(nil) != nil || telemetry.SensitiveFromJSON(json.RawMessage(`not json`)) != nil {
		t.Error("no arguments should give nothing")
	}
}

// TestRedactRealImportErrors provokes real import failures from the repo's
// test battle logs and decklist and checks that redacting their messages
// against the call's arguments leaves no battle-log line, card name or
// player name, while the playmat's own wording survives.
func TestRedactRealImportErrors(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	logPath := "../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt"
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logBytes)
	shortDeck := strings.Replace(strings.Replace(string(deckText), "10 Basic Fighting Energy MEE 14", "1 Basic Fighting Energy MEE 14", 1), "Total Cards: 60", "", 1)

	type failure struct {
		name string
		req  game.ImportRequest
		// survives is playmat wording that must still be in some message.
		survives []string
	}
	for _, f := range []failure{
		{"line before Setup", game.ImportRequest{Log: "NicJackson drew Riolu.\n" + logText, Deck: string(deckText)}, []string{`"…" before the Setup section`}},
		{"missing copies", game.ImportRequest{Log: logText, Deck: shortDeck}, []string{"for … in deck", "has no … in play"}},
		{"wrong player", game.ImportRequest{Log: logText, Deck: string(deckText), Me: "LeDjo23"}, []string{"for … in hand", "has no … in play in the bench"}},
		{"unknown player", game.ImportRequest{Log: logText, Deck: string(deckText), Me: "Nobody"}, []string{"cannot tell which player owns the log"}},
	} {
		t.Run(f.name, func(t *testing.T) {
			f.req.Seed = 1
			args, _ := json.Marshal(map[string]any{"log": f.req.Log, "deck": f.req.Deck, "me": f.req.Me, "seed": 1})
			sensitive := telemetry.SensitiveFromJSON(args)

			m := game.NewManager(t.TempDir(), nil)
			res, err := m.Import(context.Background(), f.req)
			var msgs []string
			if err != nil {
				msgs = append(msgs, err.Error())
			}
			if res != nil {
				for _, p := range res.Report.Problems {
					msgs = append(msgs, p.Err)
				}
			}
			if len(msgs) == 0 {
				t.Fatal("the import did not fail")
			}
			var redacted []string
			for _, msg := range msgs {
				redacted = append(redacted, telemetry.Redact(msg, sensitive))
			}
			all := strings.Join(redacted, "\n")
			for _, leak := range leaks(t, f.req.Log, f.req.Deck) {
				if strings.Contains(all, leak) {
					t.Errorf("%q survives redaction:\n%s", leak, all)
				}
			}
			for _, s := range f.survives {
				if !strings.Contains(all, s) {
					t.Errorf("playmat wording %q lost:\n%s", s, all)
				}
			}
		})
	}
}

// leaks lists what must never survive: the players' names, every card
// name in the decklist and named in the log, and every log line except
// the bare section headings, which are playmat words.
func leaks(t *testing.T, logText, deckText string) []string {
	t.Helper()
	out := []string{"NicJackson", "LeDjo23", "Nobody", "Riolu",
		// Cards the opponent's lines in this log name.
		"Abra", "Duskull", "Buddy-Buddy Poffin", "Budew", "Telepathic Psychic Energy", "Gwynn", "Dusknoir", "Rare Candy", "Alakazam", "Itchy Pollen"}
	list, _ := deck.Parse(deckText)
	if list == nil || len(list.Entries) == 0 {
		t.Fatal("cannot read the decklist")
	}
	for _, e := range list.Entries {
		out = append(out, e.Name)
	}
	for _, l := range strings.Split(logText, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l == "Setup" {
			continue
		}
		out = append(out, l)
	}
	return out
}

// TestRedactEveryCorpusLog redacts, against each corpus log, a message
// naming each of its lines, so no log in the corpus has a line Redact
// would let through.
func TestRedactEveryCorpusLog(t *testing.T) {
	logs, _ := filepath.Glob("../../testdata/logs/*.txt")
	if len(logs) == 0 {
		t.Skip("no test corpus")
	}
	for _, path := range logs {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		args, _ := json.Marshal(map[string]string{"log": string(text)})
		sensitive := telemetry.SensitiveFromJSON(args)
		for _, l := range strings.Split(string(text), "\n") {
			l = strings.TrimSpace(l)
			if len(strings.Fields(l)) < 3 {
				continue // headings and the like
			}
			// Unquoted, as the importer's own messages name things.
			got := telemetry.Redact("cannot replay "+l+" here", sensitive)
			if strings.Contains(got, l) {
				t.Errorf("%s: line %q survives: %q", filepath.Base(path), l, got)
			}
		}
	}
}
