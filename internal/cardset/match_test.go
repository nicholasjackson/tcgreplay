package cardset

import (
	"encoding/json"
	"reflect"
	"testing"
)

func malieIndexFixture(t *testing.T) map[string]MalieSetRef {
	t.Helper()
	var idx map[string]map[string]MalieSetRef
	if err := json.Unmarshal(readFixture(t, "malie/index.json"), &idx); err != nil {
		t.Fatal(err)
	}
	return idx["en-US"]
}

func tcgdexSet(t *testing.T, id, abbr string) TCGdexSet {
	t.Helper()
	b, err := json.Marshal(map[string]any{"id": id, "abbreviation": map[string]string{"official": abbr}})
	if err != nil {
		t.Fatal(err)
	}
	return decodeJSON[TCGdexSet](t, string(b))
}

func TestCodeOwners(t *testing.T) {
	sets := []TCGdexSet{
		tcgdexSet(t, "30th-c", "30C"),
		tcgdexSet(t, "30th", "30C"),
		tcgdexSet(t, "me06", "dlr"),
		tcgdexSet(t, "bb", "TIE"),
		tcgdexSet(t, "aa", "TIE"),
		tcgdexSet(t, "nocode", ""),
	}
	want := map[string]string{"30C": "30th", "DLR": "me06", "TIE": "aa"}
	if got := codeOwners(sets); !reflect.DeepEqual(got, want) {
		t.Errorf("codeOwners = %v, want %v", got, want)
	}
}

func TestMatchMalieSet(t *testing.T) {
	index := malieIndexFixture(t)
	owners := codeOwners([]TCGdexSet{
		tcgdexSet(t, "30th", "30C"),
		tcgdexSet(t, "30th-c", "30C"),
		tcgdexSet(t, "me06", "DLR"),
		tcgdexSet(t, "sv01", "SVI"),
		tcgdexSet(t, "svp", "SVP"),
	})
	tests := []struct {
		name    string
		set     TCGdexSet
		owner   map[string]string
		key     string
		matched bool
	}{
		{"abbr", tcgdexSet(t, "me06", "DLR"), owners, "me6", true},
		{"abbr ignores case", tcgdexSet(t, "me06", "dlr"), owners, "me6", true},
		{"abbr, another series", tcgdexSet(t, "sv01", "SVI"), owners, "sv1", true},
		{"code owner takes the shared code", tcgdexSet(t, "30th", "30C"), owners, "me5-5", true},
		{"longer id does not take a shared code", tcgdexSet(t, "30th-c", "30C"), owners, "", false},
		{"nil owner: the set owns its code", tcgdexSet(t, "30th-c", "30C"), nil, "me5-5", true},
		{"log code fallback (svbsp is PR-SV, TCGdex says SVP)", tcgdexSet(t, "svp", "SVP"), owners, "svbsp", true},
		{"log code fallback without a code", tcgdexSet(t, "me05.5", ""), owners, "me5-5", true},
		{"no match", tcgdexSet(t, "base1", "BS"), owners, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, ref, ok := matchMalieSet(tt.set, index, tt.owner)
			if key != tt.key || ok != tt.matched {
				t.Fatalf("matchMalieSet = %q, %v; want %q, %v", key, ok, tt.key, tt.matched)
			}
			if ok && !reflect.DeepEqual(ref, index[key]) {
				t.Errorf("ref = %+v, want %+v", ref, index[key])
			}
			if !ok && ref != (MalieSetRef{}) {
				t.Errorf("ref = %+v, want zero", ref)
			}
		})
	}
	// The ref carries the set file's path.
	if _, ref, _ := matchMalieSet(tcgdexSet(t, "me06", "DLR"), index, owners); ref.Path != "v0.1.9.13/me6.en-US.json" {
		t.Errorf("me06 path = %q", ref.Path)
	}
}

func TestSetPart(t *testing.T) {
	tests := []struct{ in, want string }{
		{"me06-001", "me06"},
		{"sv10.5b-079", "sv10.5b"},
		{"30th-c-001", "30th-c"},
		{"nodash", ""},
		{"-001", ""},
	}
	for _, tt := range tests {
		if got := setPart(tt.in); got != tt.want {
			t.Errorf("setPart(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
