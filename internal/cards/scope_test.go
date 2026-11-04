package cards

import "testing"

func TestScopeKey(t *testing.T) {
	cases := []struct {
		set, key string
		want     bool
	}{
		{"", "sets", true},
		{"", "sets/me01", true},
		{"", "cards/sv1-001", true},
		{"me01", "sets/me01", true},
		{"me01", "cards/me01-001", true},
		{"me01", "cards/me01-TG05", true},
		{"me01", "sets", false},
		{"me01", "sets/me01.5", false},
		{"me01", "cards/me01.5-001", false},
		{"me01", "cards/me010-001", false},
		{"me01", "cards/xme01-001", false},
		{"sv03", "cards/sv03-001", true},
		{"sv03", "sets/sv03.5", false},
		{"sv03", "cards/sv03.5-001", false},
		{"sv03.5", "cards/sv03.5-001", true},
		{"sv03.5", "cards/sv03-001", false},
		{"swsh12", "cards/swsh12-001", true},
		{"swsh12", "cards/swsh12tg-TG01", false},
		{"swsh12", "cards/swsh12.5-157", false},
		{"swsh12", "sets/swsh12tg", false},
	}
	for _, c := range cases {
		if got := (Scope{Set: c.set}).Key(c.key); got != c.want {
			t.Errorf("Scope{%q}.Key(%q) = %v, want %v", c.set, c.key, got, c.want)
		}
	}
}

func TestScopeImage(t *testing.T) {
	cases := []struct {
		set, rel string
		want     bool
	}{
		{"", "en/me/me01/001", true},
		{"", "anything", true},
		{"me01", "en/me/me01/001", true},
		{"me01", "ja/me/me01/001", true},
		{"me01", "en/me/me01.5/001", false},
		{"me01", "en/swsh/swsh12.5/157", false},
		{"swsh12.5", "en/swsh/swsh12.5/157", true},
		{"sv03", "en/sv/sv03.5/001", false},
		{"me01", "me01", false},
		{"me01", "en/me01", false},
		{"me01", "me01/me01/x/001", false},
		{"me01", "", false},
	}
	for _, c := range cases {
		if got := (Scope{Set: c.set}).Image(c.rel); got != c.want {
			t.Errorf("Scope{%q}.Image(%q) = %v, want %v", c.set, c.rel, got, c.want)
		}
	}
}

func TestScopeString(t *testing.T) {
	if s := (Scope{}).String(); s != "everything" {
		t.Errorf("all = %q", s)
	}
	if s := (Scope{Set: "me01"}).String(); s != "set me01" {
		t.Errorf("set = %q", s)
	}
}
