package importer

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// Options configure an import.
type Options struct {
	// Me is the log owner's name; detected from the itemised opening hand
	// if empty.
	Me string
	// MyDeck is the log owner's decklist (with Defs resolved if possible).
	MyDeck []deck.Entry
	// OppDeck optionally assumes the opponent's list. Cards the log never
	// reveals are filled from it with basis "reference".
	OppDeck []deck.Entry
	// Seed makes the reconstruction reproducible; 0 picks one at random.
	Seed uint64
	// Trace, if set, receives each line and a board summary after it.
	Trace io.Writer
}

// Result is a replayed game ready to be written as an event log.
type Result struct {
	Steps  []Step
	Report Report
}

// Events returns every event in order.
func (res *Result) Events() []state.Event {
	var out []state.Event
	for _, s := range res.Steps {
		out = append(out, s.Events...)
	}
	return out
}

// Import replays a TCG Live battle log. Where the log cannot tell
// identical cards apart, the replay records a choice point; if the first
// reconstruction hits problems, Import searches alternative choices and
// keeps the reconstruction with the fewest problems.
func Import(text string, opts Options) (*Result, error) {
	secs, err := Parse(text)
	if err != nil {
		return nil, err
	}
	me, opp, err := Players(secs, opts.Me)
	if err != nil {
		return nil, err
	}
	if len(opts.MyDeck) == 0 {
		return nil, fmt.Errorf("the log owner's decklist is required")
	}
	seed := opts.Seed
	if seed == 0 {
		seed = rand.Uint64() | 1
	}
	trace := opts.Trace
	opts.Trace = nil
	type attempt struct {
		res       *Result
		choices   []choice
		decisions map[string]int
	}
	run := func(d map[string]int) (*attempt, error) {
		res, r, err := replayOnce(secs, me, opp, opts, seed, d)
		if err != nil {
			return nil, err
		}
		return &attempt{res, r.choices, d}, nil
	}
	worse := func(a, b *attempt) bool { // is a worse than b?
		pa, pb := a.res.Report.Problems, b.res.Report.Problems
		if len(pa) != len(pb) {
			return len(pa) > len(pb)
		}
		return len(pa) > 0 && pa[0].Line < pb[0].Line
	}
	best, err := run(map[string]int{})
	if err != nil {
		return nil, err
	}
	for runs := 0; runs < 200 && len(best.res.Report.Problems) > 0; {
		first := best.res.Report.Problems[0].Line
		improved := false
		for i := len(best.choices) - 1; i >= 0 && !improved && runs < 200; i-- {
			c := best.choices[i]
			if c.line > first {
				continue
			}
			for alt := 0; alt < c.n && !improved; alt++ {
				if alt == c.used {
					continue
				}
				d := maps.Clone(best.decisions)
				d[c.key] = alt
				runs++
				a, err := run(d)
				if err != nil {
					return nil, err
				}
				if worse(best, a) {
					best, improved = a, true
				}
			}
		}
		if !improved {
			break
		}
	}
	if trace != nil {
		opts.Trace = trace
		res, _, err := replayOnce(secs, me, opp, opts, seed, best.decisions)
		return res, err
	}
	return best.res, nil
}

// choice is a point where the replay picked between indistinguishable
// cards.
type choice struct {
	key  string
	line int
	n    int
	used int
}

