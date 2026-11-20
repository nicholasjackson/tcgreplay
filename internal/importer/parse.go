// Package importer reconstructs a playmat game from a Pokémon TCG Live battle
// log. The importing player's side is exact up to deck order; the opponent's
// side is a plausible reconstruction: cards are bound to names only when the
// log reveals them, and the rest stay hidden.
package importer

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Node is one log line and the lines nested under it. Headlines have depth
// 0, "- " sublines depth 1 and "   • " bullet lists depth 2.
type Node struct {
	No       int
	Depth    int
	Text     string
	Items    []string // bullet items, depth 2 only
	Children []*Node
}

// Bullets returns the items of the first bullet child, if any.
func (n *Node) Bullets() []string {
	for _, c := range n.Children {
		if c.Depth == 2 {
			return c.Items
		}
	}
	return nil
}

// Section is a "Setup", "<player>'s Turn" or "Pokémon Checkup" block.
type Section struct {
	No     int
	Kind   string // setup, turn, checkup
	Player string // turn sections only
	Lines  []*Node
}

// logCodeRe matches the card code TCG Live puts before each card name, e.g.
// "(me3_121) Meowth ex" or "(me2_85_ph) Battle Cage".
var logCodeRe = regexp.MustCompile(`\(([a-z][a-z0-9-]*_\d+(?:_[a-z0-9]+)*)\) `)

// Normalize folds the typographic variants the client mixes and drops the
// card codes, so cards are matched by name as in logs without them.
func Normalize(s string) string {
	s = strings.NewReplacer("\u2019", "'", "\u2018", "'", "\r", "", "\ufeff", "").Replace(s)
	return logCodeRe.ReplaceAllString(s, "")
}

// lineCode is one card code on a log line and the text after it, which
// starts with the card's name.
type lineCode struct {
	code, rest string
}

// lineCodes returns the card codes on each line of a log, keyed by line
// number as Parse numbers lines; nil for a log without codes.
func lineCodes(text string) map[int][]lineCode {
	text = strings.NewReplacer("\u2019", "'", "\u2018", "'", "\r", "", "\ufeff", "").Replace(text)
	var out map[int][]lineCode
	for i, raw := range strings.Split(text, "\n") {
		for _, m := range logCodeRe.FindAllStringSubmatchIndex(raw, -1) {
			if out == nil {
				out = map[int][]lineCode{}
			}
			out[i+1] = append(out[i+1], lineCode{code: raw[m[2]:m[3]], rest: raw[m[1]:]})
		}
	}
	return out
}

// codeFor returns the one card code a line gives for name, or "" when it
// gives none or several different ones (two prints of the card on one
// line, as in a list of drawn cards).
func codeFor(codes []lineCode, name string) string {
	found := ""
	for _, c := range codes {
		if !strings.HasPrefix(c.rest, name) {
			continue
		}
		if found != "" && found != c.code {
			return ""
		}
		found = c.code
	}
	return found
}

