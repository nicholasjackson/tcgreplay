package cards

import "testing"

// The pair mark drives behaviour only: a marked card's text is its data
// text alone, the same as the card without the mark.
func TestTextAddsNothingForPairMark(t *testing.T) {
	const want = "Twin Peak (tst-10)\nTrainer: Stadium\nYou cannot play this card by itself. You must combine 2 different Twin Peak from your hand to play as 1 Stadium card."
	c := Card{ID: "tst-10", Name: "Twin Peak", Category: "Trainer", TrainerType: "Stadium",
		Effect: "You cannot play this card by itself. You must combine 2 different Twin Peak from your hand to play as 1 Stadium card."}
	plain := c.Text()
	c.PlayedAsPair = true
	if got := c.Text(); got != plain {
		t.Errorf("marked text differs from unmarked:\n got %q\nwant %q", got, plain)
	}
	if plain != want {
		t.Errorf("Text() = %q, want %q", plain, want)
	}
}