func replayOnce(secs []*Section, me, opp string, opts Options, seed uint64, decisions map[string]int) (*Result, *replay, error) {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	r := &replay{
		st:             state.New(),
		seats:          map[string]state.Seat{me: MeSeat, opp: OppSeat},
		players:        map[state.Seat]string{MeSeat: me, OppSeat: opp},
		nextID:         1,
		bound:          map[state.IID]string{},
		limit:          map[string]int{},
		used:           map[string]int{},
		rng:            rng,
		trace:          opts.Trace,
		boundAt:        map[state.IID]int{},
		decisions:      decisions,
		pendingPrize:   map[state.Seat][]state.IID{},
		pendingRetreat: map[state.Seat]bool{},
		rep: &Report{
			Me: me, Opponent: opp,
			Turns:       map[string]int{me: 0, opp: 0},
			PrizesTaken: map[string]int{},
			Mulligans:   map[string]int{},
			Observed:    map[string]int{},
		},
	}
	total := 0
	for _, e := range opts.MyDeck {
		r.limit[e.Name] += e.Count
		total += e.Count
	}
	if total != state.DeckSize {
		r.rep.Warnings = append(r.rep.Warnings, fmt.Sprintf("decklist has %d cards", total))
	}
	r.nodes = buildHandlers(me, opp)
	r.mulligans = map[string][][]string{}
	for _, sec := range secs {
		for _, n := range sec.Lines {
			r.flat = append(r.flat, n)
			for _, c := range n.Children {
				if c.Depth == 1 {
					r.flat = append(r.flat, c)
					if mm := regexp.MustCompile(`^Cards revealed from Mulligan \d+$`).FindString(c.Text); mm != "" {
						player := strings.SplitN(n.Text, " took ", 2)[0]
						r.mulligans[player] = append(r.mulligans[player], c.Bullets())
					}
				}
			}
		}
	}

	// Setup with anonymous cards; identities are bound as the log reveals
	// them and the setup payload is rewritten at the end.
	var cards []state.Card
	for _, seat := range state.Seats {
		n := state.DeckSize
		if seat == MeSeat {
			n = total
		}
		for i := 1; i <= n; i++ {
			cards = append(cards, state.Card{IID: state.IID(fmt.Sprintf("%s-%d", seat, i)), Owner: seat})
		}
	}
	setup := state.SetupPayload{BenchSize: 5, PrizeCount: 6, Names: map[state.Seat]string{MeSeat: me, OppSeat: opp}, Cards: cards}
	r.begin(Step{Label: "Setup", Kind: "setup"})
	if err := r.emit("", state.EvSetup, setup); err != nil {
		return nil, nil, err
	}
	r.end()

	// Some logs omit the opponent's opening draw; it still happened.
	r.begin(Step{Label: "Opening hands", Kind: "setup"})
	for _, seat := range state.Seats {
		if !HasOpening(secs[0], r.players[seat]) {
			if err := r.draw(seat, 7, nil); err != nil {
				return nil, nil, err
			}
		}
	}
	r.end()

	prizesDealt := false
	for _, sec := range secs {
		switch sec.Kind {
		case "turn":
			seat, ok := r.seats[sec.Player]
			if !ok {
				r.problem(&Node{No: sec.No, Text: sec.Player + "'s Turn"}, fmt.Errorf("unknown player %q", sec.Player))
				continue
			}
			r.begin(Step{Line: sec.No, Label: sec.Player + "'s Turn", Kind: "turn", Seat: seat})
			err := r.endTurn()
			if err == nil && !prizesDealt {
				err = r.dealPrizes()
				prizesDealt = true
			}
			if err == nil {
				err = r.emit(seat, state.EvTurn, state.TurnPayload{Phase: "start", Number: r.st.Turn + 1})
			}
			r.end()
			if err != nil {
				return nil, nil, fmt.Errorf("line %d: %w", sec.No, err)
			}
			r.rep.Turns[sec.Player]++
			if r.rep.WentFirst == "" {
				r.rep.WentFirst = sec.Player
			}
			r.pendingPrize = map[state.Seat][]state.IID{}
		case "checkup":
			r.begin(Step{Line: sec.No, Label: "Pokémon Checkup", Kind: "checkup"})
			err := r.endTurn()
			r.end()
			if err != nil {
				return nil, nil, fmt.Errorf("line %d: %w", sec.No, err)
			}
		}
		for _, n := range sec.Lines {
			r.headline(n)
		}
	}

	for _, seat := range state.Seats {
		r.rep.PrizesTaken[r.players[seat]] = r.st.PrizeCount - r.st.Sides[seat].PrizesLeft()
		if r.pendingRetreat[seat] {
			r.rep.Warnings = append(r.rep.Warnings, fmt.Sprintf("%s retreated but the new Active Pokémon was never named", r.players[seat]))
		}
	}
	for _, n := range r.rep.Turns {
		r.rep.TotalTurns += n
	}
	for iid, name := range r.bound {
		if r.st.Cards[iid].Owner == OppSeat {
			r.rep.Observed[name]++
		}
	}

	if r.trace != nil {
		for iid, name := range r.bound {
			fmt.Fprintf(r.trace, "bound %s %s at line %d\n", iid, name, r.boundAt[iid])
		}
	}
	res := &Result{Steps: r.steps, Report: *r.rep}
	if err := r.finish(res, opts); err != nil {
		return nil, nil, err
	}
	return res, r, nil
}

