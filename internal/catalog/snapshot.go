// Package catalog is how the playmat obtains and serves card data: it
// loads the card sets published from the repository (see internal/cardset)
// into immutable snapshots, each a complete card database, and keeps the
// current one up to date. A lookup is a map read that never blocks, fails
// transiently or changes under its caller; a game keeps the snapshot it
// started with for its whole life.
package catalog

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cardset"
)

// Snapshot is an immutable, fully converted catalogue built from loaded
// card sets. It implements cards.DB. Nothing in it changes once built, and
// a nil or empty snapshot answers every lookup as not found.
type Snapshot struct {
	version  string
	sets     map[string]*setData    // set id -> its converted cards
	cards    map[string]*cards.Card // card id -> card
	setIndex map[string]string      // upper set code -> set id
	byName   map[string][]string    // name key -> card ids, preferred print first
}

// setData is one loaded set, converted. It is shared, unchanged, between
// every snapshot that holds the same version of the set.
type setData struct {
	id     string
	hash   string // sha256 of the built set file it came from
	header cardset.SetHeader
	cards  []*cards.Card // in the built set's order
	// art reports, per card in cards, whether the print has art of its
	// own: a Basic Energy showing borrowed art does not, so it ranks
	// below prints with art in a name lookup, as it always has.
	art []bool
}

// SetInfo describes one set in a snapshot.
type SetInfo struct {
	ID     string
	Name   string
	Hash   string
	Prints int
}

// Empty is the snapshot with no sets: every card is unknown.
var Empty = &Snapshot{}

var _ cards.DB = (*Snapshot)(nil)

// convertSet converts a built set's prints into runtime cards, in English
// (the language game logic reads). Print.Card gives a Basic Energy without
// art the borrowed art the playmat has always shown.
func convertSet(s *cardset.Set, hash string) (*setData, error) {
	name := s.Set.Text[cardset.English].Name
	d := &setData{id: s.Set.ID, hash: hash, header: s.Set, cards: make([]*cards.Card, 0, len(s.Prints)), art: make([]bool, 0, len(s.Prints))}
	for _, p := range s.Prints {
		c, err := p.Card(cardset.English)
		if err != nil {
			return nil, fmt.Errorf("set %s: %w", s.Set.ID, err)
		}
		c.Set = cards.SetRef{ID: s.Set.ID, Name: name}
		d.cards = append(d.cards, c)
		d.art = append(d.art, c.Image != "" && slices.ContainsFunc(p.Images, func(img cardset.Image) bool { return img.URL == c.Image }))
	}
	return d, nil
}

// NewSnapshot builds a snapshot of the given sets, tagged with version
// (the sha256 of the manifest they came from; "" for none).
func NewSnapshot(version string, sets []*setData) *Snapshot {
	m := make(map[string]*setData, len(sets))
	for _, d := range sets {
		m[d.id] = d
	}
	return build(version, m)
}

// Next builds the snapshot that follows prev: prev's sets, minus removed,
// with changed replacing or adding sets. Every set not changed or removed
// is the very same *setData prev holds, so an older snapshot still pinned
// by an open game costs only the sets that changed.
func Next(prev *Snapshot, version string, changed map[string]*setData, removed []string) *Snapshot {
	m := make(map[string]*setData, len(changed))
	if prev != nil {
		maps.Copy(m, prev.sets)
	}
	for _, id := range removed {
		delete(m, id)
	}
	maps.Copy(m, changed)
	return build(version, m)
}

func build(version string, sets map[string]*setData) *Snapshot {
	s := &Snapshot{version: version, sets: sets, cards: map[string]*cards.Card{}}
	for _, d := range sets {
		for _, c := range d.cards {
			s.cards[c.ID] = c
		}
	}
	// A card showing borrowed art (a Basic Energy without its own) takes
	// the whole image list of the print it borrows from, when this
	// snapshot holds that print. The card is copied, never changed: its
	// set may be shared with snapshots holding another version of the
	// print it borrows from.
	for _, d := range sets {
		for i, c := range d.cards {
			if d.art[i] || c.Image == "" {
				continue
			}
			from := s.cards[borrowedID(c.Image)]
			if from == nil || from == c || len(from.ImageList()) == 0 {
				continue
			}
			cc := *c
			cc.Images = from.ImageList()
			cc.Image = cc.Images[0].URL
			s.cards[c.ID] = &cc
		}
	}
	s.setIndex, s.byName = indexes(sets)
	return s
}

