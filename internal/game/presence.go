package game

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/store"
)

// Timing holds the turn wait's settings. A zero field means its default.
type Timing struct {
	// WaitCap is the longest a single Wait call blocks (default 30 s).
	WaitCap time.Duration
	// CheckIn is how long the player may leave the board untouched before
	// a wait reports a check-in (default 5 min).
	CheckIn time.Duration
	// LockRelease is how long the assistant may go without a move before
	// the board unlocks by itself (default 2 min).
	LockRelease time.Duration
}

func (t Timing) withDefaults() Timing {
	if t.WaitCap <= 0 {
		t.WaitCap = 30 * time.Second
	}
	if t.CheckIn <= 0 {
		t.CheckIn = 5 * time.Minute
	}
	if t.LockRelease <= 0 {
		t.LockRelease = 2 * time.Minute
	}
	return t
}

// The outcomes a Wait reports.
const (
	WaitTurnEnded    = "turn_ended"
	WaitRespond      = "respond"
	WaitPromote      = "promote"
	WaitCheckIn      = "check_in"
	WaitEnded        = "ended"
	WaitStillWaiting = "still_waiting"
)

// The reasons an ended wait gives.
const (
	EndModeChanged    = "mode_changed"
	EndGameReplaced   = "game_replaced"
	EndGameClosed     = "game_closed"
	EndGameExpired    = "game_expired"
	EndBranchSwitched = "branch_switched"
	EndRewound        = "rewound"
)

// WaitRequest asks to wait while the player plays Seat's turn. A nil
// Since starts a fresh wait; otherwise it continues from the previous
// outcome's Next.
type WaitRequest struct {
	Seat  state.Seat
	Since *int
}

// WaitOutcome is what a Wait call reports.
type WaitOutcome struct {
	Kind string
	Seat state.Seat
	// Since is where the player's moves this outcome covers start.
	Since int
	// Next is the cursor to pass as Since on the next call.
	Next int
	// Turn and TurnSeat name the turn now in progress (turn_ended).
	Turn     int
	TurnSeat state.Seat
	// Prompt is the shared hand-off text (turn_ended).
	Prompt string
	// Cards are the cards, Abilities and attacks the player used (respond).
	Cards []UsedCard
	// Reason says why the wait ended (ended).
	Reason string
}

// UsedCard is a card, Ability or attack the player used: the card's iid
// and definition, the action's name ("Judge", "Lucario: Aura Sphere"),
// its kind (trainer, ability or attack) and the text of what was used.
type UsedCard struct {
	IID                   state.IID
	Name, Def, Kind, Text string
}

// openerKinds are the actions whose effects may ask the other side to act.
var openerKinds = map[string]bool{"trainer": true, "ability": true, "attack": true}

// presence is what the assistant is doing on its manager's current game:
// its wait session, and when the player last touched the board. Waiting
// calls block on changed, which is closed and replaced on every change, so
// a call registers nothing that outlives it.
//
// Lock order: game listeners run with the game locked and take p.mu, so
// p.mu is never held while calling into a Game.
type presence struct {
	mu      sync.Mutex
	changed chan struct{}
	// game is the manager's current game; watched is every game the
	// presence has subscribed to (only the latest is kept, since a game
	// listener cannot be removed and old games should be freed).
	game    *Game
	watched *Game
	session *waitSession
	// callGen counts Wait calls: a call whose generation is no longer the
	// latest has been superseded.
	callGen  int
	inFlight bool
	// lastBoard is when the player last did anything on the board.
	lastBoard time.Time
	onChange  []func()

	// acting is the acting lock: the assistant is taking its turn or
	// answering the player, so the board refuses the player's game
	// actions. stopped is set when the lock released itself because the
	// assistant went lockRelease without a move. releaseTimer does that
	// release; it is made once and reset, never stacked.
	acting, stopped bool
	lastMove        time.Time
	lockRelease     time.Duration
	releaseTimer    *time.Timer
	// waitingUntil keeps the board's "waiting" status between two polls;
	// graceTimer pushes the change when it lapses.
	waitingUntil time.Time
	graceTimer   *time.Timer
}

