package catalog

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cardset"
)

// header is a built set's header for the fixtures.
type header struct {
	id, serie, release, abbr, online, name string
}

// pr is a print for the fixtures: a Pokémon with optional attacks, or a
// card of another category when cat is set.
type pr struct {
	local, name string
	art         bool
	attacks     []string
	cat, energy string
	hp          int
	effect      string
}

func builtSet(h header, prints ...pr) *cardset.Set {
	if h.name == "" {
		h.name = h.id + " set"
	}
	s := &cardset.Set{
		Format: cardset.Format,
		Set: cardset.SetHeader{ID: h.id, Serie: h.serie, ReleaseDate: h.release, Abbreviation: h.abbr, TCGOnline: h.online,
			Text: map[string]cardset.SetText{"en": {Name: h.name}}},
	}
	for _, p := range prints {
		cp := cardset.Print{ID: h.id + "-" + p.local, LocalID: p.local, Category: "Pokemon", HP: p.hp,
			Text: map[string]cardset.PrintText{"en": {Name: p.name, Effect: p.effect}}}
		if p.cat != "" {
			cp.Category, cp.EnergyType = p.cat, p.energy
		}
		if p.art {
			cp.Images = []cardset.Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/" + h.serie + "/" + h.id + "/" + p.local}}
		}
		t := cp.Text["en"]
		for _, a := range p.attacks {
			cp.Attacks = append(cp.Attacks, cardset.AttackCore{})
			t.Attacks = append(t.Attacks, cardset.MoveText{Name: a})
		}
		cp.Text["en"] = t
		s.Prints = append(s.Prints, cp)
	}
	return s
}

func snapshotOf(t *testing.T, sets ...*cardset.Set) *Snapshot {
	t.Helper()
	var ds []*setData
	for _, s := range sets {
		d, err := convertSet(s, "hash-"+s.Set.ID)
		if err != nil {
			t.Fatal(err)
		}
		ds = append(ds, d)
	}
	return NewSnapshot("v1", ds)
}

func mustResolve(t *testing.T, s *Snapshot, code, num, want string) {
	t.Helper()
	got, err := s.Resolve(context.Background(), code, num)
	if err != nil || got != want {
		t.Errorf("Resolve(%s, %s) = %q, %v; want %s", code, num, got, err, want)
	}
}

// Sets sharing a code (30th and its Classic Collection are both 30C, and
// here share a TCG Online code) resolve to the main set, whose id is
// shorter, whatever order the sets come in; the 30TH and M30 aliases name
// it too.
func TestSharedCodeKeepsMainSet(t *testing.T) {
	a := builtSet(header{id: "30th", serie: "me", release: "2026-09-01", abbr: "30C", online: "TC30"}, pr{local: "001", name: "Pikachu", art: true}, pr{local: "066", name: "Eevee", art: true})
	b := builtSet(header{id: "30th-c", serie: "me", release: "2026-09-01", abbr: "30C", online: "TC30"}, pr{local: "001", name: "Charizard", art: true})
	c := builtSet(header{id: "cel25", serie: "swsh", abbr: "CEL"}, pr{local: "001", name: "Ho-Oh", art: true})
	d := builtSet(header{id: "cel25cc", serie: "swsh", abbr: "CEL"}, pr{local: "001", name: "Venusaur", art: true})
	for _, s := range []*Snapshot{snapshotOf(t, a, b, c, d), snapshotOf(t, d, c, b, a)} {
		for _, code := range []string{"30C", "30c", "TC30"} {
			mustResolve(t, s, code, "1", "30th-001")
		}
		mustResolve(t, s, "CEL", "1", "cel25-001")
		for _, code := range []string{"30TH", "M30", "30th"} {
			mustResolve(t, s, code, "66", "30th-066")
		}
	}
}

