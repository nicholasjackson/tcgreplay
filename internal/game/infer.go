package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/infer"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// ProfileReport is what is known and assumed about a seat's list at the
// current head.
type ProfileReport struct {
	Seat state.Seat `json:"seat"`
	// Profile is the latest assumed list in the game, if any. Its Observed
	// counts cover the whole imported log.
	Profile *infer.Profile `json:"profile,omitempty"`
	// SeenSoFar is the most copies of each card the other seat had seen
	// at once by the head: what a list must hold at this point.
	SeenSoFar map[string]int `json:"seen_so_far"`
}

// latestProfile returns the most recent assumed list stored anywhere in
// the game, on any branch: a list is a belief about the opponent's deck,
// not about a position, so a list given on one branch applies to all. The
// caller holds g.mu.
func (g *Game) latestProfile() (*infer.Profile, error) {
	var raw json.RawMessage
	for id := 1; id < g.log.NextID(); id++ {
		e := g.log.Event(id)
		if e.Type != state.EvSetup && e.Type != state.EvAssume {
			continue
		}
		p, err := e.Decode()
		if err != nil {
			return nil, err
		}
		switch p := p.(type) {
		case *state.SetupPayload:
			if len(p.Profile) > 0 {
				raw = p.Profile
			}
		case *state.AssumePayload:
			if len(p.Profile) > 0 {
				raw = p.Profile
			}
		}
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var prof infer.Profile
	if err := json.Unmarshal(raw, &prof); err != nil {
		return nil, fmt.Errorf("stored profile: %w", err)
	}
	return &prof, nil
}

// evidence gathers what the other seat has seen of owner's cards up to the
// head. The caller holds g.mu.
func (g *Game) evidence(owner state.Seat) (*infer.Evidence, error) {
	path, err := g.log.PathTo(g.log.Head())
	if err != nil {
		return nil, err
	}
	_, ev, err := infer.Gather(path, owner, owner.Other())
	return ev, err
}

// Profile reports the assumed list and the evidence for owner's cards.
func (g *Game) Profile(owner state.Seat) (*ProfileReport, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ev, err := g.evidence(owner)
	if err != nil {
		return nil, err
	}
	prof, err := g.latestProfile()
	if err != nil {
		return nil, err
	}
	return &ProfileReport{Seat: owner, Profile: prof, SeenSoFar: ev.Min}, nil
}

// SampleHidden gives every card of owner's that the other seat cannot see
// at the head a plausible identity, as an assume event. Cards are drawn
// from list if given (reconciled with what the log proves, and stored as
// the game's profile), else from the latest stored profile, else the
// identities already assumed are redistributed. Seed 0 uses the game's
// randomness. Review mode only: in play it would rewrite a live deck.
func (g *Game) SampleHidden(owner state.Seat, actor state.Actor, list []deck.Entry, seed uint64) (*Result, error) {
	if !owner.Valid() {
		return nil, fmt.Errorf("unknown seat %q", owner)
	}
	if g.Mode() != ModeReview {
		return nil, errors.New("sampling hidden cards is for review mode; call set_mode review")
	}
	label := fmt.Sprintf("Sample %s's hidden cards", owner)
	return g.run(owner, actor, actionSpec{label: label, kind: "assume", anyTurn: true}, func(t *tx) error {
		ev, err := t.g.evidence(owner)
		if err != nil {
			return err
		}
		prof, err := t.g.latestProfile()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if len(list) > 0 {
			observed := ev.Min
			if prof != nil && len(prof.Observed) > 0 {
				observed = prof.Observed
			}
			p, warnings := infer.Reconcile(list, observed)
			if prof != nil {
				p.Opponent, p.Archetype = prof.Opponent, prof.Archetype
			}
			prof = &p
			t.warn = append(t.warn, warnings...)
			if raw, err = json.Marshal(prof); err != nil {
				return err
			}
		}
		rng := t.g.rng
		if seed != 0 {
			rng = rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		}
		payload, warnings := infer.Sample(t.st, ev, prof, owner, owner.Other(), rng)
		t.warn = append(t.warn, warnings...)
		if prof == nil || len(prof.Cards) == 0 {
			t.warn = append(t.warn, "no assumed list: redistributed the identities already known; pass a list to fill unknown cards")
		}
		payload.Profile = raw
		if len(payload.Cards) == 0 && len(raw) == 0 {
			return errors.New("no hidden cards to sample")
		}
		t.data = summarizeSample(t.st, payload, owner)
		return t.emit(owner, state.EvAssume, payload)
	})
}

// summarizeSample lists the sampled hand and prizes by name.
func summarizeSample(st *state.State, p state.AssumePayload, owner state.Seat) map[string][]string {
	out := map[string][]string{"hand": {}, "prizes": {}}
	side := st.Sides[owner]
	for _, iid := range side.Hand {
		if a, ok := p.Cards[iid]; ok {
			out["hand"] = append(out["hand"], fmt.Sprintf("%s %s", iid, nameOr(a.Name)))
		}
	}
	for _, iid := range side.Prizes {
		if a, ok := p.Cards[iid]; ok {
			out["prizes"] = append(out["prizes"], fmt.Sprintf("%s %s", iid, nameOr(a.Name)))
		}
	}
	return out
}

func nameOr(n string) string {
	if n == "" {
		return "?"
	}
	return n
}