// waitingGrace is how long the board keeps showing the assistant as
// waiting after a call reports still_waiting, so the next call can renew
// it. With the 50 s maximum cap it still clears within a minute.
var waitingGrace = 20 * time.Second

// waitSession is one hand-over of a turn to the player.
type waitSession struct {
	game *Game
	seat state.Seat
	// cursor is the event the player's unreported moves start after.
	cursor int
	// idleFrom starts the check-in clock; checkedIn is set once a check-in
	// has been reported, so the next call restarts the clock.
	idleFrom  time.Time
	checkedIn bool
	// ended, when set, is why the session ended under the wait.
	ended string
	// koReported and reported record what was already reported mid-turn:
	// the vacancy of the assistant's Active Spot and the actions (by id).
	koReported bool
	reported   map[string]bool
}

func newPresence() *presence {
	return &presence{changed: make(chan struct{})}
}

// bumpLocked wakes every waiting call. Call with p.mu held.
func (p *presence) bumpLocked() {
	close(p.changed)
	p.changed = make(chan struct{})
}

// endLocked ends the live session, if any, with reason. Call with p.mu
// held.
func (p *presence) endLocked(reason string) {
	if p.session != nil && p.session.ended == "" {
		p.session.ended = reason
	}
}

// gameChanged makes g the presence's game, ending a session on another
// game, and subscribes to g once.
func (p *presence) gameChanged(g *Game) {
	p.mu.Lock()
	if p.session != nil && p.session.game != g {
		p.endLocked(EndGameReplaced)
	}
	p.game = g
	p.clearActingLocked()
	first := p.watched != g
	p.watched = g
	p.bumpLocked()
	p.mu.Unlock()
	if first {
		g.Subscribe(func(u Update) { p.gameUpdate(g, u) })
	}
	p.notify()
}

// cleared drops the presence's game after it expired.
func (p *presence) cleared() {
	p.mu.Lock()
	p.endLocked(EndGameExpired)
	p.game = nil
	p.clearActingLocked()
	p.bumpLocked()
	p.mu.Unlock()
	p.notify()
}

// gameUpdate is g's listener. It runs with g locked, so it only records
// why a session ended and wakes the waiting calls. A moved board (mode,
// branch, rewind, undo) also releases the acting lock; open boards refresh
// on the game's own notice.
func (p *presence) gameUpdate(g *Game, u Update) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if u.Change != "" && p.game == g {
		p.clearActingLocked()
	}
	if p.session != nil && p.session.game == g {
		switch u.Change {
		case ChangeMode:
			p.endLocked(EndModeChanged)
		case ChangeCheckout:
			p.endLocked(EndBranchSwitched)
		case ChangeRewind, ChangeUndo:
			p.endLocked(EndRewound)
		}
	}
	p.bumpLocked()
}

// boardActivity notes that the player did something on the board.
func (p *presence) boardActivity() {
	p.mu.Lock()
	p.lastBoard = time.Now()
	p.bumpLocked()
	p.mu.Unlock()
}

// startActingLocked sets the acting lock, or refreshes it, from a move
// now. Call with p.mu held.
func (p *presence) startActingLocked(release time.Duration) {
	p.acting, p.stopped = true, false
	p.lastMove = time.Now()
	p.lockRelease = release
	if p.releaseTimer == nil {
		p.releaseTimer = time.AfterFunc(release, p.releaseFired)
	} else {
		p.releaseTimer.Reset(release)
	}
}

// clearActingLocked releases the acting lock and forgets a stop. Call with
// p.mu held.
func (p *presence) clearActingLocked() {
	p.acting, p.stopped = false, false
	if p.releaseTimer != nil {
		p.releaseTimer.Stop()
	}
}

// releaseFired releases a lock the assistant left for lockRelease without
// a move, and marks it stopped responding.
func (p *presence) releaseFired() {
	p.mu.Lock()
	fire := p.acting && time.Since(p.lastMove) >= p.lockRelease
	if fire {
		p.acting, p.stopped = false, true
	}
	p.mu.Unlock()
	if fire {
		p.notify()
	}
}

