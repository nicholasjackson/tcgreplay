package game

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const testNotice = "Your game expired after an hour idle: import its saved record again with import_log."

// TestFreshManagerHasNoNotice: a manager whose game never expired reports
// the usual no-game error, which does not match ErrExpired, and no notice.
func TestFreshManagerHasNoNotice(t *testing.T) {
	m := NewManager(t.TempDir(), newFakeDB())
	if n := m.Notice(); n != "" {
		t.Errorf("Notice() = %q, want empty", n)
	}
	_, err := m.Current()
	if err == nil || !strings.HasPrefix(err.Error(), "no game open") {
		t.Fatalf("Current() error = %v, want the no game open error", err)
	}
	if errors.Is(err, ErrExpired) {
		t.Errorf("Current() error %v matches ErrExpired before any expiry", err)
	}

	// An open game that never expired: no notice, Current works.
	if _, _, err := m.New(context.Background(), NewGameRequest{ID: "t1", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
		t.Fatal(err)
	}
	if n := m.Notice(); n != "" {
		t.Errorf("Notice() with a game open = %q, want empty", n)
	}
	if _, err := m.Current(); err != nil {
		t.Errorf("Current() with a game open: %v", err)
	}
}

// TestExpireReportsNotice: after Expire, Current and everything that needs
// the game fail with the notice as the error text, matching ErrExpired, and
// OnClear hooks run.
func TestExpireReportsNotice(t *testing.T) {
	m, _ := newTestGame(t)
	cleared := 0
	m.OnClear(func() { cleared++ })
	loaded := 0
	m.OnLoad(func(*Game) { loaded++ })

	m.Expire(testNotice)

	if cleared != 1 {
		t.Errorf("OnClear ran %d times, want 1", cleared)
	}
	if loaded != 0 {
		t.Errorf("OnLoad ran %d times on Expire, want 0", loaded)
	}
	if n := m.Notice(); n != testNotice {
		t.Errorf("Notice() = %q, want %q", n, testNotice)
	}
	g, err := m.Current()
	if g != nil || err == nil {
		t.Fatalf("Current() after Expire = %v, %v; want no game and an error", g, err)
	}
	if err.Error() != testNotice {
		t.Errorf("Current() error = %q, want the notice %q", err.Error(), testNotice)
	}
	if !errors.Is(err, ErrExpired) {
		t.Errorf("Current() error %v does not match ErrExpired", err)
	}

	// Callers that need the game report the same.
	if _, err := m.Record(); err == nil || err.Error() != testNotice || !errors.Is(err, ErrExpired) {
		t.Errorf("Record() after Expire: %v, want the notice matching ErrExpired", err)
	}
	if _, err := m.SetOpponentDeck(context.Background(), testDeck, false); err == nil || !errors.Is(err, ErrExpired) || !strings.Contains(err.Error(), testNotice) {
		t.Errorf("SetOpponentDeck() after Expire: %v, want the notice matching ErrExpired", err)
	}
}

// TestExpireWithNoGame: expiring a manager with no game still sets the
// notice and tells the OnClear listeners.
func TestExpireWithNoGame(t *testing.T) {
	m := NewManager(t.TempDir(), newFakeDB())
	cleared := 0
	m.OnClear(func() { cleared++ })
	m.Expire(testNotice)
	if cleared != 1 {
		t.Errorf("OnClear ran %d times, want 1", cleared)
	}
	if n := m.Notice(); n != testNotice {
		t.Errorf("Notice() = %q, want %q", n, testNotice)
	}
	if _, err := m.Current(); err == nil || err.Error() != testNotice || !errors.Is(err, ErrExpired) {
		t.Errorf("Current() = %v, want the notice matching ErrExpired", err)
	}
}

// TestNewGameClearsNotice: starting a game after expiry works as usual and
// clears the notice.
func TestNewGameClearsNotice(t *testing.T) {
	m, old := newTestGame(t)
	m.Expire(testNotice)
	var got *Game
	m.OnLoad(func(g *Game) { got = g })

	g, _, err := m.New(context.Background(), NewGameRequest{ID: "t2", DeckP1: testDeck, DeckP2: testDeck})
	if err != nil {
		t.Fatal(err)
	}
	if got != g {
		t.Errorf("OnLoad got %p, want the new game %p", got, g)
	}
	cur, err := m.Current()
	if err != nil || cur != g {
		t.Fatalf("Current() = %v, %v; want the new game", cur, err)
	}
	if cur == old {
		t.Error("Current() is the expired game")
	}
	if n := m.Notice(); n != "" {
		t.Errorf("Notice() after New = %q, want empty", n)
	}
	// Expiring again and starting again keeps working.
	m.Expire("again")
	if n := m.Notice(); n != "again" {
		t.Errorf("Notice() after a second Expire = %q, want again", n)
	}
}

// TestImportClearsNotice: importing a battle log after expiry works as
// usual and clears the notice.
func TestImportClearsNotice(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	m := NewManager(t.TempDir(), nil)
	m.Expire(testNotice)

	res, err := m.Import(context.Background(), ImportRequest{ID: "g", Log: logText, Deck: deckText, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	cur, err := m.Current()
	if err != nil || cur != res.Game {
		t.Fatalf("Current() after Import = %v, %v; want the imported game", cur, err)
	}
	if n := m.Notice(); n != "" {
		t.Errorf("Notice() after Import = %q, want empty", n)
	}
	if _, err := m.Record(); err != nil {
		t.Errorf("Record() after Import: %v", err)
	}
}
