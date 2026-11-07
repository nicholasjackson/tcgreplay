package game

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// OppKind is what an effect text asks of the opponent.
type OppKind int

const (
	// OppNone: the opponent is never the subject of an action. Naming the
	// opponent's Pokémon as a target ("your opponent's Active Pokémon")
	// still counts, because the player resolves it on the board.
	OppNone OppKind = iota
	// OppApply: the whole text is the shuffle-your-hand-and-draw family,
	// which the playmat can carry out on the opponent's side itself.
	OppApply
	// OppChoice: the opponent (or each player) chooses something.
	OppChoice
	// OppUnknown: the opponent is the subject of anything else.
	OppUnknown
)

func (k OppKind) String() string {
	switch k {
	case OppApply:
		return "apply"
	case OppChoice:
		return "choice"
	case OppUnknown:
		return "unknown"
	}
	return "none"
}

// Hand destinations of an OppApply effect.
const (
	HandToDeck   = "deck"   // shuffled into the deck
	HandToBottom = "bottom" // put on the bottom of the deck, in a random order
)

// Draw conditions of an OppApply effect: draw only if cards went to the
// bottom of a deck.
const (
	DrawIfOpponentPut = "opponent_put" // the opponent put any cards there
	DrawIfEitherPut   = "either_put"   // either player put any cards there
)

// Use conditions ("You can use this card only if …") the reader recognises.
const (
	CondKOLastTurn      = "ko_last_turn"       // any of your Pokémon were Knocked Out during your opponent's last turn
	CondOppPrizesAtMost = "opp_prizes_at_most" // your opponent has N or fewer Prize cards remaining
	CondUnrecognised    = "unrecognised"       // a condition the playmat cannot check
)

// OppEffect is what an effect text asks of the opponent. The fields after
// Kind are set only for OppApply.
type OppEffect struct {
	Kind      OppKind
	HandTo    string    // HandToDeck | HandToBottom
	Draw      int       // fixed number of cards the opponent draws
	PerPrize  bool      // draw one card per remaining Prize card instead
	DrawIf    string    // "" | DrawIfOpponentPut | DrawIfEitherPut
	Condition Condition // the card's use condition, when it has one
}

// Condition is a card's "You can use this card only if …" sentence.
type Condition struct {
	Kind string // "" | CondKOLastTurn | CondOppPrizesAtMost | CondUnrecognised
	N    int    // for CondOppPrizesAtMost
}

// The family grammar. Every sentence of an OppApply text must match one of
// these shapes; each pattern sits beside the card wording it was written
// for (TCGdex English text, normalised: lower case, "his or her" read as
// "their", one space between words, no closing full stop).
const countRe = `(a card for each of their remaining prize cards|(?:\d+|a|an|one|two|three|four|five|six|seven|eight|nine|ten) cards?)`

