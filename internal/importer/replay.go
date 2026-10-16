package importer

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math/rand/v2"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// Problem is a log line the importer could not apply. The line is skipped
// and the import continues.
type Problem struct {
	Line int    `json:"line"`
	Text string `json:"text"`
	Err  string `json:"error"`
}

// Report summarises an import, in the same shape as the test corpus oracle.
type Report struct {
	Me             string         `json:"me"`
	Opponent       string         `json:"opponent"`
	CoinTossWinner string         `json:"coin_toss_winner"`
	WentFirst      string         `json:"went_first"`
	Turns          map[string]int `json:"turns"`
	TotalTurns     int            `json:"total_turns"`
	PrizesTaken    map[string]int `json:"prizes_taken"`
	Winner         string         `json:"winner"`
	EndReason      string         `json:"end_reason"`
	Mulligans      map[string]int `json:"mulligans,omitempty"`
	// Observed is the minimum count of each opponent card the log proves.
	Observed map[string]int `json:"observed"`
	Problems []Problem      `json:"problems,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

// Step is one log headline (with its sublines) turned into events: one
// playmat action.
type Step struct {
	Line   int
	Label  string
	Kind   string
	Seat   state.Seat
	Events []state.Event
}

// Seats used for imports: the log's owner is p1, the opponent p2.
const (
	MeSeat  = state.P1
	OppSeat = state.P2
)

type replay struct {
	st      *state.State
	seats   map[string]state.Seat
	players map[state.Seat]string
	steps   []Step
	cur     *Step
	nextID  int
	bound   map[state.IID]string
	limit   map[string]int // the owner's decklist counts
	// oppLimit is an assumed opponent list's counts; nil allows MaxCopies.
	oppLimit map[string]int
	used     map[seatName]int
	rep      *Report
	rng      *rand.Rand
	nodes    []handler
	trace    io.Writer
	flat     []*Node // every headline and subline, for lookahead
	pos      int     // index in flat of the line being replayed
	// mulligans lists each player's mulligan reveals, so a redrawn hand can
	// be drawn as what the next reveal shows.
	mulligans map[string][][]string
	// adding names a card about to be attached, for disambiguate.
	adding []string
	// lastRef is the Pokémon the current headline last named (e.g. the one
	// Wally's Compassion healed), where later sublines' cards come from.
	lastRef   *state.Loc
	decisions map[string]int
	// Visibility bookkeeping for repair: blind marks the move being emitted
	// as one the log does not identify.
	blind      bool
	lastPublic map[state.IID]int
	firstBlind map[state.IID]int
	swaps      []swapRec
	choices    []choice
	choiceSeq  int
	// consumed is a headline whose handler already used its sublines. The
	// parse tree is shared by every replay attempt, so it is never mutated.
	consumed *Node
	// boundAt records the log line that revealed each card, for tracing.
	boundAt map[state.IID]int

	pendingPrize   map[state.Seat][]state.IID
	pendingRetreat map[state.Seat]bool
	lastKO         *koInfo
	lastStadium    state.IID
	lastDevolved   state.IID
}

type koInfo struct {
	seat state.Seat
	name string
	iids []state.IID
	top  state.IID
}

type handler struct {
	re *regexp.Regexp
	fn func(r *replay, m []string, n *Node, parent *Node) error
	// keepKO lets discard and prize lines that follow a knockout see it.
	keepKO bool
}

// seatName counts one seat's bound copies of a card name.
type seatName struct {
	seat state.Seat
	name string
}

// MaxCopies is the most copies of a card, by name, a deck may hold. Basic
// Energy is unlimited.
const MaxCopies = 4

// limited reports whether name is subject to MaxCopies.
func limited(name string) bool {
	return !(strings.HasPrefix(name, "Basic ") && strings.HasSuffix(name, " Energy"))
}

func (r *replay) seat(name string) state.Seat { return r.seats[name] }

// emit applies an event to the replay state and records it in the current
// step.
func (r *replay) emit(seat state.Seat, typ state.EventType, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e := state.Event{ID: r.nextID, Parent: r.nextID - 1, Actor: state.ActorImport, Seat: seat, Type: typ, Payload: b}
	if err := r.st.Apply(&e); err != nil {
		return err
	}
	e.Turn = r.st.Turn
	r.nextID++
	r.cur.Events = append(r.cur.Events, e)
	r.track(&e)
	return nil
}

func (r *replay) move(seat state.Seat, iid state.IID, to state.Loc, as, face string) error {
	from, err := r.st.Find(iid)
	if err != nil {
		return err
	}
	return r.emit(seat, state.EvMove, state.MovePayload{IID: iid, From: &from, To: to, As: as, Face: face})
}

// bindable reports whether iid may be (or already is) bound to name.
func (r *replay) bindable(iid state.IID, name string) bool {
	if b := r.bound[iid]; b != "" {
		return b == name
	}
	owner := r.st.Cards[iid].Owner
	if owner == MeSeat && r.limit != nil {
		return r.used[seatName{owner, name}] < r.limit[name]
	}
	// The opponent's list is unknown, but no deck holds more than
	// MaxCopies (or, with an assumed list, its count): past that, a named
	// card must be a copy already seen.
	if r.oppLimit != nil {
		return r.used[seatName{owner, name}] < r.oppLimit[name]
	}
	return !limited(name) || r.used[seatName{owner, name}] < MaxCopies
}

func (r *replay) bind(iid state.IID, name string) error {
	if name == "" {
		return nil
	}
	if b := r.bound[iid]; b != "" {
		if b != name {
			return fmt.Errorf("%s is already %s, not %s", iid, b, name)
		}
		return nil
	}
	if !r.bindable(iid, name) {
		if r.st.Cards[iid].Owner == MeSeat {
			return fmt.Errorf("the decklist has only %d %s", r.limit[name], name)
		}
		if r.oppLimit != nil {
			return fmt.Errorf("the assumed list has only %d %s", r.oppLimit[name], name)
		}
		return fmt.Errorf("%s already has %d %s", r.players[r.st.Cards[iid].Owner], MaxCopies, name)
	}
	r.bound[iid] = name
	if r.boundAt != nil && r.pos < len(r.flat) {
		r.boundAt[iid] = r.flat[r.pos].No
	}
	r.st.Cards[iid].Name = name
	r.used[seatName{r.st.Cards[iid].Owner, name}]++
	return nil
}

// zoneCards lists the cards of a non-stack zone.
func (r *replay) zoneCards(seat state.Seat, z state.Zone) []state.IID {
	side := r.st.Sides[seat]
	switch z {
	case state.Deck:
		return side.Deck
	case state.Hand:
		return side.Hand
	case state.Discard:
		return side.Discard
	case state.Lost:
		return side.Lost
	}
	return nil
}

// pick finds a card in one of zones that is (or can become) name. A card
// already bound to the name wins over an unbound one. name "" picks any
// card, preferring unbound ones.
func (r *replay) pick(seat state.Seat, zones []state.Zone, name string, exclude []state.IID) (state.IID, error) {
	if name == "" {
		// An unnamed card could be any card there: pick at random so cards
		// already known (e.g. from a mulligan reveal) are drawn as often as
		// they would be.
		for _, z := range zones {
			cards := slices.DeleteFunc(slices.Clone(r.zoneCards(seat, z)), func(x state.IID) bool { return slices.Contains(exclude, x) })
			if len(cards) > 0 {
				return cards[r.rng.IntN(len(cards))], nil
			}
		}
	}
	for _, z := range zones {
		cards := r.zoneCards(seat, z)
		for _, iid := range cards {
			if name != "" && r.bound[iid] == name && !slices.Contains(exclude, iid) {
				return iid, nil
			}
		}
		for _, iid := range cards {
			if r.bound[iid] == "" && r.bindable(iid, name) && !slices.Contains(exclude, iid) {
				return iid, nil
			}
		}
		if name == "" {
			for _, iid := range cards {
				if !slices.Contains(exclude, iid) {
					return iid, nil
				}
			}
		}
	}
	if name != "" {
		for _, z := range zones {
			hs := slices.DeleteFunc(slices.Clone(r.zoneCards(seat, z)), func(x state.IID) bool { return slices.Contains(exclude, x) })
			if d, ok := r.repairAmong(seat, hs, name); ok {
				return d, nil
			}
		}
		for _, z := range zones {
			if d, ok := r.reclaim(seat, z, name, exclude); ok {
				return d, nil
			}
		}
	}
	what := name
	if what == "" {
		what = "a card"
	}
	zs := make([]string, len(zones))
	for i, z := range zones {
		zs[i] = string(z)
	}
	return "", fmt.Errorf("no %s for %s in %s", what, r.players[seat], strings.Join(zs, "/"))
}

// take picks a card and binds it.
func (r *replay) take(seat state.Seat, zones []state.Zone, name string) (state.IID, error) {
	iid, err := r.pick(seat, zones, name, nil)
	if err != nil {
		return "", err
	}
	return iid, r.bind(iid, name)
}

// stackNamed finds seat's Pokémon whose current card is name. zone narrows
// to active or bench; "" tries active first.
func (r *replay) stackNamed(seat state.Seat, name, zone string) (state.Loc, error) {
	var cands []state.Loc
	for _, l := range r.st.StackLocs(seat) {
		if zone == "active" && l.Zone != state.Active || zone == "bench" && l.Zone != state.Bench {
			continue
		}
		st, _ := r.st.Stack(l)
		if r.bound[st.Top()] == name {
			cands = append(cands, l)
		}
	}
	if len(cands) == 1 {
		return cands[0], nil
	}
	if len(cands) > 1 {
		// Identical copies: a choice point. The default is the Active
		// Pokémon when allowed, else the lookahead's pick.
		def := cands[0]
		if cands[0].Zone != state.Active || zone != "" {
			def = r.disambiguate(seat, name, cands)
		}
		used := slices.IndexFunc(cands, func(l state.Loc) bool { return l.SameStack(def) })
		key := fmt.Sprintf("%d/%d", r.pos, r.choiceSeq)
		r.choiceSeq++
		if d, ok := r.decisions[key]; ok && d < len(cands) {
			used = d
		}
		r.choices = append(r.choices, choice{key: key, line: r.flat[r.pos].No, n: len(cands), used: used})
		return cands[used], nil
	}
	where := ""
	if zone != "" {
		where = " in the " + zone
	}
	return state.Loc{}, fmt.Errorf("%s has no %s in play%s", r.players[seat], name, where)
}

// stackIs reports whether names lists exactly the Pokémon cards in the stack
// at l, in any order.
func (r *replay) stackIs(l state.Loc, names []string) bool {
	st, err := r.st.Stack(l)
	if err != nil || len(names) == 0 || len(names) != len(st.Pokemon) {
		return false
	}
	left := slices.Clone(names)
	for _, iid := range st.Pokemon {
		i := slices.Index(left, r.bound[iid])
		if i < 0 {
			return false
		}
		left = slices.Delete(left, i, i+1)
	}
	return true
}

// target resolves a Pokémon named by a damage line. The client sometimes
// prints the acting player as the owner of the target, so when the stated
// owner is the actor, the opponent's side is tried first.
func (r *replay) target(actor, stated state.Seat, name string) (state.Loc, error) {
	order := []state.Seat{stated, stated.Other()}
	if actor == stated {
		order = []state.Seat{stated.Other(), stated}
	}
	var firstErr error
	for _, s := range order {
		l, err := r.stackNamed(s, name, "")
		if err == nil {
			return l, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return state.Loc{}, firstErr
}

func zoneHint(where string) string {
	if strings.Contains(where, "Active") {
		return "active"
	}
	return "bench"
}

func (r *replay) addDamage(seat state.Seat, l state.Loc, amount int) error {
	st, err := r.st.Stack(l)
	if err != nil {
		return err
	}
	if st.Top() == "" {
		return fmt.Errorf("no Pokémon in %s", l)
	}
	return r.emit(seat, state.EvDamage, state.DamagePayload{Target: l, Value: max(st.Damage+amount, 0)})
}

// toActive makes the benched Pokémon at b the Active Pokémon. Special
// Conditions on the outgoing Active Pokémon are cleared.
func (r *replay) toActive(seat state.Seat, b state.Loc) error {
	a := state.Loc{Seat: b.Seat, Zone: state.Active}
	if st, _ := r.st.Stack(a); st != nil && len(st.Conditions) > 0 {
		if err := r.emit(seat, state.EvCondition, state.ConditionPayload{Target: a, Condition: "all"}); err != nil {
			return err
		}
	}
	return r.emit(seat, state.EvSwap, state.SwapPayload{A: b, B: a})
}

func (r *replay) shuffle(seat state.Seat) error {
	order := slices.Clone(r.st.Sides[seat].Deck)
	r.rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	return r.emit(seat, state.EvShuffle, state.ShufflePayload{Seat: seat, Order: order})
}

// drawShown draws like draw, for a line that names the cards: the log's
// owner saw them, so the opponent's named cards are revealed.
func (r *replay) drawShown(seat state.Seat, n int, names []string) error {
	before := len(r.st.Sides[seat].Hand)
	if err := r.draw(seat, n, names); err != nil {
		return err
	}
	if seat != OppSeat || len(names) == 0 {
		return nil
	}
	// Only the cards the line names were seen: draw takes those first. An
	// unnamed card may still be bound from an earlier sighting, but nobody
	// saw it this time.
	var shown []state.IID
	for i, iid := range r.st.Sides[seat].Hand[before:] {
		if i < len(names) && names[i] != "" {
			shown = append(shown, iid)
		}
	}
	if len(shown) == 0 {
		return nil
	}
	return r.emit(seat, state.EvReveal, state.RevealPayload{IIDs: shown, To: []state.Seat{MeSeat}})
}

// draw moves cards from deck to hand, binding names when given. Deck order
// is unknown, so the drawn card is whichever matches.
func (r *replay) draw(seat state.Seat, n int, names []string) error {
	for i := 0; i < n; i++ {
		name := ""
		if i < len(names) {
			name = names[i]
		}
		iid, err := r.take(seat, []state.Zone{state.Deck}, name)
		if err != nil {
			return err
		}
		top := state.Top
		r.blind = name == ""
		err = r.emit(seat, state.EvMove, state.MovePayload{
			IID: iid, From: &state.Loc{Seat: seat, Zone: state.Deck, Pos: &top}, To: state.Loc{Seat: seat, Zone: state.Hand},
		})
		r.blind = false
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *replay) dealPrizes() error {
	for _, seat := range state.Seats {
		side := r.st.Sides[seat]
		var taken []state.IID
		for i := range side.Prizes {
			if side.Prizes[i] != "" {
				continue
			}
			// Prizes are only identified when taken, so deal never-revealed
			// cards first; a known card in a prize slot could contradict it.
			var unknown []state.IID
			for _, iid := range side.Deck {
				if r.bound[iid] == "" && !slices.Contains(taken, iid) {
					unknown = append(unknown, iid)
				}
			}
			var iid state.IID
			if len(unknown) > 0 {
				iid = unknown[r.rng.IntN(len(unknown))]
			} else {
				var err error
				if iid, err = r.pick(seat, []state.Zone{state.Deck}, "", taken); err != nil {
					return err
				}
			}
			taken = append(taken, iid)
			r.blind = true
			err := r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Prizes, Slot: i}, "", "down")
			r.blind = false
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func count(s string) int {
	if s == "a" || s == "an" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}

// deckish reports whether a headline's sublines shuffle the deck, which
// marks cards it moves as coming from the deck (a search).
func deckish(parent *Node) bool {
	if parent == nil {
		return false
	}
	for _, c := range parent.Children {
		if strings.HasSuffix(c.Text, " shuffled their deck.") {
			return true
		}
	}
	return false
}

var discardListRe = regexp.MustCompile(`^(?:\d+ cards were|(.+) was) discarded from (.+)'s (.+)\.$`)

// disambiguate chooses between Pokémon with the same name, which the log
// cannot tell apart. It looks ahead for lists of cards later discarded from
// a Pokémon of that name and picks the copy whose cards (plus r.adding, a
// card about to be attached) fit the earliest such list best.
func (r *replay) disambiguate(seat state.Seat, name string, cands []state.Loc) state.Loc {
	for i := r.pos + 1; i < len(r.flat) && i < r.pos+150; i++ {
		n := r.flat[i]
		m := discardListRe.FindStringSubmatch(n.Text)
		if m == nil || r.seats[m[2]] != seat || m[3] != name {
			continue
		}
		list := map[string]int{}
		if m[1] != "" {
			list[m[1]]++
		}
		for _, nm := range n.Bullets() {
			list[nm]++
		}
		best, bestScore := -1, -1
		for ci, l := range cands {
			st, _ := r.st.Stack(l)
			left := maps.Clone(list)
			fits, score := true, 0
			held := r.adding
			for _, iid := range st.Cards() {
				if iid != st.Top() {
					held = append(slices.Clone(held), r.bound[iid])
				}
			}
			for _, nm := range held {
				if left[nm] > 0 {
					left[nm]--
					score++
				} else if m[1] == "" {
					fits = false // a full list must contain everything held
				}
			}
			if fits && score > bestScore {
				best, bestScore = ci, score
			}
		}
		if best >= 0 {
			return cands[best]
		}
	}
	return cands[0]
}
