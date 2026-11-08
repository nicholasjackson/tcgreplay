// Package cardset builds card sets ahead of time into the playmat's own
// card-set format, and reads and writes that format.
//
// A built set is one JSON file per set: a header and a list of prints. Each
// print holds its language-independent facts (HP, types, attack costs and
// damage, retreat, stage, image sources) once, and its wording in a text
// block per language, so wording in another language can be added without
// reshaping the set. English wording is required for every print: game
// logic reads it.
//
// The files are written deterministically (fixed key order, prints sorted
// by collector number, two-space indent, trailing newline), so a change to
// one card diffs as that card's lines only.
package cardset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// Format is the built-set format version. It is bumped only on a breaking
// change; adding an optional field does not bump it.
const Format = 1

// English is the language every print must have wording in.
const English = "en"

// Set is a built card set.
type Set struct {
	Format int       `json:"format"`
	Set    SetHeader `json:"set"`
	Prints []Print   `json:"prints"` // sorted by collector number
}

// SetHeader describes the set itself.
type SetHeader struct {
	ID           string             `json:"id"`    // TCGdex set id, "me06"
	Serie        string             `json:"serie"` // "me"
	ReleaseDate  string             `json:"releaseDate,omitempty"`
	Abbreviation string             `json:"abbreviation,omitempty"` // decklist code, "DLR"
	TCGOnline    string             `json:"tcgOnline,omitempty"`
	Text         map[string]SetText `json:"text"` // "en" required
}

// SetText is a set's wording in one language.
type SetText struct {
	Name string `json:"name"`
}

// Print is one print of a card: its language-independent core and its
// wording per language. Attacks and abilities in the core and in every
// language's text are lined up by position.
type Print struct {
	ID             string               `json:"id"`       // "me06-001"
	LocalID        string               `json:"localId"`  // collector number, "001"
	Category       string               `json:"category"` // Pokemon, Trainer, Energy
	HP             int                  `json:"hp,omitempty"`
	Types          []string             `json:"types,omitempty"`
	Stage          string               `json:"stage,omitempty"`
	Suffix         string               `json:"suffix,omitempty"`
	Attacks        []AttackCore         `json:"attacks,omitempty"`
	Abilities      []AbilityCore        `json:"abilities,omitempty"`
	Weaknesses     []cards.TypeValue    `json:"weaknesses,omitempty"`
	Resistances    []cards.TypeValue    `json:"resistances,omitempty"`
	Retreat        int                  `json:"retreat,omitempty"`
	TrainerType    string               `json:"trainerType,omitempty"` // Item, Supporter, Stadium, Tool
	EnergyType     string               `json:"energyType,omitempty"`  // Normal, Special
	RegulationMark string               `json:"regulationMark,omitempty"`
	PlayedAsPair   bool                 `json:"playedAsPair,omitempty"` // two different prints of this name are played together as one card
	Images         []Image              `json:"images,omitempty"`       // preferred first
	Sources        []string             `json:"sources,omitempty"`      // upstream sources that list the print, sorted
	Text           map[string]PrintText `json:"text"`                   // "en" required
}

// AttackCore is the language-independent part of an attack.
type AttackCore struct {
	Cost   []string     `json:"cost,omitempty"`
	Damage cards.Damage `json:"damage,omitempty"`
}

// AbilityCore is the language-independent part of an ability: its kind
// ("Ability", "Poké-Power", ...).
type AbilityCore struct {
	Type string `json:"type"`
}

// Image is one source of a print's image.
type Image struct {
	Source string `json:"source"` // "tcgdex", "malie", ...
	URL    string `json:"url"`
}

// PrintText is a print's wording in one language.
type PrintText struct {
	Name       string     `json:"name"`
	EvolveFrom string     `json:"evolveFrom,omitempty"`
	Attacks    []MoveText `json:"attacks,omitempty"`   // same order as the core's
	Abilities  []MoveText `json:"abilities,omitempty"` // same order as the core's
	Effect     string     `json:"effect,omitempty"`
}

// MoveText is the wording of an attack or ability.
type MoveText struct {
	Name   string `json:"name"`
	Effect string `json:"effect,omitempty"`
}

