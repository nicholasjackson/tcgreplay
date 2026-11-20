package cards

import (
	"testing"
)

func TestLogCodeID(t *testing.T) {
	for code, want := range map[string]string{
		"me1_86":       "me01-086",
		"me3_121":      "me03-121",
		"me2-5_172":    "me02.5-172",
		"me2_85_ph":    "me02-085",
		"mebsp_33":     "mep-033",
		"mee_14":       "mee-014",
		"sv5_154":      "sv05-154",
		"sv10_226":     "sv10-226",
		"zsv10-5_79":   "sv10.5b-079",
		"rsv10-5_1":    "sv10.5w-001",
		"swsh1_213":    "swsh1-213",
		"swsh12-5_157": "swsh12.5-157",
	} {
		if got, ok := LogCodeID(code); !ok || got != want {
			t.Errorf("%s: got %q, want %q", code, got, want)
		}
	}
	if _, ok := LogCodeID("Mega Absol ex"); ok {
		t.Error("a name is not a code")
	}
}
