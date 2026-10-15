// Package deck parses decklists in the Pokémon TCG Live export format:
//
//	Pokémon: 12
//	4 Riolu SVI 113
//	...
//	Trainer: 36
//	4 Ultra Ball SVI 196
//	...
//	Energy: 12
//	8 Basic {F} Energy SVE 6
//
//	Total Cards: 60
package deck

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// Entry is one decklist line.
type Entry struct {
	Count   int    `json:"count"`
	Name    string `json:"name"`
	Set     string `json:"set"`
	Number  string `json:"number"`
	Section string `json:"section,omitempty"` // Pokémon, Trainer or Energy
	// Def is the resolved TCGdex id, empty if unresolved.
	Def string `json:"def,omitempty"`
}

// List is a parsed decklist.
type List struct {
	Entries []Entry `json:"entries"`
}

// Total counts the cards in the list.
func (l *List) Total() int {
	n := 0
	for _, e := range l.Entries {
		n += e.Count
	}
	return n
}

var (
	sectionRe = regexp.MustCompile(`^(Pok[eé]mon|Trainer|Energy)\s*:\s*(\d+)\s*$`)
	totalRe   = regexp.MustCompile(`(?i)^Total Cards\s*:\s*(\d+)\s*$`)
	lineRe    = regexp.MustCompile(`^(\d+)\s+(.+?)\s+([A-Za-z0-9-]+)\s+([A-Za-z0-9-]+)$`)
	// Some exports omit set and number, e.g. "8 Basic Fighting Energy".
	bareRe = regexp.MustCompile(`^(\d+)\s+(.+)$`)
)

// Parse reads an export. Unknown lines are errors so nothing is silently
// dropped.
func Parse(text string) (*List, error) {
	l := &List{}
	section := ""
	declared := -1
	var errs []error
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		if line == "" {
			continue
		}
		if m := sectionRe.FindStringSubmatch(line); m != nil {
			section = m[1]
			if strings.HasPrefix(section, "Pok") {
				section = "Pokémon"
			}
			continue
		}
		if m := totalRe.FindStringSubmatch(line); m != nil {
			declared, _ = strconv.Atoi(m[1])
			continue
		}
		if m := lineRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			l.Entries = append(l.Entries, Entry{Count: n, Name: m[2], Set: m[3], Number: m[4], Section: section})
			continue
		}
		if m := bareRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			l.Entries = append(l.Entries, Entry{Count: n, Name: m[2], Section: section})
			continue
		}
		errs = append(errs, fmt.Errorf("line %d: cannot parse %q", i+1, line))
	}
	if len(l.Entries) == 0 {
		errs = append(errs, errors.New("no cards found"))
	}
	if declared >= 0 && declared != l.Total() {
		errs = append(errs, fmt.Errorf("list says %d cards but has %d", declared, l.Total()))
	}
	return l, errors.Join(errs...)
}

// basicEnergy maps energy symbols and names to the SVE set's basic Energy.
var basicEnergy = map[string]string{
	"G": "sve-001", "GRASS": "sve-001",
	"R": "sve-002", "FIRE": "sve-002",
	"W": "sve-003", "WATER": "sve-003",
	"L": "sve-004", "LIGHTNING": "sve-004",
	"P": "sve-005", "PSYCHIC": "sve-005",
	"F": "sve-006", "FIGHTING": "sve-006",
	"D": "sve-007", "DARKNESS": "sve-007",
	"M": "sve-008", "METAL": "sve-008",
}

var basicEnergyName = regexp.MustCompile(`(?i)^Basic\s+\{?([A-Z]+)\}?\s+Energy$`)

// Resolve fills Def for each entry using db. Unresolved entries are listed
// in the returned error but keep their names, so the deck still loads.
func (l *List) Resolve(ctx context.Context, db cards.DB) error {
	var errs []error
	for i := range l.Entries {
		e := &l.Entries[i]
		if e.Def != "" {
			continue
		}
		var err error
		if e.Set != "" {
			var id string
			if id, err = db.Resolve(ctx, e.Set, e.Number); err == nil {
				e.Def = id
				continue
			}
		} else {
			err = errors.New("no set code")
		}
		// Basic Energy prints vary; fall back to the SVE set.
		if m := basicEnergyName.FindStringSubmatch(e.Name); m != nil {
			if id, ok := basicEnergy[strings.ToUpper(m[1])]; ok {
				e.Def = id
				continue
			}
		}
		errs = append(errs, fmt.Errorf("%s %s %s: %w", e.Name, e.Set, e.Number, err))
	}
	return errors.Join(errs...)
}