// borrowedID is the card id of the print a borrowed TCGdex image address
// belongs to: https://assets.tcgdex.net/en/swsh/swsh12.5/157 is
// swsh12.5-157.
func borrowedID(addr string) string {
	parts := strings.Split(strings.TrimSuffix(addr, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] + "-" + parts[len(parts)-1]
}

// indexes derives the set-code index and the by-name index from the sets.
// It is pure, so the same sets always give the same indexes.
func indexes(sets map[string]*setData) (map[string]string, map[string][]string) {
	setIndex := map[string]string{}
	type pick struct {
		id, release string
		art         bool
		num         int
	}
	better := func(a, b pick) bool { // is a preferred to b?
		if a.art != b.art {
			return a.art
		}
		if a.release != b.release {
			return a.release > b.release
		}
		return a.num < b.num
	}
	all := map[string][]pick{}
	for _, d := range sets {
		h := d.header
		for _, code := range []string{h.Abbreviation, h.TCGOnline} {
			if code == "" {
				continue
			}
			// Sets sharing a code (30th and 30th-c, cel25 and cel25cc) keep
			// the main set, whose id is shorter, so the pick does not
			// depend on load order.
			k := strings.ToUpper(code)
			if old, ok := setIndex[k]; !ok || len(d.id) < len(old) || (len(d.id) == len(old) && d.id < old) {
				setIndex[k] = d.id
			}
		}
		if h.Serie == "tcgp" {
			continue // Pokémon TCG Pocket, a different game
		}
		for i, c := range d.cards {
			num, err := strconv.Atoi(c.LocalID)
			if err != nil {
				num = 1 << 30 // lettered prints (TG05) after numbered ones
			}
			k := cards.NameKey(c.Name)
			all[k] = append(all[k], pick{c.ID, h.ReleaseDate, d.art[i], num})
		}
	}
	byName := make(map[string][]string, len(all))
	for k, ps := range all {
		slices.SortFunc(ps, func(a, b pick) int {
			switch {
			case better(a, b):
				return -1
			case better(b, a):
				return 1
			}
			return strings.Compare(a.id, b.id)
		})
		ids := make([]string, len(ps))
		for i, p := range ps {
			ids[i] = p.id
		}
		byName[k] = ids
	}
	return setIndex, byName
}

// codeAliases maps TCG Live set codes that differ from the sets' own
// abbreviations.
var codeAliases = map[string]string{
	"PR-SV": "SVP",
	"PR-SW": "SWSHP",
}

// setAliases maps set codes straight to a set id, for sets whose official
// abbreviation is shared (30th Celebration and its Classic Collection are
// both "30C") or that exports spell their own way.
var setAliases = map[string]string{
	"30TH": "30th",
	"M30":  "30th",
}

// normCode normalises a set code as decklists print it: upper case, with
// TCG Live's own codes mapped to the sets' abbreviations (codeAliases).
// The result is looked up in setAliases, then the code index.
func normCode(setCode string) string {
	code := strings.ToUpper(strings.TrimSpace(setCode))
	if a, ok := codeAliases[code]; ok {
		code = a
	}
	return code
}

// Version is the sha256 of the manifest the snapshot came from; "" for a
// snapshot built from no manifest.
func (s *Snapshot) Version() string {
	if s == nil {
		return ""
	}
	return s.version
}

// Sets lists the snapshot's sets, by id.
func (s *Snapshot) Sets() []SetInfo {
	if s == nil {
		return nil
	}
	out := make([]SetInfo, 0, len(s.sets))
	for _, id := range slices.Sorted(maps.Keys(s.sets)) {
		d := s.sets[id]
		out = append(out, SetInfo{ID: id, Name: d.header.Text[cardset.English].Name, Hash: d.hash, Prints: len(d.cards)})
	}
	return out
}

// Prints is the number of cards in the snapshot.
func (s *Snapshot) Prints() int {
	if s == nil {
		return 0
	}
	return len(s.cards)
}

// Card implements cards.DB.
func (s *Snapshot) Card(_ context.Context, id string) (*cards.Card, error) {
	if c := s.Cached(id); c != nil {
		return c, nil
	}
	return nil, fmt.Errorf("card %s: %w", id, cards.ErrNotFound)
}

// Cached implements cards.DB: every card in a snapshot is in memory.
func (s *Snapshot) Cached(id string) *cards.Card {
	if s == nil {
		return nil
	}
	return s.cards[id]
}

// Resolve implements cards.DB: a set code as decklists and TCG Live print
// it, through the aliases and the code index, and a collector number,
// exactly or without leading zeros.
func (s *Snapshot) Resolve(_ context.Context, setCode, number string) (string, error) {
	code := normCode(setCode)
	setID, ok := setAliases[code]
	if !ok && s != nil {
		setID, ok = s.setIndex[code]
	}
	if !ok {
		return "", fmt.Errorf("unknown set code %q: %w", setCode, cards.ErrNotFound)
	}
	if s != nil {
		if d, ok := s.sets[setID]; ok {
			if id, ok := findNumber(d, number); ok {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("no card %s %s in set %s: %w", setCode, number, setID, cards.ErrNotFound)
}

// findNumber finds a collector number in a set: exactly, or as the same
// number without leading zeros.
func findNumber(d *setData, number string) (string, bool) {
	want, numErr := strconv.Atoi(number)
	for _, c := range d.cards {
		if strings.EqualFold(c.LocalID, number) {
			return c.ID, true
		}
		if numErr == nil {
			if n, err := strconv.Atoi(c.LocalID); err == nil && n == want {
				return c.ID, true
			}
		}
	}
	return "", false
}

// ByName implements cards.DB. A name has many prints; it picks the newest
// one with art (what Standard play uses), preferring the regular print
// over higher-numbered alternate arts. Pokémon TCG Pocket sets are
// skipped. "Basic Fire Energy", as logs name it, also matches "Fire
// Energy". With moves, it picks the preferred print that has the most of
// them: Chi-Yu seen using Ground Melter is the Twilight Masquerade Fire
// Chi-Yu, not a newer Darkness one.
func (s *Snapshot) ByName(_ context.Context, name string, moves ...string) (string, error) {
	ids, err := s.prints(name)
	if err != nil {
		return "", err
	}
	if len(moves) == 0 {
		return ids[0], nil
	}
	best, most := ids[0], 0
	for _, id := range ids {
		c := s.cards[id]
		if c == nil {
			continue
		}
		if n := hasMoves(c, moves); n > most {
			best, most = id, n
			if n == len(moves) {
				break
			}
		}
	}
	return best, nil
}

// prints lists the prints of name, preferred first.
func (s *Snapshot) prints(name string) ([]string, error) {
	if s != nil {
		key := cards.NameKey(name)
		if ids, ok := s.byName[key]; ok {
			return ids, nil
		}
		if k, ok := strings.CutPrefix(key, "basic "); ok {
			if ids, ok := s.byName[k]; ok {
				return ids, nil
			}
		}
	}
	return nil, fmt.Errorf("no card named %q: %w", name, cards.ErrNotFound)
}

// hasMoves counts the moves the card has as an attack or Ability.
func hasMoves(c *cards.Card, moves []string) int {
	n := 0
	for _, m := range moves {
		if slices.ContainsFunc(c.Attacks, func(a cards.Attack) bool { return a.Name == m }) ||
			slices.ContainsFunc(c.Abilities, func(a cards.Ability) bool { return a.Name == m }) {
			n++
		}
	}
	return n
}

// SetID names a set for the admin page: by its id in any case (me01,
// ME01), or by a set code as decklists print it (MEG), through the same
// aliases and code index Resolve uses. It returns the set's id and display
// name, or an error wrapping cards.ErrUnknownSet for a name no set in the
// snapshot has.
func (s *Snapshot) SetID(name string) (id, setName string, err error) {
	name = strings.TrimSpace(name)
	if name == "" || s == nil {
		return "", "", fmt.Errorf("%q: %w", name, cards.ErrUnknownSet)
	}
	find := func() (string, bool) {
		for id := range s.sets {
			if strings.EqualFold(id, name) {
				return id, true
			}
		}
		code := normCode(name)
		if id, ok := setAliases[code]; ok {
			return id, true
		}
		id, ok := s.setIndex[code]
		return id, ok
	}
	id, ok := find()
	if !ok {
		return "", "", fmt.Errorf("%q: %w", name, cards.ErrUnknownSet)
	}
	setName = id
	if d, ok := s.sets[id]; ok && d.header.Text[cardset.English].Name != "" {
		setName = d.header.Text[cardset.English].Name
	}
	return id, setName, nil
}