var (
	// Unfair Stamp sv06-165, Special Red Card me04-082, Roxanne
	// swsh12.5gg-GG66: "you can use this card only if …".
	reCondition = regexp.MustCompile(`^you can use this card only if (.+)$`)
	// Unfair Stamp sv06-165.
	reCondKO = regexp.MustCompile(`^any of your pok[eé]mon were knocked out during your opponent's last turn$`)
	// Special Red Card me04-082, Roxanne swsh12.5gg-GG66.
	reCondPrizes = regexp.MustCompile(`^your opponent has (\d+|one|two|three|four|five|six) or fewer prize cards remaining$`)

	// Judge sv01-176 "each player shuffles their hand into their deck and
	// draws 4 cards"; Red Card xy1-124 "your opponent shuffles his or her
	// hand into his or her deck and draws 4 cards"; Reset Stamp sm11-253
	// "… and draws a card for each of their remaining prize cards"; Unfair
	// Stamp and Roxanne "each player shuffles their hand into their deck".
	reDispDeck = regexp.MustCompile(`^(each player|your opponent) shuffles their hand into their deck(?: and draws ` + countRe + `)?$`)
	// Iono sv02-185, Marnie swshp-SWSH121 "each player shuffles their hand
	// and puts it on the bottom of their deck"; Special Red Card "your
	// opponent shuffles their hand and puts it on the bottom of their deck".
	reDispBottom = regexp.MustCompile(`^(each player|your opponent) shuffles their hand and puts it on the bottom of their deck(?: and draws ` + countRe + `)?$`)

	// The lead-ins of a draw sentence: Unfair Stamp "then, …"; Iono and
	// Marnie "if either player put any cards on the bottom of their deck in
	// this way, …"; Special Red Card "if they put any cards …".
	reDrawLead = regexp.MustCompile(`^(?:then, )?(?:if (either player|they|your opponent) put any cards on the bottom of their deck in this way, )?(.+)$`)
	// Unfair Stamp "you draw 5 cards, and your opponent draws 2 cards";
	// Roxanne "you draw 6 cards, and your opponent draws 2 cards"; Marnie
	// "you draw 5 cards, and your opponent draws 4 cards".
	reDrawBoth = regexp.MustCompile(`^you draw ` + countRe + `,? and your opponent draws ` + countRe + `$`)
	// Iono "each player draws a card for each of their remaining prize
	// cards"; Special Red Card "they draw 3 cards"; and the plain forms.
	reDrawSubject = regexp.MustCompile(`^(each player draws|your opponent draws|they draw|you draw) ` + countRe + `$`)
)

// Words that make a sentence with the opponent as its subject a choice:
// "each player switches their Active Pokémon with 1 of their Benched
// Pokémon" (Escape Rope), "your opponent chooses …".
var choiceWords = []string{"choose", "chooses", "of their choice", "switches their active", "switch their active"}

var numberWords = map[string]int{"a": 1, "an": 1, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10}

// ReadEffect reads an effect text and says what it asks of the opponent.
// It knows no card names: the shuffle-your-hand-and-draw family is
// recognised by its wording, and a text is OppApply only when every one of
// its sentences is understood. Empty text reads as OppNone; callers treat
// an unknown card (no definition) as unknown themselves. Card text is
// always English (TCGdex /v2/en), so the reader handles English only.
func ReadEffect(text string) OppEffect {
	sentences := splitSentences(normaliseEffect(text))
	if len(sentences) == 0 {
		return OppEffect{Kind: OppNone}
	}
	if eff, ok := readFamily(sentences); ok {
		return eff
	}
	kind := OppNone
	for _, s := range sentences {
		if !opponentIsSubject(s) {
			continue
		}
		if containsAny(s, choiceWords) {
			return OppEffect{Kind: OppChoice}
		}
		kind = OppUnknown
	}
	return OppEffect{Kind: kind}
}

// NamesOpponent reports whether a text names the opponent at all, even
// only as a target. The board's decision hold uses it.
func NamesOpponent(text string) bool {
	t := normaliseEffect(text)
	return strings.Contains(t, "opponent") || strings.Contains(t, "each player") || strings.Contains(t, "both players")
}

// readFamily parses sentences as the family grammar: an optional
// condition first, then one hand disposal naming the opponent, then draw
// sentences. The opponent must end with exactly one draw.
func readFamily(sentences []string) (OppEffect, bool) {
	eff := OppEffect{Kind: OppApply}
	i := 0
	if m := reCondition.FindStringSubmatch(sentences[0]); m != nil {
		eff.Condition = readCondition(m[1])
		i++
	}
	if i >= len(sentences) {
		return OppEffect{}, false
	}
	var subject, count string
	if m := reDispDeck.FindStringSubmatch(sentences[i]); m != nil {
		eff.HandTo, subject, count = HandToDeck, m[1], m[2]
	} else if m := reDispBottom.FindStringSubmatch(sentences[i]); m != nil {
		eff.HandTo, subject, count = HandToBottom, m[1], m[2]
	} else {
		return OppEffect{}, false
	}
	i++
	draws := 0
	if count != "" {
		if !setCount(&eff, count) {
			return OppEffect{}, false
		}
		draws++
	}
	for ; i < len(sentences); i++ {
		m := reDrawLead.FindStringSubmatch(sentences[i])
		if m == nil {
			return OppEffect{}, false
		}
		drawIf, body := m[1], m[2]
		oppCount := ""
		if b := reDrawBoth.FindStringSubmatch(body); b != nil {
			oppCount = b[2]
		} else if b := reDrawSubject.FindStringSubmatch(body); b != nil {
			switch b[1] {
			case "each player draws", "your opponent draws", "they draw":
				// "they" is the disposal's subject, which always includes
				// the opponent.
				oppCount = b[2]
			case "you draw":
				// The player's own draw: theirs to resolve on the board.
			}
		} else {
			return OppEffect{}, false
		}
		if oppCount == "" {
			continue
		}
		if draws > 0 || !setCount(&eff, oppCount) {
			return OppEffect{}, false
		}
		draws++
		switch drawIf {
		case "either player":
			eff.DrawIf = DrawIfEitherPut
		case "they", "your opponent":
			eff.DrawIf = DrawIfOpponentPut
		}
	}
	if subject == "" || draws != 1 {
		return OppEffect{}, false
	}
	return eff, true
}

