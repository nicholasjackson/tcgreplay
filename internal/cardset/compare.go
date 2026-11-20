package cardset

import (
	"encoding/json"
	"maps"
)

// comparison is what a fresh build changed against the set already in the
// project.
type comparison struct {
	set     *Set     // the set to write
	added   []string // prints new since the previous build
	changed []string // prints whose English text changed
	gone    []string // prints upstream no longer lists, kept unchanged
}

// compare carries a fresh build over the previous built set (nil if there
// is none). Every print of the fresh build is kept with its new core,
// images and English text (corrections are already applied, so corrected
// fields do not churn); wording in other languages already in the file is
// carried over untouched, since the import writes English only; a print
// upstream no longer lists is kept unchanged and listed as gone.
func compare(prev, next *Set) comparison {
	out := comparison{set: next}
	if prev == nil {
		for _, p := range next.Prints {
			out.added = append(out.added, p.ID)
		}
		SortPrints(next.Prints)
		return out
	}
	old := make(map[string]Print, len(prev.Prints))
	for _, p := range prev.Prints {
		old[p.ID] = p
	}
	fresh := make(map[string]bool, len(next.Prints))
	for i := range next.Prints {
		p := &next.Prints[i]
		fresh[p.ID] = true
		was, ok := old[p.ID]
		if !ok {
			out.added = append(out.added, p.ID)
			continue
		}
		if !sameJSON(was.Text[English], p.Text[English]) {
			out.changed = append(out.changed, p.ID)
		}
		p.Text = carryLanguages(was.Text, p.Text)
	}
	for _, p := range prev.Prints {
		if !fresh[p.ID] {
			next.Prints = append(next.Prints, p)
			out.gone = append(out.gone, p.ID)
		}
	}
	next.Set.Text = carryLanguages(prev.Set.Text, next.Set.Text)
	SortPrints(next.Prints)
	return out
}

// carryLanguages returns next's text with every language other than
// English taken from prev.
func carryLanguages[T any](prev, next map[string]T) map[string]T {
	out := maps.Clone(next)
	if out == nil {
		out = map[string]T{}
	}
	for lang, t := range prev {
		if lang != English {
			out[lang] = t
		}
	}
	return out
}

func sameJSON(a, b any) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(ab) == string(bb)
}
