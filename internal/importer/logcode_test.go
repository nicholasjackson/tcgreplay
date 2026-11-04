package importer

import (
	"slices"
	"testing"
)

func TestNormalizeStripsLogCodes(t *testing.T) {
	in := "NicJackson's (me1_74) Lunatone used Lunar Cycle.\n" +
		"   • (me2_85_ph) Battle Cage, (zsv10-5_79) Air Balloon, (me2-5_172) Mega Audino ex\n" +
		"Damage to (me3_121) Meowth ex was prevented. (not a code) stays"
	want := "NicJackson's Lunatone used Lunar Cycle.\n" +
		"   • Battle Cage, Air Balloon, Mega Audino ex\n" +
		"Damage to Meowth ex was prevented. (not a code) stays"
	if got := Normalize(in); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestLogCodes(t *testing.T) {
	log := "played (sv6_95) Munkidori, (sv6_95) Munkidori ex? (me1_86) Mega Absol ex and (x_1) Munkidori"
	if got := LogCodes(log, "Munkidori"); !slices.Equal(got, []string{"sv6_95", "x_1"}) {
		t.Errorf("got %v", got)
	}
	if got := LogCodes(log, "Absol ex"); got != nil {
		t.Errorf("a name's tail matched: %v", got)
	}
}
