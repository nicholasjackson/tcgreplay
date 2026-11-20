package cardset

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// committedDir is the project's card-set directory, from this package.
var committedDir = filepath.Join("..", "..", DefaultDir)

// TestCommittedSets guards the card data kept in the project: every built
// set must pass the offline checks (no markup, English text everywhere,
// text lined up with the facts, corrections that resolve, no open item),
// so a bad hand edit fails the test suite.
func TestCommittedSets(t *testing.T) {
	if _, err := os.Stat(filepath.Join(committedDir, "sets")); os.IsNotExist(err) {
		t.Skip("no built sets in the project yet")
	}
	guardCommitted(t, committedDir)
}

// guardCommitted fails t for every open item in dir's built sets.
func guardCommitted(t testing.TB, dir string) {
	items, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		t.Errorf("open: %s", it)
	}
}

// recorder is a testing.TB that records failures instead of failing.
type recorder struct {
	testing.TB
	errs []string
}

func (r *recorder) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}
func (r *recorder) Fatal(args ...any) { r.errs = append(r.errs, fmt.Sprint(args...)) }

// The guard fails the suite on a committed set with a bad hand edit and
// passes a clean one.
func TestCommittedGuardCatchesBadEdits(t *testing.T) {
	clean := cleanDir(t)
	r := &recorder{TB: t}
	guardCommitted(r, clean)
	if len(r.errs) != 0 {
		t.Errorf("guard failed a clean set: %v", r.errs)
	}
	bad := cleanDir(t)
	b, _ := os.ReadFile(filepath.Join(bad, "sets", "tst.json"))
	os.WriteFile(filepath.Join(bad, "sets", "tst.json"), []byte(strings.Replace(string(b), `"name": "Riolu"`, `"name": "<em>Riolu</em>"`, 1)), 0o644)
	r = &recorder{TB: t}
	guardCommitted(r, bad)
	if len(r.errs) == 0 || !strings.Contains(strings.Join(r.errs, "\n"), "markup") {
		t.Errorf("guard passed a set with markup: %v", r.errs)
	}
}

// TestCommittedDeltaReign holds Delta Reign, the first set built with
// agent verification, to its acceptance criteria.
func TestCommittedDeltaReign(t *testing.T) {
	path := filepath.Join(committedDir, "sets", "me06.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("Delta Reign is not built yet")
	}
	s, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	corrs, err := ReadCorrections(filepath.Join(committedDir, "corrections", "me06.json"))
	if err != nil {
		t.Fatal(err)
	}
	acked := func(card, kind string) bool {
		for _, a := range corrs.Acknowledged {
			if a.Card == card && a.Kind == kind {
				return true
			}
		}
		return false
	}
	if len(s.Prints) != 141 {
		t.Errorf("Delta Reign has %d prints, want 141", len(s.Prints))
	}
	for _, p := range s.Prints {
		en := p.Text[English]
		if p.NeedsEffect() && en.Effect == "" && !acked(p.ID, KindMissingEffect) {
			t.Errorf("%s %s: no effect text", p.ID, en.Name)
		}
		if (p.Stage == "Stage1" || p.Stage == "Stage2") && en.EvolveFrom == "" {
			t.Errorf("%s %s: no evolves-from", p.ID, en.Name)
		}
		if p.RegulationMark == "" {
			t.Errorf("%s %s: no regulation mark", p.ID, en.Name)
		}
		if len(p.Images) == 0 && !acked(p.ID, KindNoImage) {
			t.Errorf("%s %s: no image source and no acknowledgement", p.ID, en.Name)
		}
	}
	// The Legendary Stadiums carry the pair rule neither source has, and
	// the pair mark; no other print is marked.
	legendary := 0
	for _, p := range s.Prints {
		en := p.Text[English]
		name, ok := strings.CutPrefix(en.Name, "Legendary ")
		if !ok || p.TrainerType != "Stadium" {
			if p.PlayedAsPair {
				t.Errorf("%s %s: marked as played as a pair, but is no Legendary Stadium", p.ID, en.Name)
			}
			continue
		}
		legendary++
		if !p.PlayedAsPair {
			t.Errorf("%s %s: not marked as played as a pair", p.ID, en.Name)
		}
		rule := "You cannot play this card by itself. You must combine 2 different Legendary " + name + " from your hand to play as 1 Stadium card."
		if !strings.Contains(en.Effect, rule) {
			t.Errorf("%s %s: effect %q lacks the pair rule", p.ID, en.Name, en.Effect)
		}
	}
	if legendary != 6 {
		t.Errorf("%d Legendary Stadium prints, want 6", legendary)
	}
	// The marks come from recorded corrections, with reasons.
	marked := 0
	for _, c := range corrs.Corrections {
		if c.Path == "/playedAsPair" && string(c.Value) == "true" && c.Reason != "" {
			marked++
		}
	}
	if marked != 6 {
		t.Errorf("%d pair-mark corrections with reasons, want 6", marked)
	}
}
