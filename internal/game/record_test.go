package game

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Characters note apps substitute, built from code points so the source
// stays ASCII.
var (
	leftDouble  = string(rune(0x201C))
	rightDouble = string(rune(0x201D))
	rightSingle = string(rune(0x2019))
	nbsp        = string(rune(0x00A0))
	bom         = string(rune(0xFEFF))
)

const (
	fixtureLog = "Setup\n" +
		"nicj's Riolu is now in the Active Spot.\n" +
		"Opponent drew  7 cards.\n" +
		"nicj said \"go\" and played Ultra Ball.\n" +
		"Turn # 1 - nicj's Turn"
	fixtureDeck    = "Pokemon: 2\n4 Riolu SVI 113\n3 Lucario SVI 114\n\nEnergy: 1\n8 Fighting Energy SVE 6"
	fixtureOppDeck = "Pokemon: 1\n4 Abra MEW 63"
	fixtureMe      = "nicj"
	fixtureSeed    = uint64(424242)
)

// sealedFixture is the record every test starts from.
func sealedFixture() Record {
	return Record{Me: fixtureMe, Seed: fixtureSeed, Deck: fixtureDeck, OppDeck: fixtureOppDeck, Log: fixtureLog}.Seal()
}

// curlStructural turns every unescaped double quote in JSON text into
// alternating left and right curly quotes, as a note app does.
func curlStructural(text string) string {
	var b strings.Builder
	left := true
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '"' && (i == 0 || text[i-1] != '\\') {
			if left {
				b.WriteString(leftDouble)
			} else {
				b.WriteString(rightDouble)
			}
			left = !left
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// curlAll turns every double quote, escaped or not, into alternating curly
// quotes.
func curlAll(text string) string {
	var b strings.Builder
	left := true
	for _, c := range text {
		if c == '"' {
			if left {
				b.WriteString(leftDouble)
			} else {
				b.WriteString(rightDouble)
			}
			left = !left
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

// TestRecordReformatted accepts a record re-indented or compacted
// unchanged.
func TestRecordReformatted(t *testing.T) {
	want := sealedFixture()
	text := want.JSON()
	var tabbed, compact bytes.Buffer
	if err := json.Indent(&tabbed, []byte(text), "", "\t"); err != nil {
		t.Fatal(err)
	}
	if err := json.Compact(&compact, []byte(text)); err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]string{
		"as written":       text,
		"tab indented":     tabbed.String(),
		"compacted":        compact.String(),
		"surrounding ws":   "\n\n  " + text + "\n\n",
		"prefixed indent":  strings.ReplaceAll(text, "\n", "\n    "),
		"markdown fence":   "```json\n" + text + "\n```",
		"bare fence":       "```\n" + text + "\n```",
		"byte order mark":  bom + text,
		"bom and fence":    bom + "```json\n" + text + "\n```\n",
		"compact in fence": "```json\n" + compact.String() + "\n```",
	} {
		t.Run(name, func(t *testing.T) {
			got, isRecord, err := ParseRecord(in)
			if err != nil || !isRecord {
				t.Fatalf("ParseRecord = %v, %v, want record and no error", isRecord, err)
			}
			if got != want {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}

// TestRecordNoteAppDamage accepts a record altered only by the
// substitutions note apps make, and returns the original sealed record.
func TestRecordNoteAppDamage(t *testing.T) {
	want := sealedFixture()
	text := want.JSON()

	crlfRec := want
	crlfRec.Log = strings.ReplaceAll(want.Log, "\n", "\r\n")
	crlfDeck := want
	crlfDeck.Log = crlfRec.Log
	crlfDeck.Deck = strings.ReplaceAll(want.Deck, "\n", "\r\n")

	// A single space inside the log value, not the JSON indentation.
	logSpace := strings.Replace(text, "Riolu is now", "Riolu"+nbsp+"is now", 1)

	cases := map[string]string{
		"curly structural quotes":   curlStructural(text),
		"curly quotes everywhere":   curlAll(text),
		"nbsp in log value":         logSpace,
		"nbsp in indentation":       strings.ReplaceAll(text, "\n  ", "\n"+nbsp+nbsp),
		"windows line ends in log":  crlfRec.JSON(),
		"windows line ends in deck": crlfDeck.JSON(),
		"curly apostrophes":         strings.ReplaceAll(text, "'", rightSingle),
		"every substitution":        strings.ReplaceAll(curlAll(strings.Replace(crlfRec.JSON(), "Riolu is now", "Riolu"+nbsp+"is now", 1)), "'", rightSingle),
		"curly quotes in fence":     "```json\n" + curlStructural(text) + "\n```",
	}
	// Every space in the log value turned to a non-breaking space.
	nbspLog := want
	nbspLog.Log = strings.ReplaceAll(want.Log, " ", nbsp)
	cases["nbsp in every log space"] = nbspLog.JSON()

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if in == text {
				t.Fatal("fixture damage did not change the text")
			}
			got, isRecord, err := ParseRecord(in)
			if err != nil || !isRecord {
				t.Fatalf("ParseRecord = %v, %v, want record and no error", isRecord, err)
			}
			if got != want {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}

// jsonInner is s encoded as a JSON string without its surrounding quotes.
func jsonInner(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b[1 : len(b)-1])
}

// TestRecordCurlyContentKept accepts a record whose own content holds
// curly quotes and non-breaking spaces, without "repairing" them away.
func TestRecordCurlyContentKept(t *testing.T) {
	want := Record{Me: fixtureMe, Seed: fixtureSeed, Deck: fixtureDeck, Log: "nicj" + rightSingle + "s turn:" + nbsp + leftDouble + "go" + rightDouble}.Seal()
	got, isRecord, err := ParseRecord(want.JSON())
	if err != nil || !isRecord {
		t.Fatalf("ParseRecord = %v, %v, want record and no error", isRecord, err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestRecordDamaged refuses any other change to a record's content.
func TestRecordDamaged(t *testing.T) {
	orig := sealedFixture()
	text := orig.JSON()
	edit := func(f func(r *Record)) string {
		r := orig
		f(&r)
		return r.JSON()
	}
	altered := []byte(orig.Check)
	if altered[len(altered)-1] == '0' {
		altered[len(altered)-1] = '1'
	} else {
		altered[len(altered)-1] = '0'
	}
	checkLine := `,` + "\n" + `  "check": "` + orig.Check + `"`
	if !strings.Contains(text, checkLine) {
		t.Fatalf("fixture JSON has no check line %q:\n%s", checkLine, text)
	}

	for name, in := range map[string]string{
		"double space collapsed": edit(func(r *Record) { r.Log = strings.Replace(r.Log, "drew  7", "drew 7", 1) }),
		"deck count edited":      edit(func(r *Record) { r.Deck = strings.Replace(r.Deck, "4 Riolu", "3 Riolu", 1) }),
		"deck count in text":     strings.Replace(text, `4 Riolu`, `3 Riolu`, 1),
		"opp deck edited":        edit(func(r *Record) { r.OppDeck = "" }),
		"me edited":              edit(func(r *Record) { r.Me = "someone" }),
		"log line dropped":       edit(func(r *Record) { r.Log = strings.Replace(r.Log, "Setup\n", "", 1) }),
		"seed edited":            edit(func(r *Record) { r.Seed++ }),
		"seed in text":           strings.Replace(text, `"seed": "424242"`, `"seed": "424243"`, 1),
		"check emptied":          edit(func(r *Record) { r.Check = "" }),
		"check removed":          strings.Replace(text, checkLine, "", 1),
		"check altered":          edit(func(r *Record) { r.Check = string(altered) }),
		"version zero":           strings.Replace(text, `"tcgreplay_record": 1`, `"tcgreplay_record": 0`, 1),
		"truncated json":         text[:len(text)/2],
		"not json":               `{"tcgreplay_record": 1, oops}`,
	} {
		t.Run(name, func(t *testing.T) {
			if in == text {
				t.Fatal("fixture edit did not change the text")
			}
			got, isRecord, err := ParseRecord(in)
			if !isRecord {
				t.Fatal("isRecord = false, want true")
			}
			if !errors.Is(err, ErrRecordDamaged) {
				t.Errorf("err = %v, want ErrRecordDamaged", err)
			}
			if got != (Record{}) {
				t.Errorf("got %+v, want the zero record", got)
			}
		})
	}
}

// TestRecordNewerVersion refuses a record from a newer format, naming its
// version rather than calling it damaged.
func TestRecordNewerVersion(t *testing.T) {
	r := Record{Version: 2, Me: fixtureMe, Seed: fixtureSeed, Deck: fixtureDeck, Log: fixtureLog}.Seal()
	text := r.JSON()
	if !strings.Contains(text, `"tcgreplay_record": 2`) {
		t.Fatalf("JSON lacks version 2 marker:\n%s", text)
	}
	got, isRecord, err := ParseRecord(text)
	if !isRecord {
		t.Fatal("isRecord = false, want true")
	}
	if err == nil {
		t.Fatal("err = nil, want a version error")
	}
	if errors.Is(err, ErrRecordDamaged) {
		t.Errorf("err = %v, should not be ErrRecordDamaged", err)
	}
	if !strings.Contains(err.Error(), "version 2") {
		t.Errorf("err = %q, want it to name version 2", err)
	}
	if got != (Record{}) {
		t.Errorf("got %+v, want the zero record", got)
	}
}

// TestRecordNotARecord treats battle logs and other text as not a record.
func TestRecordNotARecord(t *testing.T) {
	inputs := map[string]string{
		"empty":         "",
		"plain log":     "Setup\nnicj chose heads for the opening coin flip.\n",
		"fenced log":    "```\nSetup\nTurn # 1\n```",
		"other json":    `{"me": "nicj", "log": "Setup"}`,
		"json array":    `[{"tcgreplay_record": 1}]`,
		"marker in log": "Setup\n\"tcgreplay_record\": 1\n",
	}
	logs, _ := filepath.Glob("../../testdata/logs/*.txt")
	if len(logs) == 0 {
		t.Log("no test corpus; checking hand-written inputs only")
	}
	for _, path := range logs {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		inputs["corpus/"+filepath.Base(path)] = string(text)
	}
	for name, in := range inputs {
		t.Run(name, func(t *testing.T) {
			if LooksLikeRecord(in) {
				t.Error("LooksLikeRecord = true, want false")
			}
			got, isRecord, err := ParseRecord(in)
			if isRecord || err != nil {
				t.Errorf("ParseRecord = %v, %v, want false and no error", isRecord, err)
			}
			if got != (Record{}) {
				t.Errorf("got %+v, want the zero record", got)
			}
		})
	}
}

// TestRecordLooksLike recognises records however they were wrapped.
func TestRecordLooksLike(t *testing.T) {
	text := sealedFixture().JSON()
	for name, in := range map[string]string{
		"plain":         text,
		"fenced":        "```json\n" + text + "\n```",
		"bom":           bom + text,
		"curly quotes":  curlStructural(text),
		"leading space": "\n  " + text,
	} {
		if !LooksLikeRecord(in) {
			t.Errorf("%s: LooksLikeRecord = false, want true", name)
		}
	}
}

// TestRecordSeedAsText keeps a 64-bit seed exact by writing it as a
// decimal string.
func TestRecordSeedAsText(t *testing.T) {
	r := Record{Me: fixtureMe, Seed: math.MaxUint64, Deck: fixtureDeck, Log: fixtureLog}.Seal()
	text := r.JSON()
	if !strings.Contains(text, `"seed": "18446744073709551615"`) {
		t.Errorf("JSON lacks the seed as text:\n%s", text)
	}
	got, isRecord, err := ParseRecord(text)
	if err != nil || !isRecord {
		t.Fatalf("ParseRecord = %v, %v, want record and no error", isRecord, err)
	}
	if got.Seed != math.MaxUint64 {
		t.Errorf("seed = %d, want %d", got.Seed, uint64(math.MaxUint64))
	}
	if got != r {
		t.Errorf("got %+v, want %+v", got, r)
	}
}

// TestRecordSeal sets the version and a check that changes with content.
func TestRecordSeal(t *testing.T) {
	r := Record{Me: fixtureMe, Seed: fixtureSeed, Deck: fixtureDeck, Log: fixtureLog}
	s := r.Seal()
	if s.Version != RecordVersion {
		t.Errorf("Version = %d, want %d", s.Version, RecordVersion)
	}
	if RecordVersion != 1 {
		t.Errorf("RecordVersion = %d, want 1", RecordVersion)
	}
	if !strings.HasPrefix(s.Check, "sha256:") || len(s.Check) != len("sha256:")+64 {
		t.Errorf("Check = %q, want sha256: and 64 hex digits", s.Check)
	}
	if r.Version != 0 || r.Check != "" {
		t.Error("Seal modified its receiver")
	}
	if again := s.Seal(); again != s {
		t.Errorf("resealing changed the record: %+v, want %+v", again, s)
	}
	other := r
	other.Log += "\n"
	if other.Seal().Check == s.Check {
		t.Error("check did not change with the log")
	}
}

// TestRecordJSON writes the fields in order, unescaped, with no trailing
// newline.
func TestRecordJSON(t *testing.T) {
	r := Record{Me: fixtureMe, Seed: 7, Deck: fixtureDeck, Log: "a <b> & c"}.Seal()
	text := r.JSON()
	if !strings.Contains(text, `"log": "a <b> & c"`) {
		t.Errorf("log not written literally:\n%s", text)
	}
	for _, esc := range []string{`\u003c`, `\u003e`, `\u0026`} {
		if strings.Contains(text, esc) {
			t.Errorf("JSON contains %s:\n%s", esc, text)
		}
	}
	if strings.HasSuffix(text, "\n") {
		t.Error("JSON ends with a newline")
	}
	if strings.Contains(text, `"opp_deck"`) {
		t.Errorf("empty opp_deck written:\n%s", text)
	}
	keys := []string{`"tcgreplay_record": 1`, `"me": "nicj"`, `"seed": "7"`, `"deck":`, `"log":`, `"check": "sha256:`}
	last := -1
	for _, k := range keys {
		i := strings.Index(text, k)
		if i < 0 {
			t.Fatalf("JSON lacks %s:\n%s", k, text)
		}
		if i < last {
			t.Errorf("%s out of order:\n%s", k, text)
		}
		last = i
	}
	got, isRecord, err := ParseRecord(text)
	if err != nil || !isRecord || got != r {
		t.Errorf("ParseRecord = %+v, %v, %v, want %+v", got, isRecord, err, r)
	}
}