// Prints by name: art first, then the newest release, then the lowest
// number (lettered prints after numbered ones), and Pokémon TCG Pocket
// prints never, though a Pocket set still resolves by code.
func TestPrintPreference(t *testing.T) {
	s := snapshotOf(t,
		builtSet(header{id: "sva", serie: "sv", release: "2023-01-01", abbr: "SVA"}, pr{local: "010", name: "Pikachu", art: true}),
		builtSet(header{id: "svb", serie: "sv", release: "2024-01-01", abbr: "SVB"},
			pr{local: "050", name: "Pikachu"},
			pr{local: "TG05", name: "Pikachu", art: true},
			pr{local: "020", name: "Pikachu", art: true},
			pr{local: "005", name: "Pikachu", art: true},
			pr{local: "030", name: "Fire Energy", art: true, cat: "Energy", energy: "Normal"}),
		builtSet(header{id: "tcgp1", serie: "tcgp", release: "2025-01-01", abbr: "TP1"}, pr{local: "001", name: "Pikachu", art: true}),
	)
	ids, err := s.prints("pikachu")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"svb-005", "svb-020", "svb-TG05", "sva-010", "svb-050"}; !slices.Equal(ids, want) {
		t.Errorf("Pikachu prints %v, want %v", ids, want)
	}
	for name, want := range map[string]string{"Pikachu": "svb-005", "Basic Fire Energy": "svb-030", "PIKACHU": "svb-005"} {
		if id, err := s.ByName(context.Background(), name); err != nil || id != want {
			t.Errorf("ByName(%s) = %q, %v; want %s", name, id, err, want)
		}
	}
	mustResolve(t, s, "TP1", "1", "tcgp1-001")
	mustResolve(t, s, "SVB", "5", "svb-005")
	mustResolve(t, s, "svb", "005", "svb-005")
	mustResolve(t, s, "SVB", "tg05", "svb-TG05")
	if _, err := s.Resolve(context.Background(), "SVB", "999"); !errors.Is(err, cards.ErrNotFound) {
		t.Errorf("missing number: %v", err)
	}
	if _, err := s.Resolve(context.Background(), "NOPE", "1"); !errors.Is(err, cards.ErrNotFound) {
		t.Errorf("unknown code: %v", err)
	}
}

// Apostrophe style and case do not matter to a name lookup.
func TestByNameApostrophes(t *testing.T) {
	s := snapshotOf(t, builtSet(header{id: "me01", serie: "me", release: "2025-09-26"}, pr{local: "100", name: "Lillie's Determination", art: true}))
	for _, name := range []string{"Lillie's Determination", "Lillie’s Determination", "lillie's  determination"} {
		if id, err := s.ByName(context.Background(), name); err != nil || id != "me01-100" {
			t.Errorf("ByName(%q) = %q, %v", name, id, err)
		}
	}
}

// Same-named cards are picked by the moves seen: the newest Chi-Yu is a
// Darkness Pokémon with Whirling Envy, but one seen using Ground Melter is
// the older Fire Chi-Yu.
func TestByNameMoves(t *testing.T) {
	s := snapshotOf(t,
		builtSet(header{id: "sv06", serie: "sv", release: "2024-05-24"}, pr{local: "039", name: "Chi-Yu", art: true, attacks: []string{"Allure", "Ground Melter"}}),
		builtSet(header{id: "me05", serie: "me", release: "2026-09-01"}, pr{local: "059", name: "Chi-Yu", art: true, attacks: []string{"Whirling Envy"}}),
	)
	for _, tc := range []struct {
		moves []string
		want  string
	}{
		{nil, "me05-059"},
		{[]string{"Ground Melter"}, "sv06-039"},
		{[]string{"Whirling Envy"}, "me05-059"},
		{[]string{"Borrowed Attack"}, "me05-059"},
		{[]string{"Allure", "Whirling Envy"}, "me05-059"},
	} {
		if got, err := s.ByName(context.Background(), "Chi-Yu", tc.moves...); err != nil || got != tc.want {
			t.Errorf("Chi-Yu using %v: %s, %v; want %s", tc.moves, got, err, tc.want)
		}
	}
}

// A Basic Energy print without art shows Crown Zenith's art of its type,
// as today; a special Energy borrows nothing.
func TestEnergyArtBorrowed(t *testing.T) {
	s := snapshotOf(t, builtSet(header{id: "sv01", serie: "sv"},
		pr{local: "257", name: "Basic Fighting Energy", cat: "Energy", energy: "Normal"},
		pr{local: "258", name: "Rocky Fighting Energy", cat: "Energy", energy: "Special", effect: "Prevents damage."}))
	if c := s.Cached("sv01-257"); c == nil || c.Image != "https://assets.tcgdex.net/en/swsh/swsh12.5/157" {
		t.Errorf("Basic Fighting Energy: %+v", c)
	}
	if c := s.Cached("sv01-258"); c == nil || c.Image != "" {
		t.Errorf("special Energy got art: %+v", c)
	}
}

