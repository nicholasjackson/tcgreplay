package importer

import (
	"regexp"
	"slices"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// Card effects whose consequences TCG Live's battle log does not print.
// The log shows that the effect happened (a Trainer played, a Pokémon
// healed) but not every card it moved, and later lines depend on those
// moves. Each table here is keyed by the card's log name, holds the card
// text it follows, and is run by the handler for the line that shows the
// effect happened.

// afterHeal lists effects that follow a heal by the card played in the
// headline the heal line sits under. l is the Pokémon healed.
var afterHeal = map[string]func(r *replay, seat state.Seat, l state.Loc, n, parent *Node) error{
	// Wally's Compassion me01-132: "Heal all damage from 1 of your Mega
	// Evolution Pokémon ex. If you healed any damage in this way, put all
	// Energy attached to that Pokémon into your hand." Newer logs print
	// only the heal; older ones also print the Energy moving to the hand.
	"Wally's Compassion": func(r *replay, seat state.Seat, l state.Loc, n, parent *Node) error {
		if laterSubline(parent, n, func(c *Node) bool {
			return strings.Contains(c.Text, " moved ") && strings.HasSuffix(c.Text, " to their hand.")
		}) {
			return nil // the log prints the move; its own line does it
		}
		st, err := r.st.Stack(l)
		if err != nil {
			return err
		}
		for _, iid := range slices.Clone(st.Energy) {
			if err := r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Hand}, "", ""); err != nil {
				return err
			}
		}
		return nil
	},
}

var playedRe = regexp.MustCompile(`^(.+) played (.+)\.$`)

// playedCard is the card a headline like "NicJackson played Wally's
// Compassion." says seat's player played, or "".
func (r *replay) playedCard(seat state.Seat, head *Node) string {
	if head == nil {
		return ""
	}
	m := playedRe.FindStringSubmatch(head.Text)
	if m == nil || r.seats[m[1]] != seat {
		return ""
	}
	return m[2]
}

// laterSubline reports whether a subline of parent after n satisfies ok.
func laterSubline(parent, n *Node, ok func(*Node) bool) bool {
	if parent == nil {
		return false
	}
	i := slices.Index(parent.Children, n)
	if i < 0 {
		return false
	}
	return slices.ContainsFunc(parent.Children[i+1:], ok)
}