// Read reads a built set, refusing unknown fields, and checks its shape
// (ValidateShape).
func Read(path string) (*Set, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s, err := Decode(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// Decode parses a built set's bytes, refusing unknown fields, and checks
// its shape (ValidateShape). A set whose English text has gaps still
// decodes, so the import and the checks can read it and report the gaps;
// Validate refuses it.
func Decode(b []byte) (*Set, error) {
	var s Set
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return nil, err
	}
	if err := s.ValidateShape(); err != nil {
		return nil, err
	}
	return &s, nil
}

// Write writes a built set deterministically: prints sorted by collector
// number, fixed key order, two-space indent and a trailing newline. It
// writes a temporary file beside path and renames it, so a failed write
// never leaves half a set.
func Write(path string, s *Set) error {
	b, err := Marshal(s)
	if err != nil {
		return err
	}
	return writeFile(path, b)
}

// Marshal returns the deterministic bytes Write writes. It sorts s's
// prints in place.
func Marshal(s *Set) ([]byte, error) {
	if s.Prints == nil {
		s.Prints = []Print{} // an empty set writes "prints": [], not null
	}
	SortPrints(s.Prints)
	return marshal(s)
}

// marshal encodes v as two-space-indented JSON with a trailing newline,
// leaving characters such as ×, é and & literal.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeFile writes b to path through a temporary file in the same
// directory, creating the directory if needed.
func writeFile(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// SortPrints sorts prints by collector number: numbered prints in numeric
// order, then lettered ones (TG05, SV001) in string order, ties by id.
func SortPrints(ps []Print) {
	slices.SortStableFunc(ps, func(a, b Print) int {
		na, ea := strconv.Atoi(a.LocalID)
		nb, eb := strconv.Atoi(b.LocalID)
		switch {
		case ea == nil && eb == nil && na != nb:
			if na < nb {
				return -1
			}
			return 1
		case ea == nil && eb != nil:
			return -1
		case ea != nil && eb == nil:
			return 1
		}
		if c := strings.Compare(a.LocalID, b.LocalID); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

// Validate checks a set is valid: well formed (ValidateShape), and every
// print has English names for its attacks and abilities and English text
// for each ability and for the effect its kind of card carries
// (MissingEnglish).
func (s *Set) Validate() error {
	errs := []error{s.ValidateShape()}
	for i := range s.Prints {
		p := &s.Prints[i]
		for _, m := range p.MissingEnglish() {
			errs = append(errs, fmt.Errorf("print %s: no English text for %s", p.ID, m))
		}
	}
	return errors.Join(errs...)
}

// ValidateShape checks a set is well formed: it has an English name, every
// print has an id and English wording with a name, every language's attacks
// and abilities line up with the core (and are named, in languages other
// than English, whose gaps MissingEnglish reports), and no language holds
// wording English lacks.
func (s *Set) ValidateShape() error {
	var errs []error
	if s.Set.ID == "" {
		errs = append(errs, errors.New("set has no id"))
	}
	if strings.TrimSpace(s.Set.Text[English].Name) == "" {
		errs = append(errs, fmt.Errorf("set %s has no English name", s.Set.ID))
	}
	seen := make(map[string]bool, len(s.Prints))
	for i := range s.Prints {
		p := &s.Prints[i]
		if p.ID == "" {
			errs = append(errs, fmt.Errorf("print %d has no id", i))
			continue
		}
		if seen[p.ID] {
			errs = append(errs, fmt.Errorf("print %s: listed twice", p.ID))
		}
		seen[p.ID] = true
		if err := p.ValidateShape(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// MissingEnglish lists the English text the print lacks, as JSON Pointer
// paths into the print: an attack's or ability's name, an ability's effect
// (every ability has text), and the effect of a Trainer or a special
// Energy. Attack effects are optional: many attacks only do damage.
func (p *Print) MissingEnglish() []string {
	en := p.Text[English]
	var missing []string
	for i, a := range en.Attacks {
		if strings.TrimSpace(a.Name) == "" {
			missing = append(missing, fmt.Sprintf("/text/en/attacks/%d/name", i))
		}
	}
	for i, a := range en.Abilities {
		if strings.TrimSpace(a.Name) == "" {
			missing = append(missing, fmt.Sprintf("/text/en/abilities/%d/name", i))
		}
		if strings.TrimSpace(a.Effect) == "" {
			missing = append(missing, fmt.Sprintf("/text/en/abilities/%d/effect", i))
		}
	}
	if p.NeedsEffect() && strings.TrimSpace(en.Effect) == "" {
		missing = append(missing, "/text/en/effect")
	}
	return missing
}

// NeedsEffect reports whether the print is a kind of card whose own text is
// its effect: a Trainer or a special Energy.
func (p *Print) NeedsEffect() bool {
	return p.Category == "Trainer" || (p.Category == "Energy" && p.EnergyType == "Special")
}

// ValidateShape checks one print's wording against its core.
func (p *Print) ValidateShape() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("print %s: "+format, append([]any{p.ID}, args...)...))
	}
	en, ok := p.Text[English]
	if !ok {
		bad("no English text")
	} else if strings.TrimSpace(en.Name) == "" {
		bad("no English name")
	}
	for _, lang := range sortedKeys(p.Text) {
		t := p.Text[lang]
		if len(t.Attacks) != len(p.Attacks) {
			bad("%s text has %d attacks, the card has %d", lang, len(t.Attacks), len(p.Attacks))
		}
		if len(t.Abilities) != len(p.Abilities) {
			bad("%s text has %d abilities, the card has %d", lang, len(t.Abilities), len(p.Abilities))
		}
		if lang == English || !ok {
			// A missing English attack or ability name is a gap the import
			// reports (MissingEnglish), not a malformed set.
			continue
		}
		for i, a := range t.Attacks {
			if strings.TrimSpace(a.Name) == "" {
				bad("%s attack %d has no name", lang, i)
			}
		}
		for i, a := range t.Abilities {
			if strings.TrimSpace(a.Name) == "" {
				bad("%s ability %d has no name", lang, i)
			}
		}
		// English is what game logic reads, so wording another language
		// has must exist in English too.
		if t.EvolveFrom != "" && en.EvolveFrom == "" {
			bad("%s text has evolves-from, English has none", lang)
		}
		if t.Effect != "" && en.Effect == "" {
			bad("%s text has an effect, English has none", lang)
		}
		for i, a := range t.Attacks {
			if a.Effect != "" && i < len(en.Attacks) && en.Attacks[i].Effect == "" {
				bad("%s attack %d has effect text, English has none", lang, i)
			}
		}
		for i, a := range t.Abilities {
			if a.Effect != "" && i < len(en.Abilities) && en.Abilities[i].Effect == "" {
				bad("%s ability %d has effect text, English has none", lang, i)
			}
		}
	}
	return errors.Join(errs...)
}

// Print returns the print with the given id.
func (s *Set) Print(id string) (*Print, bool) {
	for i := range s.Prints {
		if s.Prints[i].ID == id {
			return &s.Prints[i], true
		}
	}
	return nil, false
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}