// A Basic Energy print without art borrows the whole image list of the
// Crown Zenith print it takes its art from, when the snapshot holds it.
func TestEnergyArtBorrowsWholeList(t *testing.T) {
	const malie = "https://cdn.malie.io/file/malie-io/tcgl/cards/jpg/en/swsh12pt5/swsh12pt5_en_157_std.jpg"
	cz := builtSet(header{id: "swsh12.5", serie: "swsh", abbr: "CRZ"}, pr{local: "157", name: "Fighting Energy", art: true, cat: "Energy", energy: "Normal"})
	cz.Prints[0].Images = append(cz.Prints[0].Images, cardset.Image{Source: "malie", URL: malie})
	s := snapshotOf(t, cz, builtSet(header{id: "sv01", serie: "sv"}, pr{local: "257", name: "Basic Fighting Energy", cat: "Energy", energy: "Normal"}))
	want := []cards.ImageRef{{URL: "https://assets.tcgdex.net/en/swsh/swsh12.5/157"}, {URL: malie, File: true}}
	c := s.Cached("sv01-257")
	if c == nil || !slices.Equal(c.Images, want) || c.Image != want[0].URL {
		t.Errorf("borrowed list %+v", c)
	}
	// The set's own converted card is untouched (sets are shared between
	// snapshots).
	if got := s.sets["sv01"].cards[0].Images; len(got) != 1 {
		t.Errorf("shared set card changed: %+v", got)
	}
	// Without the Crown Zenith set, the borrowed base path alone.
	alone := snapshotOf(t, builtSet(header{id: "sv01", serie: "sv"}, pr{local: "257", name: "Basic Fighting Energy", cat: "Energy", energy: "Normal"}))
	if c := alone.Cached("sv01-257"); c == nil || !slices.Equal(c.Images, want[:1]) {
		t.Errorf("borrowed without the print: %+v", c)
	}
}

// Borrowed art does not count as art in a name lookup: a newer Basic
// Energy print showing borrowed art ranks below an older print with art of
// its own, as in today's catalogue, which ranks on the print's own image.
func TestBorrowedArtRanksBelowOwnArt(t *testing.T) {
	s := snapshotOf(t,
		builtSet(header{id: "swsh1", serie: "swsh", release: "2020-02-07"}, pr{local: "200", name: "Grass Energy", art: true, cat: "Energy", energy: "Normal"}),
		builtSet(header{id: "sv01", serie: "sv", release: "2023-03-31"}, pr{local: "250", name: "Basic Grass Energy", cat: "Energy", energy: "Normal"}, pr{local: "251", name: "Grass Energy", cat: "Energy", energy: "Normal"}),
	)
	if c := s.Cached("sv01-251"); c == nil || c.Image == "" {
		t.Fatalf("sv01-251 did not borrow art: %+v", c)
	}
	ids, err := s.prints("Grass Energy")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"swsh1-200", "sv01-251"}; !slices.Equal(ids, want) {
		t.Errorf("Grass Energy prints %v, want %v", ids, want)
	}
	if id, err := s.ByName(context.Background(), "Basic Grass Energy"); err != nil || id != "sv01-250" {
		t.Errorf("Basic Grass Energy = %q, %v; want the exact name's print sv01-250", id, err)
	}
}

