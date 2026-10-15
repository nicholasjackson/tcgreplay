package deck

import (
	"context"
	"os"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// TestLiveResolve resolves the real test deck against TCGdex. Run with
// TCGREPLAY_LIVE=1; it caches under the user cache dir.
func TestLiveResolve(t *testing.T) {
	if os.Getenv("TCGREPLAY_LIVE") == "" {
		t.Skip("set TCGREPLAY_LIVE=1 to hit TCGdex")
	}
	b, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Fatal(err)
	}
	l, err := Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	db := cards.NewClient(cards.DefaultCacheDir())
	if err := l.Resolve(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for _, e := range l.Entries {
		c, err := db.Card(context.Background(), e.Def)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%d %s %s %s -> %s (%s)", e.Count, e.Name, e.Set, e.Number, e.Def, c.Name)
		if c.Name != e.Name {
			t.Errorf("%s resolved to %s", e.Name, c.Name)
		}
	}
}
