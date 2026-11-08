package cardset

import "testing"

func TestCleanText(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{
			"energy span keeps its word",
			`This attack does 30 damage for each Basic <span class="energy-symbol Fire" title="Fire">Fire</span> Energy card in your discard pile.`,
			"This attack does 30 damage for each Basic Fire Energy card in your discard pile.",
		},
		{
			"emphasis stripped",
			"This attack also does 120 damage to 1 of your opponent's Benched Pokémon. <em>(Don't apply Weakness and Resistance for Benched Pokémon.)</em>",
			"This attack also does 120 damage to 1 of your opponent's Benched Pokémon. (Don't apply Weakness and Resistance for Benched Pokémon.)",
		},
		{"paragraph stripped", "<p>Search your deck for an Energy card.</p>", "Search your deck for an Energy card."},
		{"energy symbols pass through", "Attach a Basic {R} Energy and a {C} Energy.", "Attach a Basic {R} Energy and a {C} Energy."},
		{"entities unescaped", "Pok&eacute;mon &amp; Trainer &quot;cards&quot; &#39;here&#39;", `Pokémon & Trainer "cards" 'here'`},
		{"br becomes a newline", "Line one.<br>Line two.<br/>Line three.<BR />Line four.", "Line one.\nLine two.\nLine three.\nLine four."},
		{"malie blank line with a space", "Provides {W} Energy.\n \nThe {W} Pokémon recovers.", "Provides {W} Energy.\n\nThe {W} Pokémon recovers."},
		{"runs of spaces collapse and lines trim", "  Draw   2\t cards.  \n  Then shuffle. ", "Draw 2 cards.\nThen shuffle."},
		{"non-breaking spaces collapse", "Draw&nbsp;&nbsp;2 cards.", "Draw 2 cards."},
		{"at most one blank line", "A.\n\n\n\nB.", "A.\n\nB."},
		{"carriage returns", "A.\r\nB.\rC.", "A.\nB.\nC."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanText(tt.in); got != tt.want {
				t.Errorf("cleanText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCompareKey(t *testing.T) {
	if got, want := compareKey("Attach {G} and {C}  to\nthe opponent’s Pokémon."), "Attach Grass and Colorless to the opponent's Pokémon."; got != want {
		t.Errorf("compareKey = %q, want %q", got, want)
	}

	tests := []struct {
		name  string
		a, b  string
		equal bool
	}{
		{"spacing", "Draw  2 cards.\nThen  shuffle.", "Draw 2 cards. Then shuffle.", true},
		{"apostrophe style", "your opponent’s Active Pokémon", "your opponent's Active Pokémon", true},
		{"energy symbol vs word", "each Basic {R} Energy card", "each Basic Fire Energy card", true},
		{"energy span vs symbol", `each Basic <span class="energy-symbol Darkness" title="Darkness">Darkness</span> Energy card`, "each Basic {D} Energy card", true},
		{"malie blank line vs one newline", "A.\n \nB.", "A. B.", true},
		{"different words", "Draw 2 cards.", "Draw 3 cards.", false},
		{"different energy", "each Basic {R} Energy card", "each Basic Water Energy card", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ka, kb := compareKey(tt.a), compareKey(tt.b)
			if (ka == kb) != tt.equal {
				t.Errorf("compareKey(%q) = %q, compareKey(%q) = %q; equal = %v, want %v", tt.a, ka, tt.b, kb, ka == kb, tt.equal)
			}
		})
	}
}

func TestNameKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Basic {G} Energy", "Grass Energy"},
		{"Grass Energy", "Grass Energy"},
		{"Bubbly {W} Energy", "Bubbly Water Energy"},
		{"Basic Pokémon", "Basic Pokémon"}, // not an Energy: "Basic" stays
		{"Professor’s Research", "Professor's Research"},
	}
	for _, tt := range tests {
		if got := nameKey(tt.in); got != tt.want {
			t.Errorf("nameKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if nameKey("Basic {G} Energy") != nameKey("Grass Energy") {
		t.Error(`"Basic {G} Energy" and "Grass Energy" should compare equal as names`)
	}
}

func TestHasMarkup(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"<em>(Don't apply Weakness.)</em>", true},
		{`Basic <span class="energy-symbol Fire" title="Fire">Fire</span> Energy`, true},
		{"Line<br>break", true},
		{"Attach a {R} Energy.", false},
		{"Plain words.", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := hasMarkup(tt.in); got != tt.want {
			t.Errorf("hasMarkup(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
