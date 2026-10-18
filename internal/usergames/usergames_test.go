package usergames

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
)

// fakeDB serves a tiny card pool with set code TST (copied from the webui
// package tests).
type fakeDB map[string]*cards.Card

func newFakeDB() fakeDB {
	return fakeDB{
		"tst-1":   {ID: "tst-1", Name: "Riolu", Category: "Pokemon", Stage: "Basic", HP: 70},
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

// fakeClock is a settable clock for the registry.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

var (
	alice = hostauth.Account{ID: "google-sub-alice", Email: "alice@example.com"}
	bob   = hostauth.Account{ID: "google-sub-bob", Email: "bob@example.com"}
)

// newRegistry returns a registry whose managers share dir and the fake card
// DB, with the given clock (nil for time.Now).
func newRegistry(t *testing.T, dir string, now func() time.Time) *Registry {
	t.Helper()
	db := newFakeDB()
	public, _ := url.Parse("https://playmat.example.com")
	return New(Config{
		NewManager: func() *game.Manager { return game.NewManager(dir, db) },
		NewBoard: func(m *game.Manager) *webui.Server {
			ui := webui.New(m, state.P1)
			ui.Public = public
			return ui
		},
		Public: public,
		Now:    now,
	})
}

func use(t *testing.T, r *Registry, acct hostauth.Account) *Entry {
	t.Helper()
	e, err := r.Use(acct)
	if err != nil {
		t.Fatalf("Use(%s): %v", acct.ID, err)
	}
	return e
}

func TestAccountsGetTheirOwnGamesAndBoards(t *testing.T) {
	r := newRegistry(t, t.TempDir(), nil)

	a, b := use(t, r, alice), use(t, r, bob)
	if a.M == b.M {
		t.Error("two accounts share a game manager")
	}
	if a.UI == b.UI {
		t.Error("two accounts share a board server")
	}
	if a.Handle == b.Handle {
		t.Errorf("two accounts share handle %q", a.Handle)
	}

	for _, again := range []*Entry{use(t, r, alice), r.Get(alice)} {
		if again != a || again.M != a.M || again.UI != a.UI || again.Handle != a.Handle {
			t.Error("the same account got a different game or board on a later lookup")
		}
	}
}

func TestHandleIsStableAndOpaque(t *testing.T) {
	// Computed independently: base64url(sha256("tcgreplay board\x00google-sub-123")[:12]).
	if got, want := Handle("google-sub-123"), "TDH3Mf8J6vckmara"; got != want {
		t.Errorf("Handle(google-sub-123) = %q, want %q", got, want)
	}

	first := use(t, newRegistry(t, t.TempDir(), nil), alice).Handle
	restarted := use(t, newRegistry(t, t.TempDir(), nil), alice).Handle
	if first != restarted {
		t.Errorf("handle changed across registries: %q then %q", first, restarted)
	}
	if first == Handle(bob.ID) {
		t.Error("two accounts share a handle")
	}

	urlSafe := regexp.MustCompile(`^[A-Za-z0-9_-]{16}$`)
	for _, acct := range []hostauth.Account{alice, bob} {
		h := Handle(acct.ID)
		if !urlSafe.MatchString(h) {
			t.Errorf("handle %q is not 16 URL-safe characters", h)
		}
		if strings.Contains(h, acct.ID) || strings.Contains(h, acct.Email) ||
			strings.Contains(acct.ID, h) {
			t.Errorf("handle %q reveals account %+v", h, acct)
		}
	}
}

func TestUseRecordsActivityGetDoesNot(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := newRegistry(t, t.TempDir(), clock.Now)

	e := use(t, r, alice)
	if got := r.LastActive(e); !got.Equal(clock.t) {
		t.Fatalf("after first Use, LastActive = %v, want %v", got, clock.t)
	}
	used := clock.t

	clock.Advance(10 * time.Minute)
	r.Get(alice)
	if got := r.LastActive(e); !got.Equal(used) {
		t.Errorf("Get changed LastActive to %v, want %v", got, used)
	}

	clock.Advance(5 * time.Minute)
	use(t, r, alice)
	if got := r.LastActive(e); !got.Equal(clock.t) {
		t.Errorf("after second Use, LastActive = %v, want %v", got, clock.t)
	}
}

func TestGetCreatesANewAccount(t *testing.T) {
	r := newRegistry(t, t.TempDir(), nil)

	e := r.Get(bob)
	if e == nil || e.M == nil || e.UI == nil {
		t.Fatalf("Get on a new account returned %+v", e)
	}
	if e.Handle != Handle(bob.ID) {
		t.Errorf("Handle = %q, want %q", e.Handle, Handle(bob.ID))
	}
	if again := use(t, r, bob); again != e {
		t.Error("Use after Get returned a different entry")
	}
}

func TestLink(t *testing.T) {
	r := newRegistry(t, t.TempDir(), nil)
	e := use(t, r, alice)
	public, _ := url.Parse("https://playmat.example.com")

	want := "https://playmat.example.com/?board=" + Handle(alice.ID)
	if got := e.Link(public); got != want {
		t.Errorf("Link = %q, want %q", got, want)
	}
}

func TestGamesAreNotWrittenToDisk(t *testing.T) {
	dir := t.TempDir()
	r := newRegistry(t, dir, nil)
	e := use(t, r, alice)

	if _, _, err := e.M.New(context.Background(), game.NewGameRequest{ID: "x", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range entries {
		if f.Name() == "assistant.txt" {
			continue
		}
		t.Errorf("data dir holds %q after starting a game", f.Name())
	}
}