// waitingFor keeps the board's "waiting" status for d after a call, or
// clears it at once when d is 0. Call with p.mu held.
func (p *presence) waitingForLocked(d time.Duration) {
	if d <= 0 {
		p.waitingUntil = time.Time{}
		return
	}
	p.waitingUntil = time.Now().Add(d)
	if p.graceTimer == nil {
		p.graceTimer = time.AfterFunc(d, p.notify)
	} else {
		p.graceTimer.Reset(d)
	}
}

// opponentLocked is what the board shows of the assistant. Call with p.mu
// held.
func (p *presence) opponentLocked() string {
	switch {
	case p.acting:
		return "acting"
	case p.stopped:
		return "stopped"
	case p.inFlight || time.Now().Before(p.waitingUntil):
		return "waiting"
	}
	return ""
}

// notify runs the change hooks. Call without p.mu held.
func (p *presence) notify() {
	p.mu.Lock()
	hooks := slices.Clone(p.onChange)
	p.mu.Unlock()
	for _, f := range hooks {
		f()
	}
}

// waitView is what a wait needs from the game, read in one lock.
type waitView struct {
	// onPath is false when the cursor is no longer on the current branch.
	onPath bool
	// turnEnded is set when seat ended a turn after the cursor;
	// movedSince when a non-system event followed that end.
	turnEnded  bool
	movedSince bool
	head       int
	mode       Mode
	current    state.Seat
	turnOpen   bool
	// activeEmpty is set when the other seat (the assistant's) has no
	// Active Pokémon during play.
	activeEmpty bool
	// used holds the cards, Abilities and attacks seat's human player used
	// after the cursor, by action id, in order.
	used    []UsedCard
	usedIDs []string
}

func (g *Game) waitView(cursor int, seat state.Seat) waitView {
	g.mu.Lock()
	defer g.mu.Unlock()
	v := waitView{head: g.log.Head(), mode: g.mode, current: g.st.Current, turnOpen: g.st.TurnOpen}
	if side := g.st.Sides[seat.Other()]; side != nil && g.st.Phase == state.PhasePlaying {
		v.activeEmpty = side.Active.Empty()
	}
	evs, ok := g.eventsSince(cursor)
	v.onPath = ok
	seen := map[string]bool{}
	for i := range evs {
		e := &evs[i]
		if v.turnEnded {
			if e.Actor != state.ActorSystem {
				v.movedSince = true
			}
			continue
		}
		if e.Type == state.EvTurn && e.Seat == seat {
			if p, err := e.Decode(); err == nil && p.(*state.TurnPayload).Phase == "end" {
				v.turnEnded = true
			}
		}
		a := g.actionOf(e.Action)
		if a == nil || seen[a.ID] || a.Seat != seat || a.Actor != state.ActorHuman || !openerKinds[a.Kind] {
			continue
		}
		seen[a.ID] = true
		v.used = append(v.used, g.usedCard(a))
		v.usedIDs = append(v.usedIDs, a.ID)
	}
	return v
}

// usedCard describes an opener action for the assistant: the card and the
// text of the Trainer, Ability or attack used, the card's full text when
// the Ability or attack is not found, and no text when the card is
// unknown. Call with g.mu held.
func (g *Game) usedCard(a *store.Action) UsedCard {
	u := UsedCard{IID: a.Source, Name: a.Label, Kind: a.Kind}
	if c, ok := g.st.Cards[a.Source]; ok {
		u.Def = c.Def
	}
	c := g.card(g.st, a.Source)
	if c == nil {
		return u
	}
	_, part, _ := strings.Cut(a.Label, ": ")
	switch a.Kind {
	case "trainer":
		u.Text = c.Effect
		return u
	case "ability":
		if i := slices.IndexFunc(c.Abilities, func(x cards.Ability) bool { return strings.EqualFold(x.Name, part) }); i >= 0 {
			u.Text = c.Abilities[i].Effect
			return u
		}
	case "attack":
		if i := slices.IndexFunc(c.Attacks, func(x cards.Attack) bool { return strings.EqualFold(x.Name, part) }); i >= 0 {
			u.Text = c.Attacks[i].Effect
			return u
		}
	}
	u.Text = c.Text()
	return u
}

