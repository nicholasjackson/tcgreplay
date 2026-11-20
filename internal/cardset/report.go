package cardset

import (
	"cmp"
	"slices"
	"strings"
)

// Report kinds: what an import says still needs attention.
const (
	KindDisagreement        = "disagreement"         // the sources give a field different values
	KindMissingEffect       = "missing-effect"       // a Trainer, special Energy or ability without effect text
	KindMissingName         = "missing-name"         // an attack or ability without an English name
	KindNoAttacks           = "no-attacks"           // a Pokémon without attacks
	KindMissingEvolvesFrom  = "missing-evolves-from" // a Stage 1 or Stage 2 without evolves-from
	KindNoImage             = "no-image"             // a print no source has an image for
	KindTextChanged         = "text-changed"         // English text changed since the last import
	KindCorrectionRedundant = "correction-redundant" // the sources now give a correction's value
	KindPrintGone           = "print-gone-upstream"  // a print upstream no longer lists, kept
)

// Report statuses.
const (
	StatusOpen         = "open"         // needs a correction, an acknowledgement or a look
	StatusCorrected    = "corrected"    // a correction covers the field
	StatusAcknowledged = "acknowledged" // a reason is recorded in the corrections file
)

// Report is an import's machine-readable list of what still needs
// attention, written beside the built set. It holds no timestamps, so an
// import against unchanged sources rewrites it byte for byte.
type Report struct {
	Set     string       `json:"set"`
	Summary Summary      `json:"summary"`
	Items   []ReportItem `json:"items"`
}

// ReportItem is one thing needing attention.
type ReportItem struct {
	Card   string         `json:"card"`
	Name   string         `json:"name,omitempty"`
	Kind   string         `json:"kind"`
	Field  string         `json:"field,omitempty"` // JSON Pointer into the print
	Values map[string]any `json:"values,omitempty"`
	Status string         `json:"status"`
	Reason string         `json:"reason,omitempty"`
}

// gapItems lists a print's structural gaps: move names, effect text, attacks,
// evolves-from and image. Evolves-from is checked for Stage 1 and Stage 2
// only, as the spec states: other evolving stages (VMAX, BREAK, LEVEL-UP)
// follow other evolve rules in the sources.
func gapItems(p *Print) []ReportItem {
	var out []ReportItem
	add := func(kind, field string) {
		out = append(out, ReportItem{Card: p.ID, Name: p.Text[English].Name, Kind: kind, Field: field})
	}
	for _, f := range p.MissingEnglish() {
		if strings.HasSuffix(f, "/name") {
			add(KindMissingName, f)
		} else {
			add(KindMissingEffect, f)
		}
	}
	if p.Category == "Pokemon" && len(p.Attacks) == 0 {
		add(KindNoAttacks, "/attacks")
	}
	if (p.Stage == "Stage1" || p.Stage == "Stage2") && strings.TrimSpace(p.Text[English].EvolveFrom) == "" {
		add(KindMissingEvolvesFrom, "/text/en/evolveFrom")
	}
	if len(p.Images) == 0 {
		add(KindNoImage, "/images")
	}
	return out
}

// reportInput is what one import found.
type reportInput struct {
	set           *Set                      // the set as written
	disagreements map[string][]disagreement // by print id
	applied       []applied
	cmp           comparison
	acks          []Ack
}

// buildReport lists every item of an import with its status, sorted by
// collector number, kind and field.
func buildReport(in reportInput) Report {
	var items []ReportItem
	name := func(id string) string {
		if p, ok := in.set.Print(id); ok {
			return p.Text[English].Name
		}
		return ""
	}
	for _, p := range in.set.Prints {
		for _, d := range in.disagreements[p.ID] {
			items = append(items, ReportItem{Card: p.ID, Name: p.Text[English].Name, Kind: KindDisagreement, Field: d.Field, Values: d.Values})
		}
		items = append(items, gapItems(&p)...)
	}
	for _, id := range in.cmp.changed {
		items = append(items, ReportItem{Card: id, Name: name(id), Kind: KindTextChanged, Field: "/text/en"})
	}
	for _, id := range in.cmp.gone {
		items = append(items, ReportItem{Card: id, Name: name(id), Kind: KindPrintGone})
	}
	for _, a := range in.applied {
		if a.Redundant {
			items = append(items, ReportItem{Card: a.Card, Name: name(a.Card), Kind: KindCorrectionRedundant, Field: a.Path})
		}
	}
	for i := range items {
		setStatus(&items[i], in.applied, in.acks)
	}
	sortItems(in.set, items)
	r := Report{Set: in.set.Set.ID, Items: items}
	if r.Items == nil {
		r.Items = []ReportItem{}
	}
	return r
}

// setStatus marks an item corrected when an applied correction covers its
// field (a correction's path equal to or above it), acknowledged when the
// corrections file records a reason for it, else open. A redundant
// correction is never covered by itself.
func setStatus(it *ReportItem, applied []applied, acks []Ack) {
	it.Status = StatusOpen
	if it.Kind != KindCorrectionRedundant && it.Field != "" {
		for _, a := range applied {
			if a.Card == it.Card && covers(a.Path, it.Field) {
				it.Status = StatusCorrected
				return
			}
		}
	}
	for _, a := range acks {
		if a.Card == it.Card && a.Kind == it.Kind && (a.Field == "" || a.Field == it.Field) {
			it.Status, it.Reason = StatusAcknowledged, a.Reason
			return
		}
	}
}

// covers reports whether a correction at path settles field: the same
// field or one inside it.
func covers(path, field string) bool {
	return path == field || strings.HasPrefix(field, path+"/")
}

// sortItems orders items by their print's collector number, then kind and
// field.
func sortItems(s *Set, items []ReportItem) {
	ps := slices.Clone(s.Prints)
	SortPrints(ps)
	order := make(map[string]int, len(ps))
	for i, p := range ps {
		order[p.ID] = i
	}
	slices.SortStableFunc(items, func(a, b ReportItem) int {
		oa, okA := order[a.Card]
		ob, okB := order[b.Card]
		switch {
		case okA && okB && oa != ob:
			return cmp.Compare(oa, ob)
		case okA != okB:
			if okA {
				return -1
			}
			return 1
		case !okA && a.Card != b.Card:
			return strings.Compare(a.Card, b.Card)
		}
		if c := strings.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		return strings.Compare(a.Field, b.Field)
	})
}

// open counts the open items.
func (r Report) open() int {
	n := 0
	for _, it := range r.Items {
		if it.Status == StatusOpen {
			n++
		}
	}
	return n
}

// WriteReport writes a report deterministically.
func WriteReport(path string, r Report) error {
	b, err := marshal(r)
	if err != nil {
		return err
	}
	return writeFile(path, b)
}
