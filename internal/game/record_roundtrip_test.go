package game

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	corpusDeckPath = "../../testdata/decks/mega-lucario-ex-v12.txt"
	donkLogPath    = "../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt"
)

// assumedOppDeck is a 60-card opponent list in TCG Live export format.
const assumedOppDeck = `Pokémon: 12
4 Abra
3 Kadabra
3 Alakazam
2 Duskull

Trainer: 4
4 Buddy-Buddy Poffin

Energy: 44
44 Basic Psychic Energy`

// corpusDeck reads the corpus deck, skipping the test without it.
func corpusDeck(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(corpusDeckPath)
	if err != nil {
		t.Skip("no test corpus")
	}
	return string(b)
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// sameGame checks two games have the same event path and the same state
// at event 1, half way and at the head. It moves both onto branches.
func sameGame(t *testing.T, want, got *Game) {
	t.Helper()
	wp, err := want.log.PathTo(want.log.Head())
	if err != nil {
		t.Fatal(err)
	}
	gp, err := got.log.PathTo(got.log.Head())
	if err != nil {
		t.Fatal(err)
	}
	if len(wp) != len(gp) {
		t.Fatalf("event paths differ in length: %d, restored %d", len(wp), len(gp))
	}
	if !reflect.DeepEqual(wp, gp) {
		for i := range wp {
			if !reflect.DeepEqual(wp[i], gp[i]) {
				t.Fatalf("event paths first differ at index %d:\n  %+v\n  restored %+v", i, wp[i], gp[i])
			}
		}
	}
	head := want.Head()
	if got.Head() != head {
		t.Fatalf("head %d, restored %d", head, got.Head())
	}
	for _, ev := range []int{1, head / 2, head} {
		if _, err := want.JumpTo(ev, "check"); err != nil {
			t.Fatal(err)
		}
		if _, err := got.JumpTo(ev, "check"); err != nil {
			t.Fatal(err)
		}
		if snapshot(t, want) != snapshot(t, got) {
			t.Errorf("state at event %d differs after restoring", ev)
		}
		if !reflect.DeepEqual(want.State(), got.State()) {
			t.Errorf("state at event %d differs (DeepEqual) after restoring", ev)
		}
	}
}

// restore imports rec into a fresh manager.
func restore(t *testing.T, rec Record) *ImportResult {
	t.Helper()
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(context.Background(), ImportRequest{Log: rec.JSON()})
	if err != nil {
		t.Fatalf("restoring record: %v", err)
	}
	return res
}

// TestRecordFromImport checks the record built by an import holds exactly
// what was imported.
func TestRecordFromImport(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	ctx := context.Background()
	m := NewManager(t.TempDir(), nil)

	res, err := m.Import(ctx, ImportRequest{ID: "fixed", Log: logText, Deck: deckText, OppDeck: assumedOppDeck, Seed: 12345})
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Record
	if rec.Version != RecordVersion || RecordVersion != 1 {
		t.Errorf("version %d, want %d (1)", rec.Version, RecordVersion)
	}
	if rec.Log != logText {
		t.Error("record log differs from the log imported")
	}
	if rec.Me == "" || rec.Me != res.Report.Me {
		t.Errorf("me %q, report me %q", rec.Me, res.Report.Me)
	}
	if rec.Deck != deckText {
		t.Error("record deck differs from the deck imported")
	}
	if rec.OppDeck != assumedOppDeck {
		t.Errorf("opp deck %q, want the list given", rec.OppDeck)
	}
	if rec.Seed != 12345 {
		t.Errorf("seed %d, want 12345", rec.Seed)
	}
	if !strings.HasPrefix(rec.Check, "sha256:") || len(rec.Check) != len("sha256:")+64 {
		t.Errorf("check %q", rec.Check)
	}
	if parsed, ok, err := ParseRecord(rec.JSON()); err != nil || !ok || parsed != rec {
		t.Errorf("record does not parse back to itself: ok %v, %v", ok, err)
	}
	if got, err := m.Record(); err != nil || got != rec {
		t.Errorf("manager record differs: %v", err)
	}
	if got, ok := res.Game.Record(); !ok || got != rec {
		t.Error("game record differs")
	}

	// A random seed: the record carries the seed actually used.
	res, err = m.Import(ctx, ImportRequest{ID: "random", Log: logText, Deck: deckText})
	if err != nil {
		t.Fatal(err)
	}
	rec = res.Record
	if rec.Seed == 0 {
		t.Fatal("record of a random-seed import has seed 0")
	}
	if rec.OppDeck != "" {
		t.Errorf("opp deck %q, want none", rec.OppDeck)
	}
	if got, err := m.Record(); err != nil || got != rec {
		t.Errorf("manager record is not the latest import's: %v", err)
	}
	// Importing with that seed explicitly gives the same game. A separate
	// manager, so the game compared stays current in this one.
	again, err := NewManager(t.TempDir(), nil).Import(ctx, ImportRequest{ID: "again", Log: logText, Deck: deckText, Seed: rec.Seed})
	if err != nil {
		t.Fatal(err)
	}
	if again.Record != rec {
		t.Error("explicit-seed import's record differs from the random-seed one's")
	}
	sameGame(t, res.Game, again.Game)

	// No deck: the last deck is reused and recorded.
	res, err = m.Import(ctx, ImportRequest{ID: "nodeck", Log: logText, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	if res.Record.Deck != deckText {
		t.Error("record of an import without a deck does not hold the reused deck")
	}
	if got, ok := res.Game.Record(); !ok || got != res.Record {
		t.Error("game record differs")
	}
}

// TestRecordRestoresCorpus restores every corpus log from its record in a
// fresh manager and checks the game comes out identical.
func TestRecordRestoresCorpus(t *testing.T) {
	deckText := corpusDeck(t)
	logs, _ := filepath.Glob("../../testdata/logs/*.txt")
	if len(logs) == 0 {
		t.Skip("no test corpus")
	}
	for _, path := range logs {
		logText := readText(t, path)
		for _, opp := range []struct{ name, list string }{{"no-opp", ""}, {"opp", assumedOppDeck}} {
			t.Run(filepath.Base(path)+"/"+opp.name, func(t *testing.T) {
				m1 := NewManager(t.TempDir(), nil)
				res, err := m1.Import(context.Background(), ImportRequest{Log: logText, Deck: deckText, OppDeck: opp.list})
				if err != nil {
					t.Fatal(err)
				}
				if res.Record.Seed == 0 || res.Record.OppDeck != opp.list {
					t.Fatalf("record seed %d, opp deck %q", res.Record.Seed, res.Record.OppDeck)
				}
				back := restore(t, res.Record)
				if back.Record != res.Record {
					t.Errorf("restored game's record differs:\n  %+v\n  %+v", res.Record, back.Record)
				}
				if !reflect.DeepEqual(res.Report, back.Report) {
					t.Errorf("report differs:\n  %+v\n  restored %+v", res.Report, back.Report)
				}
				sameGame(t, res.Game, back.Game)
			})
		}
	}
}

// TestRecordAfterSetOpponentDeck checks a rebuild with an assumed list
// carries a record holding it and the original seed, which restores the
// rebuilt game.
func TestRecordAfterSetOpponentDeck(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	ctx := context.Background()
	m := NewManager(t.TempDir(), nil)
	first, err := m.Import(ctx, ImportRequest{ID: "g", Log: logText, Deck: deckText})
	if err != nil {
		t.Fatal(err)
	}
	res, err := m.SetOpponentDeck(ctx, assumedOppDeck, false)
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Record
	if rec.OppDeck != assumedOppDeck {
		t.Errorf("opp deck %q, want the list given", rec.OppDeck)
	}
	if rec.Seed != first.Record.Seed {
		t.Errorf("seed %d, original import %d", rec.Seed, first.Record.Seed)
	}
	if rec.Deck != deckText || rec.Log != logText || rec.Me != first.Record.Me {
		t.Error("deck, log or name changed in the rebuild")
	}
	if got, err := m.Record(); err != nil || got != rec {
		t.Errorf("manager record differs: %v", err)
	}
	back := restore(t, rec)
	if !reflect.DeepEqual(res.Report, back.Report) {
		t.Errorf("report differs:\n  %+v\n  restored %+v", res.Report, back.Report)
	}
	sameGame(t, res.Game, back.Game)
}

// TestRecordRejectsExtras refuses a record imported with any of the values
// it already holds, leaving the current game in place.
func TestRecordRejectsExtras(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	ctx := context.Background()
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(ctx, ImportRequest{ID: "g", Log: logText, Deck: deckText, Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Record.JSON()
	want := snapshot(t, res.Game)
	for name, req := range map[string]ImportRequest{
		"deck":     {ID: "x1", Log: text, Deck: deckText},
		"opp deck": {ID: "x2", Log: text, OppDeck: assumedOppDeck},
		"me":       {ID: "x3", Log: text, Me: "someone"},
		"seed":     {ID: "x4", Log: text, Seed: 9},
	} {
		_, err := m.Import(ctx, req)
		if err == nil || !strings.Contains(err.Error(), "already holds") {
			t.Errorf("%s: err %v, want one saying the record already holds it", name, err)
		}
		cur, err := m.Current()
		if err != nil || cur != res.Game {
			t.Fatalf("%s: current game replaced (%v)", name, err)
		}
		if snapshot(t, cur) != want {
			t.Errorf("%s: current game changed", name)
		}
		if got, err := m.Record(); err != nil || got != res.Record {
			t.Errorf("%s: current record changed (%v)", name, err)
		}
	}
	// Blank extras are not extras.
	if _, err := m.Import(ctx, ImportRequest{ID: "ok", Log: text, Deck: "  \n", Me: " "}); err != nil {
		t.Errorf("record with blank deck and name: %v", err)
	}
}

// TestRecordDamagedImport refuses a record whose values were changed and
// keeps the current game.
func TestRecordDamagedImport(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	ctx := context.Background()
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(ctx, ImportRequest{ID: "g", Log: logText, Deck: deckText, Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Record.JSON()
	damaged := strings.Replace(text, "3 Riolu MEG 76", "4 Riolu MEG 76", 1)
	if damaged == text {
		t.Fatal("deck count not found in the record")
	}
	if _, err := m.Import(ctx, ImportRequest{ID: "d", Log: damaged}); !errors.Is(err, ErrRecordDamaged) {
		t.Errorf("err %v, want ErrRecordDamaged", err)
	}
	if cur, err := m.Current(); err != nil || cur != res.Game {
		t.Fatalf("current game replaced (%v)", err)
	}
	if got, err := m.Record(); err != nil || got != res.Record {
		t.Errorf("current record changed (%v)", err)
	}
}

// TestNewGameHasNoRecord: a game started from two decklists has no record,
// and neither does a manager with no game.
func TestNewGameHasNoRecord(t *testing.T) {
	m, g := newTestGame(t)
	if _, ok := g.Record(); ok {
		t.Error("new game has a record")
	}
	if _, err := m.Record(); err == nil || !strings.Contains(err.Error(), "no record") {
		t.Errorf("err %v, want one saying the game has no record", err)
	}
	if _, err := NewManager(t.TempDir(), nil).Record(); err == nil {
		t.Error("manager with no game returned a record")
	}
}