func (m *Manager) presenceRef() *presence {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.presence == nil {
		m.presence = newPresence()
	}
	return m.presence
}

// OnPresence registers a hook run whenever what the assistant is doing
// changes. Hooks must not block.
func (m *Manager) OnPresence(f func()) {
	p := m.presenceRef()
	p.mu.Lock()
	p.onChange = append(p.onChange, f)
	p.mu.Unlock()
}

// BoardActivity notes that the player did something on the board, which
// restarts the check-in clock.
func (m *Manager) BoardActivity() {
	m.presenceRef().boardActivity()
}

// Locked reports whether the assistant is acting, so the board refuses
// the player's game actions.
func (m *Manager) Locked() bool {
	p := m.presenceRef()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.acting
}

// Opponent is what the board shows of the assistant: "waiting" while it
// waits for the player's turn, "acting" while the board is locked for it,
// "stopped" after the lock released itself, or "".
func (m *Manager) Opponent() string {
	p := m.presenceRef()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.opponentLocked()
}

// AssistantMoved notes one of the assistant's tool calls. It refreshes the
// acting lock's clock while the lock is set; ownTurn, for a write on the
// assistant's own turn in play mode, sets the lock even when no wait came
// first, or again after it released itself.
func (m *Manager) AssistantMoved(ownTurn bool) {
	p := m.presenceRef()
	p.mu.Lock()
	changed := ownTurn && !p.acting
	if p.acting || ownTurn {
		p.startActingLocked(m.Timing.withDefaults().LockRelease)
	}
	p.mu.Unlock()
	if changed {
		p.notify()
	}
}

// AssistantDone notes that the assistant ended its turn: the board
// unlocks.
func (m *Manager) AssistantDone() {
	p := m.presenceRef()
	p.mu.Lock()
	changed := p.acting || p.stopped
	p.clearActingLocked()
	p.mu.Unlock()
	if changed {
		p.notify()
	}
}