func (r *replay) begin(s Step) {
	r.steps = append(r.steps, s)
	r.cur = &r.steps[len(r.steps)-1]
}

// end drops a step that produced no events.
func (r *replay) end() {
	if len(r.cur.Events) == 0 {
		r.steps = r.steps[:len(r.steps)-1]
	}
	r.cur = nil
}

func (r *replay) problem(n *Node, err error) {
	r.rep.Problems = append(r.rep.Problems, Problem{Line: n.No, Text: n.Text, Err: err.Error()})
}

// snapshot captures replay state so a failed line can be undone.
type snapshot struct {
	st      *state.State
	events  int
	nextID  int
	bound   map[state.IID]string
	used    map[string]int
	prize   map[state.Seat][]state.IID
	retreat map[state.Seat]bool
	// repair bookkeeping
	lastPublic, firstBlind map[state.IID]int
	swaps                  int
}

func (r *replay) save() snapshot {
	prize := map[state.Seat][]state.IID{}
	for k, v := range r.pendingPrize {
		prize[k] = slices.Clone(v)
	}
	return snapshot{r.st.Clone(), len(r.cur.Events), r.nextID, maps.Clone(r.bound), maps.Clone(r.used), prize, maps.Clone(r.pendingRetreat),
		maps.Clone(r.lastPublic), maps.Clone(r.firstBlind), len(r.swaps)}
}

func (r *replay) restore(s snapshot) {
	r.cur.Events = r.cur.Events[:s.events]
	for i := len(r.swaps) - 1; i >= s.swaps; i-- {
		w := r.swaps[i]
		r.rewriteSwap(w.a, w.b, w.from)
	}
	r.swaps = r.swaps[:s.swaps]
	r.st, r.nextID, r.bound, r.used, r.pendingPrize, r.pendingRetreat = s.st, s.nextID, s.bound, s.used, s.prize, s.retreat
	r.lastPublic, r.firstBlind, r.blind = s.lastPublic, s.firstBlind, false
}

// headline replays one headline and its sublines as one step. Each line is
// atomic: a failing line is undone, reported and skipped.
func (r *replay) headline(n *Node) {
	label := n.Text
	if label == "" && len(n.Children) > 0 {
		label = n.Children[0].Text
	}
	if len(label) > 140 {
		label = label[:140] + "…"
	}
	seat := state.Seat("")
	for name, s := range r.seats {
		if strings.HasPrefix(n.Text, name+" ") || strings.HasPrefix(n.Text, name+"'s ") {
			seat = s
		}
	}
	r.begin(Step{Line: n.No, Label: label, Kind: "log", Seat: seat})
	if n.Text != "" {
		r.line(n, nil, true)
	}
	for _, c := range n.Children {
		if c.Depth == 1 {
			r.line(c, n, false)
		}
	}
	r.end()
	if r.trace != nil {
		fmt.Fprintf(r.trace, "%4d %s\n     %s\n", n.No, n.Text, r.summary())
	}
}

// summary renders the board compactly for tracing.
func (r *replay) summary() string {
	var b strings.Builder
	for _, seat := range state.Seats {
		side := r.st.Sides[seat]
		fmt.Fprintf(&b, "%s deck=%d hand=%d[", seat, len(side.Deck), len(side.Hand))
		for _, h := range side.Hand {
			fmt.Fprintf(&b, "%s,", cmpOr(r.bound[h], "?"))
		}
		b.WriteString("] A=")
		for _, l := range r.st.StackLocs(seat) {
			st, _ := r.st.Stack(l)
			fmt.Fprintf(&b, "%s(", r.bound[st.Top()])
			for _, x := range st.Cards() {
				if x != st.Top() {
					fmt.Fprintf(&b, "%s,", r.bound[x])
				}
			}
			b.WriteString(") ")
		}
		b.WriteString(" | ")
	}
	return b.String()
}

