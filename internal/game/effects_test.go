package game

import "testing"

// Real card wording from TCGdex (/v2/en, fetched 2026-11-07), exactly as
// the card catalogue returns it, newlines included.
const (
	textJudge          = "Each player shuffles their hand into their deck and draws 4 cards."                                                                                                                                                                                // sv01-176
	textSpecialRedCard = "You can use this card only if your opponent has 3 or fewer Prize cards remaining.\n\nYour opponent shuffles their hand and puts it on the bottom of their deck. If they put any cards on the bottom of their deck in this way, they draw 3 cards." // me04-082
	textIono           = "Each player shuffles their hand and puts it on the bottom of their deck. If either player put any cards on the bottom of their deck in this way, each player draws a card for each of their remaining Prize cards."                                // sv02-185
	textUnfairStamp    = "You can use this card only if any of your Pokémon were Knocked Out during your opponent's last turn.\n\nEach player shuffles their hand into their deck. Then, you draw 5 cards, and your opponent draws 2 cards."                                 // sv06-165
	textRedCard        = "Your opponent shuffles his or her hand into his or her deck and draws 4 cards."                                                                                                                                                                    // xy1-124
	textResetStamp     = "Your opponent shuffles their hand into their deck and draws a card for each of their remaining Prize cards."                                                                                                                                       // sm11-253
	textRoxanne        = "You can use this card only if your opponent has 3 or fewer Prize cards remaining.\nEach player shuffles their hand into their deck. Then, you draw 6 cards, and your opponent draws 2 cards."                                                      // swsh12.5gg-GG66
	textMarnie         = "Each player shuffles their hand and puts it on the bottom of their deck. If either player put any cards on the bottom of their deck in this way, you draw 5 cards, and your opponent draws 4 cards."                                               // swshp-SWSH121
	textEri            = "Your opponent reveals their hand, and you discard up to 2 Item cards you find there."                                                                                                                                                              // sv05-210
	textBossOrders     = "Switch in 1 of your opponent's Benched Pokémon to the Active Spot."                                                                                                                                                                                // me01-114
	textPremiumPower   = "During this turn, attacks used by your {F} Pokémon do 30 more damage to your opponent's Active Pokémon (before applying Weakness and Resistance)."                                                                                                 // me01-124
	textPhantomDive    = "Put 6 damage counters on your opponent's Benched Pokémon in any way you like."                                                                                                                                                                     // me02.5-160 Dragapult ex
	textTerminalPeriod = "If your opponent's Active Pokémon has exactly 6 damage counters on it, that Pokémon is Knocked Out."                                                                                                                                               // me01-086 Mega Absol ex
	textClawOfDarkness = "Your opponent reveals their hand, and you discard a card you find there."                                                                                                                                                                          // me01-086 Mega Absol ex
	textUltraBall      = "You can use this card only if you discard 2 other cards from your hand. Search your deck for a Pokémon, reveal it, and put it into your hand. Then, shuffle your deck."                                                                            // tst-3
	textOldIono        = "Each player shuffles their hand and puts it on the bottom of their deck. Then, each player draws a card for each of their remaining Prize cards."                                                                                                  // tst-4 (older fixture wording)
	textEscapeRope     = "Each player switches their Active Pokémon with 1 of their Benched Pokémon."
)