// Wait blocks while the player plays req.Seat's turn, for at most the
// wait cap, and reports what the assistant should do: the turn ended, the
// player has been idle long enough for a check-in, the game changed under
// the wait, or it is still waiting. A fresh wait in play mode during the
// other seat's turn is refused unless the player's turn ended with nothing
// done since.
func (m *Manager) Wait(ctx context.Context, req WaitRequest) (out WaitOutcome, err error) {
	if !req.Seat.Valid() {
		return WaitOutcome{}, fmt.Errorf("unknown seat %q", req.Seat)
	}
	t := m.Timing.withDefaults()
	p := m.presenceRef()
	out = WaitOutcome{Seat: req.Seat}
	g, err := m.Current()
	if err != nil {
		out.Kind, out.Reason = WaitEnded, EndGameClosed
		if errors.Is(err, ErrExpired) {
			out.Reason = EndGameExpired
		}
		if req.Since != nil {
			out.Since, out.Next = *req.Since, *req.Since
		}
		p.mu.Lock()
		p.session = nil
		p.mu.Unlock()
		return out, nil
	}

	// A new call closes the response window an earlier outcome opened.
	g.setResponder("")

	// Start or continue the session.
	var fresh *waitSession
	if req.Since == nil {
		cursor := g.TurnStartedAt(req.Seat)
		v := g.waitView(cursor, req.Seat)
		if v.current != req.Seat || !v.turnOpen {
			switch {
			case v.mode == ModeReview:
				// Wait for the seat's next turn.
				cursor = v.head
			case v.current == "":
				return WaitOutcome{}, errors.New("no turn is in progress")
			case !v.turnEnded || v.movedSince:
				return WaitOutcome{}, fmt.Errorf("it is %s's turn: end_turn first, then wait", v.current)
			}
		}
		fresh = &waitSession{game: g, seat: req.Seat, cursor: cursor, idleFrom: time.Now()}
	}
	p.mu.Lock()
	switch {
	case fresh != nil:
		p.session = fresh
	case p.session == nil || p.session.seat != req.Seat:
		p.session = &waitSession{game: g, seat: req.Seat, cursor: *req.Since, idleFrom: time.Now()}
	default:
		if p.session.checkedIn {
			p.session.idleFrom, p.session.checkedIn = time.Now(), false
		}
		p.session.cursor = *req.Since
	}
	s := p.session
	p.callGen++
	gen := p.callGen
	p.inFlight = true
	p.clearActingLocked() // waiting again releases the board
	p.bumpLocked()        // supersedes any call still in flight
	p.mu.Unlock()
	p.notify()
	defer func() {
		// What the board shows next: the assistant acts on a hand-off or
		// a mid-turn outcome, keeps showing as waiting between polls, and
		// stops showing as waiting at once after a check-in or an end.
		p.mu.Lock()
		if p.callGen == gen {
			p.inFlight = false
			switch out.Kind {
			case WaitTurnEnded, WaitRespond, WaitPromote:
				p.waitingForLocked(0)
				p.startActingLocked(t.LockRelease)
			case WaitStillWaiting:
				p.waitingForLocked(waitingGrace)
			default:
				p.waitingForLocked(0)
			}
		}
		p.mu.Unlock()
		p.notify()
	}()

	capTimer := time.NewTimer(t.WaitCap)
	defer capTimer.Stop()
	idleTimer := time.NewTimer(t.CheckIn)
	defer idleTimer.Stop()
	for {
		p.mu.Lock()
		changed := p.changed
		out.Since, out.Next = s.cursor, s.cursor
		if p.callGen != gen {
			p.mu.Unlock()
			out.Kind = WaitStillWaiting
			return out, nil
		}
		if s.ended != "" {
			if p.session == s {
				p.session = nil
			}
			p.mu.Unlock()
			s.game.setResponder("")
			out.Kind, out.Reason = WaitEnded, s.ended
			return out, nil
		}
		cursor := s.cursor
		idleAt := s.idleFrom
		if p.lastBoard.After(idleAt) {
			idleAt = p.lastBoard
		}
		p.mu.Unlock()

		v := s.game.waitView(cursor, s.seat)
		switch {
		case !v.onPath:
			// The board moved off the cursor's line between notices.
			p.mu.Lock()
			if s.ended == "" {
				s.ended = EndRewound
			}
			p.mu.Unlock()
			continue
		case v.turnEnded:
			p.mu.Lock()
			if p.session == s {
				p.session = nil
			}
			p.mu.Unlock()
			st := s.game.State()
			out.Kind, out.Next = WaitTurnEnded, v.head
			out.Turn, out.TurnSeat = state.Round(st.Turn), st.Current
			out.Prompt = s.game.HandOff(cursor)
			return out, nil
		}

		// Mid-turn: the assistant's Active was emptied (once per vacancy),
		// or the player used cards not yet reported. Either opens the
		// response window for the assistant's seat. The cursor stays at
		// the turn's start so the hand-off still covers the whole turn.
		p.mu.Lock()
		promote := v.activeEmpty && !s.koReported
		s.koReported = v.activeEmpty
		var used []UsedCard
		if !promote {
			if s.reported == nil {
				s.reported = map[string]bool{}
			}
			for i, id := range v.usedIDs {
				if !s.reported[id] {
					s.reported[id] = true
					used = append(used, v.used[i])
				}
			}
		}
		p.mu.Unlock()
		if promote || len(used) > 0 {
			s.game.setResponder(s.seat.Other())
			out.Kind, out.Cards = WaitRespond, used
			if promote {
				out.Kind = WaitPromote
			}
			return out, nil
		}
		idle := time.Until(idleAt.Add(t.CheckIn))
		if idle <= 0 {
			p.mu.Lock()
			s.checkedIn = true
			p.mu.Unlock()
			out.Kind = WaitCheckIn
			return out, nil
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(idle)

		select {
		case <-changed:
		case <-idleTimer.C:
		case <-capTimer.C:
			out.Kind = WaitStillWaiting
			return out, nil
		case <-ctx.Done():
			out.Kind = WaitStillWaiting
			return out, ctx.Err()
		}
	}
}
