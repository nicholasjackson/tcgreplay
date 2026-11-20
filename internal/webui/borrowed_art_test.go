package webui

import (
	"context"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// artFirstDB is the fake card pool with the real database's ByName
// preference: a print with art, among those with the asked moves.
type artFirstDB struct {
	cardstest.FakeDB
	asked [][]string
}

func (d *artFirstDB) ByName(_ context.Context, name string, moves ...string) (string, error) {
	d.asked = append(d.asked, moves)
	best := ""
	for id, c := range d.FakeDB {
		if c.Name != name || (len(moves) > 0 && !slices.ContainsFunc(c.Attacks, func(a cards.Attack) bool { return a.Name == moves[0] })) {
			continue
		}
		if best == "" || (c.Image != "" && d.FakeDB[best].Image == "") {
			best = id
		}
	}
	if best == "" {
		return "", cards.ErrNotFound
	}
	return best, nil
}

// TestCardBorrowsArtWhenItHasNone: a card TCGdex has no image for (a new
// promo) shows the art of the same Pokémon's preferred print, keeping its
// own identity; one with no such print, or Energy, borrows nothing.
func TestCardBorrowsArtWhenItHasNone(t *testing.T) {
	const art = "https://assets.tcgdex.net/en/me/me01/188"
	db := &artFirstDB{FakeDB: cardstest.New()}
	// tst-2 is a Lucario without art; tst-20 the same Pokémon with art; a
	// different Pokémon of the same name (other attack) has art too.
	db.FakeDB["tst-20"] = &cards.Card{ID: "tst-20", Name: "Lucario", Category: "Pokemon", HP: 130, Attacks: []cards.Attack{{Name: "Aura Sphere", Damage: "130"}}, Image: art}
	db.FakeDB["tst-21"] = &cards.Card{ID: "tst-21", Name: "Lucario", Category: "Pokemon", HP: 120, Attacks: []cards.Attack{{Name: "Other Move"}}, Image: "https://assets.tcgdex.net/en/other"}
	m := game.NewManager(t.TempDir(), game.Static{DB: db})
	s := New(m, state.P1)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	got := cardJSON(t, ts, "tst-2")
	if firstImage(t, got)["image"] != art {
		t.Fatalf("imageless Lucario image = %v, want the same Pokémon's art %s", firstImage(t, got)["image"], art)
	}
	if got["id"] != "tst-2" || got["hp"] != float64(130) {
		t.Errorf("borrowing art changed the card: id %v, hp %v", got["id"], got["hp"])
	}
	if !slices.ContainsFunc(db.asked, func(m []string) bool { return slices.Equal(m, []string{"Aura Sphere"}) }) {
		t.Errorf("ByName asked with moves %v, want [Aura Sphere]", db.asked)
	}

	// A card with art keeps its own.
	if got := cardJSON(t, ts, "tst-20"); firstImage(t, got)["image"] != art {
		t.Errorf("Lucario with art: image %v, want %s", firstImage(t, got)["image"], art)
	}
	// Nothing to borrow from: still no image.
	if _, ok := firstImage(t, cardJSON(t, ts, "tst-7"))["image"]; ok {
		t.Error("Makuhita, with no print that has art, got an image")
	}
	// The whole image list is borrowed, in order.
	malie := "https://cdn.malie.io/x/me1_en_188_std.jpg"
	db.FakeDB["tst-20"].Images = []cards.ImageRef{{URL: art}, {URL: malie, File: true}}
	imgs := cardImages(t, cardJSON(t, ts, "tst-2"))
	if len(imgs) != 2 || imgs[0]["image"] != art || imgs[1]["image"] != malie || imgs[1]["image_file"] != true {
		t.Errorf("borrowed list %v, want the whole list of tst-20", imgs)
	}
	// Energy is left to the card database's own borrowing.
	if _, ok := firstImage(t, cardJSON(t, ts, "sve-006"))["image"]; ok {
		t.Error("Energy borrowed art in the board server")
	}
}