func cmpOr(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

func (r *replay) line(n, parent *Node, isHead bool) {
	r.choiceSeq = 0
	if i := slices.Index(r.flat, n); i >= 0 {
		r.pos = i
	}
	for _, h := range r.nodes {
		m := h.re.FindStringSubmatch(n.Text)
		if m == nil {
			continue
		}
		if isHead {
			if !h.keepKO {
				r.lastKO = nil
			}
			r.lastStadium, r.lastDevolved, r.lastRef = "", "", nil
		}
		snap := r.save()
		if err := h.fn(r, m, n, parent); err != nil {
			r.restore(snap)
			r.problem(n, err)
		}
		return
	}
	r.problem(n, fmt.Errorf("unrecognised line"))
}

// finish assigns identities to every unbound card and rewrites the setup
// event: the owner's unseen cards come from the decklist; the opponent's
// stay hidden unless OppDeck assumes them.
func (r *replay) finish(res *Result, opts Options) error {
	defs := map[string]string{}
	for _, e := range opts.MyDeck {
		defs[e.Name] = e.Def
	}
	oppDefs := map[string]string{}
	for _, e := range opts.OppDeck {
		oppDefs[e.Name] = e.Def
	}

	remaining := map[string]int{}
	for _, e := range opts.MyDeck {
		remaining[e.Name] += e.Count
	}
	for iid, name := range r.bound {
		if r.st.Cards[iid].Owner == MeSeat {
			remaining[name]--
		}
	}
	var pool []string
	for _, e := range opts.MyDeck {
		for range max(remaining[e.Name], 0) {
			pool = append(pool, e.Name)
		}
		remaining[e.Name] = min(remaining[e.Name], 0)
	}
	r.rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })

	var oppPool []string
	if len(opts.OppDeck) > 0 {
		left := map[string]int{}
		for _, e := range opts.OppDeck {
			left[e.Name] += e.Count
		}
		for name, n := range r.rep.Observed {
			if left[name] < n {
				r.rep.Warnings = append(r.rep.Warnings, fmt.Sprintf("the log shows %d %s, the assumed list has %d", n, name, left[name]))
			}
			left[name] -= n
		}
		for _, e := range opts.OppDeck {
			for range max(left[e.Name], 0) {
				oppPool = append(oppPool, e.Name)
			}
			left[e.Name] = min(left[e.Name], 0)
		}
		r.rng.Shuffle(len(oppPool), func(i, j int) { oppPool[i], oppPool[j] = oppPool[j], oppPool[i] })
	}

	setupEv := &res.Steps[0].Events[0]
	var setup state.SetupPayload
	if err := json.Unmarshal(setupEv.Payload, &setup); err != nil {
		return err
	}
	assume := state.AssumePayload{Cards: map[state.IID]state.Assumption{}}
	for i := range setup.Cards {
		c := &setup.Cards[i]
		name := r.bound[c.IID]
		switch {
		case c.Owner == MeSeat && name != "":
			c.Name, c.Def = name, defs[name]
		case c.Owner == MeSeat:
			if len(pool) == 0 {
				r.rep.Warnings = append(r.rep.Warnings, fmt.Sprintf("no decklist card left for %s", c.IID))
				continue
			}
			c.Name, c.Def, c.Basis = pool[0], defs[pool[0]], "decklist"
			pool = pool[1:]
		case name != "":
			c.Name, c.Def, c.Basis = name, oppDefs[name], "observed"
		case len(oppPool) > 0:
			assume.Cards[c.IID] = state.Assumption{Def: oppDefs[oppPool[0]], Name: oppPool[0], Basis: "reference"}
			oppPool = oppPool[1:]
		}
	}
	profile, _ := json.Marshal(map[string]any{"observed": r.rep.Observed, "opponent": r.rep.Opponent})
	setup.Profile = profile
	b, err := json.Marshal(setup)
	if err != nil {
		return err
	}
	setupEv.Payload = b
	if len(assume.Cards) > 0 {
		b, _ := json.Marshal(assume)
		res.Steps = slices.Insert(res.Steps, 1, Step{Label: "Assume opponent list", Kind: "assume", Events: []state.Event{{
			Actor: state.ActorImport, Type: state.EvAssume, Payload: b,
		}}})
	}
	// Renumber: ids are sequential along a single branch.
	id := 1
	for i := range res.Steps {
		for j := range res.Steps[i].Events {
			e := &res.Steps[i].Events[j]
			e.ID, e.Parent = id, id-1
			id++
		}
	}
	return nil
}