func TestReadEffect(t *testing.T) {
	tests := []struct {
		name string
		text string
		want OppEffect
	}{
		// The minimum set, each with its own count and hand destination.
		{"Judge", textJudge, OppEffect{Kind: OppApply, HandTo: HandToDeck, Draw: 4}},
		{"Special Red Card", textSpecialRedCard, OppEffect{Kind: OppApply, HandTo: HandToBottom, Draw: 3, DrawIf: DrawIfOpponentPut, Condition: Condition{Kind: CondOppPrizesAtMost, N: 3}}},
		{"Iono", textIono, OppEffect{Kind: OppApply, HandTo: HandToBottom, PerPrize: true, DrawIf: DrawIfEitherPut}},
		{"Unfair Stamp", textUnfairStamp, OppEffect{Kind: OppApply, HandTo: HandToDeck, Draw: 2, Condition: Condition{Kind: CondKOLastTurn}}},

		// The rest of the family.
		{"Red Card", textRedCard, OppEffect{Kind: OppApply, HandTo: HandToDeck, Draw: 4}},
		{"Reset Stamp", textResetStamp, OppEffect{Kind: OppApply, HandTo: HandToDeck, PerPrize: true}},
		{"Roxanne", textRoxanne, OppEffect{Kind: OppApply, HandTo: HandToDeck, Draw: 2, Condition: Condition{Kind: CondOppPrizesAtMost, N: 3}}},
		{"Marnie", textMarnie, OppEffect{Kind: OppApply, HandTo: HandToBottom, Draw: 4, DrawIf: DrawIfEitherPut}},
		{"old fixture Iono", textOldIono, OppEffect{Kind: OppApply, HandTo: HandToBottom, PerPrize: true}},
		{"unchecked use condition still applies (the player's responsibility)", "You can use this card only if you have exactly 1 Benched Pokémon. " + textJudge, OppEffect{Kind: OppApply, HandTo: HandToDeck, Draw: 4, Condition: Condition{Kind: CondUnrecognised}}},
		{"number words", "Your opponent shuffles their hand into their deck and draws three cards.", OppEffect{Kind: OppApply, HandTo: HandToDeck, Draw: 3}},

		// Choices.
		{"Escape Rope-style switch", textEscapeRope, OppEffect{Kind: OppChoice}},
		{"opponent chooses", "Your opponent chooses 2 cards from their hand and discards them.", OppEffect{Kind: OppChoice}},

		// The opponent is the subject, outside the family.
		{"Eri", textEri, OppEffect{Kind: OppUnknown}},
		{"Mega Absol ex Claw of Darkness", textClawOfDarkness, OppEffect{Kind: OppUnknown}},

		// Asks nothing of the opponent.
		{"Boss's Orders", textBossOrders, OppEffect{Kind: OppNone}},
		{"Premium Power Pro", textPremiumPower, OppEffect{Kind: OppNone}},
		{"Dragapult ex Phantom Dive", textPhantomDive, OppEffect{Kind: OppNone}},
		{"Mega Absol ex Terminal Period", textTerminalPeriod, OppEffect{Kind: OppNone}},
		{"Ultra Ball", textUltraBall, OppEffect{Kind: OppNone}},
		{"empty text", "", OppEffect{Kind: OppNone}},
		{"comparison with the opponent", "If you have more Prize cards remaining than your opponent, draw 3 cards.", OppEffect{Kind: OppNone}},

		// A family card with one sentence the grammar does not accept.
		{"Judge plus an extra sentence", textJudge + " Discard a card from your hand.", OppEffect{Kind: OppUnknown}},
		{"Iono plus an extra sentence", textIono + " Your opponent reveals their hand.", OppEffect{Kind: OppUnknown}},
		{"disposal with no opponent draw", "Your opponent shuffles their hand into their deck. Then, you draw 5 cards.", OppEffect{Kind: OppUnknown}},
		{"two opponent draws", textJudge + " Then, your opponent draws 2 cards.", OppEffect{Kind: OppUnknown}},
		{"player-only disposal", "Shuffle your hand into your deck. Then, draw 6 cards.", OppEffect{Kind: OppNone}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadEffect(tt.text); got != tt.want {
				t.Errorf("ReadEffect(%q)\n got  %+v (%s)\n want %+v (%s)", tt.text, got, got.Kind, tt.want, tt.want.Kind)
			}
		})
	}
}

func TestNamesOpponent(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"Boss's Orders", textBossOrders, true},
		{"Premium Power Pro", textPremiumPower, true},
		{"Judge", textJudge, true},
		{"Escape Rope-style switch", textEscapeRope, true},
		{"Red Card", textRedCard, true},
		{"Ultra Ball", textUltraBall, false},
		{"empty text", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NamesOpponent(tt.text); got != tt.want {
				t.Errorf("NamesOpponent(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
