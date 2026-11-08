package cards

import "testing"

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
