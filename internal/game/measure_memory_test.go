package game_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// TestMemoryPerGame measures the heap one open game adds: a full imported
// battle log with an opponent list and two extra branches, each with a few
// moves, in its own manager, as each hosted account has. Every game shares
// one card catalogue, as hosted mode does, so the catalogue's snapshot is
// not counted per game. It reports a figure and never fails on one.
//
// It runs only when asked for:
//
//	TCGREPLAY_MEASURE_MEMORY=1 go test -run TestMemoryPerGame -v ./internal/game/
//
// TCGREPLAY_MEASURE_GAMES sets how many games are held open at once
// (default 10).
func TestMemoryPerGame(t *testing.T) {
	if os.Getenv("TCGREPLAY_MEASURE_MEMORY") != "1" {
		t.Skip("set TCGREPLAY_MEASURE_MEMORY=1 to measure the memory one game uses")
	}
	n := 10
	if v := os.Getenv("TCGREPLAY_MEASURE_GAMES"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil || n < 1 {
			t.Fatalf("TCGREPLAY_MEASURE_GAMES=%q: want a whole number above 0", v)
		}
	}
	deck, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Fatal(err)
	}
	// A mirror match, so the corpus deck doubles as the opponent's list.
	logText, err := os.ReadFile("../../testdata/logs/2026-10-11_lukiees_mirror.txt")
	if err != nil {
		t.Fatal(err)
	}
	cat := committedCatalog(t)
	dir := t.TempDir()

	open := func(i int) *game.Manager {
		m := game.NewManager(filepath.Join(dir, strconv.Itoa(i)), cat)
		res, err := m.Import(context.Background(), game.ImportRequest{
			ID: fmt.Sprintf("measure-%d", i), Log: string(logText), Deck: string(deck), OppDeck: string(deck), Seed: uint64(i + 1),
		})
		if err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
		g := res.Game
		head := g.Head()
		for b, at := range []int{head / 3, 2 * head / 3} {
			if _, err := g.Rewind(at, fmt.Sprintf("line-%d", b+1)); err != nil {
				t.Fatalf("branch at %d: %v", at, err)
			}
			// A few moves on the branch; a refused move still leaves the
			// branch, which is what is measured.
			for _, seat := range []state.Seat{state.P1, state.P2, state.P1} {
				g.Draw(seat, state.ActorHuman, 1)
			}
		}
		return m
	}

	heap := func() uint64 {
		runtime.GC()
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return ms.HeapInuse
	}
	// One game first, so one-off costs (the catalogue's snapshot, parsers,
	// caches) are paid before the baseline.
	warm := open(-1)
	before := heap()
	games := make([]*game.Manager, n)
	for i := range games {
		games[i] = open(i)
	}
	after := heap()
	runtime.KeepAlive(games)
	runtime.KeepAlive(warm)

	per := float64(after-before) / float64(n)
	if after < before {
		per = 0
	}
	const mib = 1 << 20
	t.Logf("%d games: heap in use %.1f MiB before, %.1f MiB after", n, float64(before)/mib, float64(after)/mib)
	t.Logf("average per game (a full imported game with an opponent list and two branches): %.0f bytes (%.2f MiB)", per, per/mib)
	t.Logf("baseline heap with one game open: %.1f MiB", float64(before)/mib)
	for _, size := range []struct {
		name  string
		bytes float64
	}{{"512 MiB", 512 * mib}, {"1 GiB", 1024 * mib}} {
		budget := 0.7*size.bytes - float64(before)
		fits := 0
		if per > 0 && budget > 0 {
			fits = int(budget / per)
		}
		t.Logf("games that fit in %s at 70%% use, after the baseline: %d", size.name, fits)
	}
}