func readCondition(c string) Condition {
	if reCondKO.MatchString(c) {
		return Condition{Kind: CondKOLastTurn}
	}
	if m := reCondPrizes.FindStringSubmatch(c); m != nil {
		if n, ok := parseNumber(m[1]); ok {
			return Condition{Kind: CondOppPrizesAtMost, N: n}
		}
	}
	return Condition{Kind: CondUnrecognised}
}

// setCount sets the opponent's draw from a count phrase.
func setCount(eff *OppEffect, count string) bool {
	if count == "a card for each of their remaining prize cards" {
		eff.PerPrize = true
		return true
	}
	n, ok := parseNumber(strings.Fields(count)[0])
	if !ok {
		return false
	}
	eff.Draw = n
	return true
}

func parseNumber(w string) (int, bool) {
	if n, err := strconv.Atoi(w); err == nil {
		return n, true
	}
	n, ok := numberWords[w]
	return n, ok
}

// normaliseEffect lower-cases a text, reads "his or her" as "their" and
// collapses whitespace (newlines included) to single spaces.
func normaliseEffect(text string) string {
	t := strings.ToLower(text)
	t = strings.ReplaceAll(t, "’", "'")
	t = strings.ReplaceAll(t, "his or her", "their")
	return strings.Join(strings.Fields(t), " ")
}

var reSentenceEnd = regexp.MustCompile(`[.!]\s+`)

// splitSentences splits a normalised text into sentences without their
// closing full stops.
func splitSentences(t string) []string {
	var out []string
	for _, s := range reSentenceEnd.Split(t, -1) {
		s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), ".!"))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

var reOpponentSubject = regexp.MustCompile(`(your opponent|each player|both players)`)

// opponentIsSubject reports whether a sentence has the opponent (or each
// player) as the subject of something, rather than only naming the
// opponent's things ("your opponent's Active Pokémon") or comparing with
// them ("more Prize cards than your opponent").
func opponentIsSubject(s string) bool {
	for _, loc := range reOpponentSubject.FindAllStringIndex(s, -1) {
		if strings.HasPrefix(s[loc[1]:], "'s") {
			continue
		}
		if strings.HasSuffix(s[:loc[0]], "than ") {
			continue
		}
		return true
	}
	return false
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// OpenerNamesOpponent reports whether the card a board request would open
// with names the opponent at all: the Trainer iid, or, for verb
// "use_ability", the Ability named ability of the Pokémon whose top card
// is iid. An unknown card, or an Ability not found, counts as naming the
// opponent, the safe side for the board's decision hold.
func (g *Game) OpenerNamesOpponent(verb string, iid state.IID, ability string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	c := g.card(g.st, iid)
	if c == nil {
		return true
	}
	if verb != "use_ability" {
		return NamesOpponent(c.Effect)
	}
	for _, a := range c.Abilities {
		if strings.EqualFold(a.Name, ability) {
			return NamesOpponent(a.Effect)
		}
	}
	return true
}