func TestCardAndSetID(t *testing.T) {
	s := snapshotOf(t, builtSet(header{id: "me01", serie: "me", abbr: "MEG", name: "Mega Evolution"}, pr{local: "001", name: "Bulbasaur", art: true, hp: 70}))
	c, err := s.Card(context.Background(), "me01-001")
	if err != nil || c.Name != "Bulbasaur" || c.HP != 70 || c.Set != (cards.SetRef{ID: "me01", Name: "Mega Evolution"}) {
		t.Errorf("Card = %+v, %v", c, err)
	}
	if _, err := s.Card(context.Background(), "me01-999"); !errors.Is(err, cards.ErrNotFound) {
		t.Errorf("missing card: %v", err)
	}
	for _, name := range []string{"MEG", "meg", "me01", "ME01", " me01 "} {
		id, setName, err := s.SetID(name)
		if err != nil || id != "me01" || setName != "Mega Evolution" {
			t.Errorf("SetID(%q) = %q, %q, %v", name, id, setName, err)
		}
	}
	for _, name := range []string{"XYZ", ""} {
		if _, _, err := s.SetID(name); !errors.Is(err, cards.ErrUnknownSet) {
			t.Errorf("SetID(%q) = %v, want ErrUnknownSet", name, err)
		}
	}
	if got := s.Sets(); len(got) != 1 || got[0] != (SetInfo{ID: "me01", Name: "Mega Evolution", Hash: "hash-me01", Prints: 1}) {
		t.Errorf("Sets = %+v", got)
	}
	if s.Version() != "v1" || s.Prints() != 1 {
		t.Errorf("Version %q, Prints %d", s.Version(), s.Prints())
	}
}

// A snapshot built after one set changed shares every other set with the
// previous one, and the previous snapshot still answers with the old data.
func TestNextSharesUnchangedSets(t *testing.T) {
	a := builtSet(header{id: "sva", serie: "sv", abbr: "SVA"}, pr{local: "001", name: "Pikachu", art: true, hp: 60})
	b := builtSet(header{id: "svb", serie: "sv", abbr: "SVB"}, pr{local: "001", name: "Eevee", art: true, hp: 50})
	c := builtSet(header{id: "svc", serie: "sv", abbr: "SVC"}, pr{local: "001", name: "Mew", art: true, hp: 70})
	old := snapshotOf(t, a, b, c)

	a2 := builtSet(header{id: "sva", serie: "sv", abbr: "SVA"}, pr{local: "001", name: "Pikachu", art: true, hp: 90})
	d, err := convertSet(a2, "hash-sva-2")
	if err != nil {
		t.Fatal(err)
	}
	next := Next(old, "v2", map[string]*setData{"sva": d}, []string{"svc"})

	if next.sets["svb"] != old.sets["svb"] {
		t.Error("unchanged set svb not shared")
	}
	if next.Cached("svb-001") != old.Cached("svb-001") {
		t.Error("unchanged card not shared")
	}
	if got := next.Cached("sva-001"); got == nil || got.HP != 90 {
		t.Errorf("new snapshot sva-001: %+v", got)
	}
	if got := old.Cached("sva-001"); got == nil || got.HP != 60 {
		t.Errorf("old snapshot sva-001 changed: %+v", got)
	}
	if next.Cached("svc-001") != nil {
		t.Error("removed set still answers")
	}
	if old.Cached("svc-001") == nil {
		t.Error("old snapshot lost the removed set")
	}
	if _, err := next.Resolve(context.Background(), "SVC", "1"); !errors.Is(err, cards.ErrNotFound) {
		t.Errorf("removed set's code: %v", err)
	}
	if next.Version() != "v2" || old.Version() != "v1" {
		t.Error("versions")
	}
}

func TestEmptySnapshot(t *testing.T) {
	ctx := context.Background()
	for name, s := range map[string]*Snapshot{"empty": Empty, "nil": nil, "no sets": NewSnapshot("", nil)} {
		if _, err := s.Card(ctx, "sv01-001"); !errors.Is(err, cards.ErrNotFound) {
			t.Errorf("%s Card: %v", name, err)
		}
		if s.Cached("sv01-001") != nil {
			t.Errorf("%s Cached", name)
		}
		if _, err := s.Resolve(ctx, "SVI", "1"); !errors.Is(err, cards.ErrNotFound) {
			t.Errorf("%s Resolve: %v", name, err)
		}
		if _, err := s.Resolve(ctx, "M30", "1"); !errors.Is(err, cards.ErrNotFound) {
			t.Errorf("%s Resolve alias: %v", name, err)
		}
		if _, err := s.ByName(ctx, "Pikachu"); !errors.Is(err, cards.ErrNotFound) {
			t.Errorf("%s ByName: %v", name, err)
		}
		if _, _, err := s.SetID("sv01"); !errors.Is(err, cards.ErrUnknownSet) {
			t.Errorf("%s SetID: %v", name, err)
		}
		if s.Version() != "" || len(s.Sets()) != 0 || s.Prints() != 0 {
			t.Errorf("%s reports content", name)
		}
	}
}
