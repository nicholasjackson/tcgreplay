package importer

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// Hidden-card repair.
//
// Unnamed draws and prize deals pick cards arbitrarily, because the log
// does not say which card moved. A later line can then contradict that
// pick, e.g. a hand made of cards known to be other things must produce a
// Supporter. Since neither the picked card nor some other card in the deck
// was visible to anyone in the meantime, swapping the two cards' iids in
// every event since the blind move gives an equally valid history in which
// the right card was picked.

// iidsOf lists the cards an event exposes. Shuffles and blind moves expose
// nothing.
func iidsOf(e *state.Event, blind bool) []state.IID {
	if blind {
		return nil
	}
	p, err := e.Decode()
	if err != nil {
		return nil
	}
	switch p := p.(type) {
	case *state.MovePayload:
		return []state.IID{p.IID}
	case *state.RevealPayload:
		return p.IIDs
	}
	return nil
}

// track updates visibility bookkeeping after an event is emitted.
func (r *replay) track(e *state.Event) {
	if r.lastPublic == nil {
		r.lastPublic, r.firstBlind = map[state.IID]int{}, map[state.IID]int{}
	}
	if r.blind {
		if p, err := e.Decode(); err == nil {
			if mp, ok := p.(*state.MovePayload); ok && r.firstBlind[mp.IID] <= r.lastPublic[mp.IID] {
				r.firstBlind[mp.IID] = e.ID
			}
		}
		return
	}
	for _, iid := range iidsOf(e, false) {
		r.lastPublic[iid] = e.ID
	}
}

// blindSince returns the event since which iid has only moved blindly, or 0.
func (r *replay) blindSince(iid state.IID) int {
	if fb := r.firstBlind[iid]; fb > r.lastPublic[iid] {
		return fb
	}
	return 0
}

type swapRec struct {
	a, b state.IID
	from int
}

// repairAmong swaps one of hs (blindly placed cards that are known to be
// something else) with a hidden card that can be name, returning the card
// now standing where the swapped one was.
func (r *replay) repairAmong(seat state.Seat, hs []state.IID, name string) (state.IID, bool) {
	if name == "" {
		return "", false
	}
	for _, h := range slices.Clone(hs) {
		if r.bound[h] == name || r.bound[h] == "" {
			continue
		}
		since := r.blindSince(h)
		if since == 0 {
			continue
		}
		for _, d := range r.hiddenCards(seat) {
			if d == h || slices.Contains(hs, d) || !r.bindable(d, name) || r.lastPublic[d] >= since {
				continue
			}
			if err := r.swap(h, d, since); err != nil {
				continue
			}
			return d, true
		}
	}
	return "", false
}

// hiddenCards lists seat's cards in the deck and prize slots.
func (r *replay) hiddenCards(seat state.Seat) []state.IID {
	side := r.st.Sides[seat]
	out := slices.Clone(side.Deck)
	for _, p := range side.Prizes {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// swap exchanges two iids in every event from id from onward and rebuilds
// the state.
func (r *replay) swap(a, b state.IID, from int) error {
	r.rewriteSwap(a, b, from)
	var all []state.Event
	for _, s := range r.steps {
		all = append(all, s.Events...)
	}
	st, err := state.Fold(all)
	if err != nil {
		r.rewriteSwap(a, b, from) // swapping twice restores the events
		return fmt.Errorf("swap %s/%s: %w", a, b, err)
	}
	for iid, name := range r.bound {
		st.Cards[iid].Name = name
	}
	r.st = st
	r.lastPublic[a], r.lastPublic[b] = r.lastPublic[b], r.lastPublic[a]
	r.firstBlind[a], r.firstBlind[b] = r.firstBlind[b], r.firstBlind[a]
	for seat, ps := range r.pendingPrize {
		for i, p := range ps {
			switch p {
			case a:
				r.pendingPrize[seat][i] = b
			case b:
				r.pendingPrize[seat][i] = a
			}
		}
	}
	r.swaps = append(r.swaps, swapRec{a, b, from})
	return nil
}

// rewriteSwap exchanges two iids in the payloads of events from id from on.
func (r *replay) rewriteSwap(a, b state.IID, from int) {
	qa, qb := []byte(`"`+string(a)+`"`), []byte(`"`+string(b)+`"`)
	tmp := []byte("\"\x00swap\x00\"")
	for i := range r.steps {
		for j := range r.steps[i].Events {
			e := &r.steps[i].Events[j]
			if e.ID < from {
				continue
			}
			p := bytes.ReplaceAll(e.Payload, qa, tmp)
			p = bytes.ReplaceAll(p, qb, qa)
			e.Payload = bytes.ReplaceAll(p, tmp, qb)
		}
	}
}
