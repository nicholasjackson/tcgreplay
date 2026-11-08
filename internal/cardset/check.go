package cardset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Kinds only the check raises.
const (
	KindInvalid            = "invalid"             // the built set does not read or is not well formed
	KindMarkup             = "markup"              // HTML in card text
	KindCorrectionsInvalid = "corrections-invalid" // the corrections file does not read
	KindCorrectionDangling = "correction-dangling" // a correction's card or path no longer resolves
)

// CheckItem is an open item the check found in one built set.
type CheckItem struct {
	Set string
	ReportItem
}

// String is the line sets check prints.
func (c CheckItem) String() string {
	s := c.Set + " " + c.Card + " " + c.Kind
	if c.Field != "" {
		s += " " + c.Field
	}
	if c.Reason != "" {
		s += ": " + c.Reason
	}
	return s
}

// gapKinds are the report kinds the check recomputes from the built set.
var gapKinds = []string{KindMissingName, KindMissingEffect, KindNoAttacks, KindMissingEvolvesFrom, KindNoImage}

// Check runs the offline checks over built sets in dir (every
// <dir>/sets/*.json when no set ids are given) and returns the open items:
// a set that does not read or is not well formed, HTML in any text, a
// corrections file that does not read or a correction whose card or path
// no longer resolves, a structural gap (missing effect, no attacks,
// missing evolves-from, no image) without an acknowledgement, and any item
// of the set's last import report still open under the current
// corrections file. It never touches the network.
func Check(dir string, sets ...string) ([]CheckItem, error) {
	if len(sets) == 0 {
		paths, err := filepath.Glob(filepath.Join(dir, "sets", "*.json"))
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			sets = append(sets, strings.TrimSuffix(filepath.Base(p), ".json"))
		}
	}
	slices.Sort(sets)
	var out []CheckItem
	for _, id := range sets {
		items, err := checkSet(dir, id)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			out = append(out, CheckItem{Set: id, ReportItem: it})
		}
	}
	return out, nil
}

// checkSet checks one built set; it errors only when the set file is
// missing.
func checkSet(dir, id string) ([]ReportItem, error) {
	path := filepath.Join(dir, "sets", id+".json")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("set %s: %w", id, err)
	}
	s, err := Read(path)
	if err != nil {
		return []ReportItem{{Card: id, Kind: KindInvalid, Status: StatusOpen, Reason: err.Error()}}, nil
	}
	var items []ReportItem
	open := func(it ReportItem) {
		it.Status = StatusOpen
		items = append(items, it)
	}
	for _, p := range s.Prints {
		for _, f := range markupFields(&p) {
			open(ReportItem{Card: p.ID, Name: p.Text[English].Name, Kind: KindMarkup, Field: f})
		}
	}
	for lang, t := range s.Set.Text {
		if hasMarkup(t.Name) {
			open(ReportItem{Card: id, Kind: KindMarkup, Field: "/set/text/" + lang + "/name"})
		}
	}

	corrs, err := ReadCorrections(filepath.Join(dir, "corrections", id+".json"))
	if err != nil {
		open(ReportItem{Card: id, Kind: KindCorrectionsInvalid, Reason: err.Error()})
		corrs = &Corrections{}
	}
	var applied []applied
	for _, c := range corrs.Corrections {
		cp, err := cloneSet(s)
		if err != nil {
			return nil, err
		}
		a, err := applyCorrections(cp, []Correction{c})
		if err != nil {
			open(ReportItem{Card: c.Card, Kind: KindCorrectionDangling, Field: c.Path, Reason: err.Error()})
			continue
		}
		applied = append(applied, a...)
	}

	for _, p := range s.Prints {
		for _, it := range gapItems(&p) {
			setStatus(&it, nil, corrs.Acknowledged)
			if it.Status == StatusOpen {
				items = append(items, it)
			}
		}
	}

	// The last import's items that need the sources to see (disagreements,
	// changed text, redundant corrections, gone prints), judged against
	// the corrections file as it is now.
	rep, err := readReport(filepath.Join(dir, "reports", id+".json"))
	if err != nil {
		open(ReportItem{Card: id, Kind: KindInvalid, Field: "/report", Reason: err.Error()})
	}
	for _, it := range rep.Items {
		if slices.Contains(gapKinds, it.Kind) {
			continue
		}
		if it.Kind == KindCorrectionRedundant && !slices.ContainsFunc(corrs.Corrections, func(c Correction) bool { return c.Card == it.Card && c.Path == it.Field }) {
			continue // the redundant correction has been removed
		}
		it.Reason = ""
		setStatus(&it, applied, corrs.Acknowledged)
		if it.Status == StatusOpen {
			items = append(items, it)
		}
	}
	sortItems(s, items)
	return items, nil
}

// readReport reads a set's last import report; a missing report is empty.
func readReport(path string) (Report, error) {
	var r Report
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return Report{}, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// markupFields lists the JSON Pointer of every text of the print, in any
// language, that holds an HTML tag.
func markupFields(p *Print) []string {
	var out []string
	for _, lang := range sortedKeys(p.Text) {
		t := p.Text[lang]
		base := "/text/" + lang
		check := func(field, s string) {
			if hasMarkup(s) {
				out = append(out, base+field)
			}
		}
		check("/name", t.Name)
		check("/evolveFrom", t.EvolveFrom)
		check("/effect", t.Effect)
		for i, a := range t.Attacks {
			check("/attacks/"+strconv.Itoa(i)+"/name", a.Name)
			check("/attacks/"+strconv.Itoa(i)+"/effect", a.Effect)
		}
		for i, a := range t.Abilities {
			check("/abilities/"+strconv.Itoa(i)+"/name", a.Name)
			check("/abilities/"+strconv.Itoa(i)+"/effect", a.Effect)
		}
	}
	return out
}

func cloneSet(s *Set) (*Set, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var c Set
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