// LogCodes returns the card codes the log gives for name, in order of
// first appearance; nil for a log without codes.
func LogCodes(text, name string) []string {
	text = strings.NewReplacer("\u2019", "'", "\u2018", "'").Replace(text)
	re := regexp.MustCompile(`\(([a-z][a-z0-9-]*_\d+(?:_[a-z0-9]+)*)\) ` + regexp.QuoteMeta(name))
	var out []string
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

var turnRe = regexp.MustCompile(`^(.+)'s Turn$`)

// Parse splits a log into sections of nested lines.
func Parse(text string) ([]*Section, error) {
	var secs []*Section
	var cur *Section
	var head, sub *Node
	for i, raw := range strings.Split(Normalize(text), "\n") {
		no := i + 1
		if strings.TrimSpace(raw) == "" {
			continue
		}
		trimmed := strings.TrimSpace(raw)
		switch {
		case trimmed == "Setup":
			cur = &Section{No: no, Kind: "setup"}
			secs = append(secs, cur)
			head, sub = nil, nil
			continue
		case trimmed == "Pokémon Checkup":
			cur = &Section{No: no, Kind: "checkup"}
			secs = append(secs, cur)
			head, sub = nil, nil
			continue
		case !strings.HasPrefix(raw, " ") && !strings.HasPrefix(raw, "-") && turnRe.MatchString(trimmed):
			cur = &Section{No: no, Kind: "turn", Player: turnRe.FindStringSubmatch(trimmed)[1]}
			secs = append(secs, cur)
			head, sub = nil, nil
			continue
		}
		if cur == nil {
			return nil, fmt.Errorf("line %d: %q before the Setup section", no, trimmed)
		}
		switch {
		case strings.HasPrefix(trimmed, "• "):
			items := splitItems(strings.TrimPrefix(trimmed, "• "))
			n := &Node{No: no, Depth: 2, Text: trimmed, Items: items}
			parent := sub
			if parent == nil {
				parent = head
			}
			if parent == nil {
				return nil, fmt.Errorf("line %d: bullet without a parent line", no)
			}
			parent.Children = append(parent.Children, n)
		case strings.HasPrefix(trimmed, "- "):
			n := &Node{No: no, Depth: 1, Text: strings.TrimPrefix(trimmed, "- ")}
			if head == nil {
				// A subline right after a section header hangs off a
				// synthetic headline so nothing is lost.
				head = &Node{No: no, Depth: 0}
				cur.Lines = append(cur.Lines, head)
			}
			head.Children = append(head.Children, n)
			sub = n
		default:
			head = &Node{No: no, Depth: 0, Text: trimmed}
			cur.Lines = append(cur.Lines, head)
			sub = nil
		}
	}
	if len(secs) == 0 || secs[0].Kind != "setup" {
		return nil, fmt.Errorf("log does not start with a Setup section")
	}
	return secs, nil
}

func splitItems(s string) []string {
	parts := strings.Split(s, ", ")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var (
	coinChoiceRe = regexp.MustCompile(`^(.+) chose (heads|tails) for the opening coin flip\.$`)
	coinWonRe    = regexp.MustCompile(`^(.+) won the coin toss\.$`)
	openingRe    = regexp.MustCompile(`^(.+) drew \d+ cards for the opening hand\.$`)
	activeRe     = regexp.MustCompile(`^(.+?) played .+ to the Active Spot\.$`)
)

// HasOpening reports whether the Setup section logs player's opening draw;
// some logs omit the opponent's.
func HasOpening(setup *Section, player string) bool {
	for _, n := range setup.Lines {
		if m := openingRe.FindStringSubmatch(n.Text); m != nil && m[1] == player {
			return true
		}
	}
	return false
}

// Players finds the two player names in the Setup section and decides which
// one is the log's owner: the player whose opening hand is itemised, unless
// me is given.
func Players(secs []*Section, me string) (string, string, error) {
	var names []string
	add := func(n string) {
		for _, x := range names {
			if x == n {
				return
			}
		}
		names = append(names, n)
	}
	itemised := ""
	for _, n := range secs[0].Lines {
		for _, re := range []*regexp.Regexp{coinChoiceRe, coinWonRe, openingRe, activeRe} {
			if m := re.FindStringSubmatch(n.Text); m != nil {
				add(m[1])
				if re == openingRe && itemised == "" {
					for _, c := range n.Children {
						if len(c.Bullets()) > 0 {
							itemised = m[1]
						}
					}
				}
			}
		}
	}
	for _, s := range secs {
		if s.Kind == "turn" {
			add(s.Player)
		}
	}
	if len(names) != 2 {
		return "", "", fmt.Errorf("expected two players in Setup, found %v", names)
	}
	if me == "" {
		me = itemised
	}
	switch me {
	case names[0]:
		return names[0], names[1], nil
	case names[1]:
		return names[1], names[0], nil
	}
	return "", "", fmt.Errorf("cannot tell which player owns the log (players %v)", names)
}
