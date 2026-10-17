package game

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// RecordVersion is the version of the game record format this playmat
// writes, and the newest it reads.
const RecordVersion = 1

// recordMarker is the JSON key that both identifies a game record and
// carries its version.
const recordMarker = "tcgreplay_record"

// Record is the portable form of an imported game: the raw game (battle
// log, the log owner's name, both decklists as used and the seed that fixed
// how hidden cards were dealt), enough to rebuild it exactly by importing
// the record again. It is plain JSON text the player can keep anywhere.
//
// Check is a SHA-256 over the field values, not the JSON bytes, so a record
// can be re-indented or reflowed freely, while any change to a value is
// caught. Branches, notes and findings are deliberately not part of it.
//
// The field order is the JSON order: small fields first so a note shows
// them above the long log, and the check last.
type Record struct {
	// Version is the record format version; its key also marks the JSON
	// as a game record.
	Version int `json:"tcgreplay_record"`
	// Me is the log owner's player name.
	Me string `json:"me"`
	// Seed fixes how cards the log never names are dealt. It is a decimal
	// string in JSON so 64-bit values survive tools that use doubles.
	Seed uint64 `json:"seed,string"`
	// Deck is the log owner's decklist in TCG Live export format.
	Deck string `json:"deck"`
	// OppDeck is the assumed opponent list as it was passed, if any.
	OppDeck string `json:"opp_deck,omitempty"`
	// Log is the battle log text as copied from TCG Live.
	Log string `json:"log"`
	// Check is "sha256:" and the hex digest of the canonical content.
	Check string `json:"check"`
}

// ErrRecordDamaged is returned for text that is a game record but whose
// content no longer matches its check, even after undoing the
// substitutions note apps are known to make.
var ErrRecordDamaged = errors.New("this game record was damaged on the way (its check does not match), so it was not restored: copy it again unchanged from where it was saved, or import the original battle log")

// sum is the check over the record's canonical content: every field but
// Check, in a fixed order. Changing this function invalidates every record
// players have saved, so it must stay as it is for version 1.
func (r Record) sum() string {
	canonical := struct {
		V       int    `json:"v"`
		Me      string `json:"me"`
		Seed    string `json:"seed"`
		Deck    string `json:"deck"`
		OppDeck string `json:"opp_deck"`
		Log     string `json:"log"`
	}{r.Version, r.Me, strconv.FormatUint(r.Seed, 10), r.Deck, r.OppDeck, r.Log}
	b, err := json.Marshal(canonical)
	if err != nil {
		// Marshalling a struct of strings and an int cannot fail.
		panic(err)
	}
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// Seal returns the record with its version set (if unset) and its check
// computed over its current content.
func (r Record) Seal() Record {
	if r.Version == 0 {
		r.Version = RecordVersion
	}
	r.Check = r.sum()
	return r
}

// JSON renders the record as indented JSON text, without escaping <, > and
// & so a log reads naturally in a note.
func (r Record) JSON() string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		// A struct of strings and integers always encodes.
		panic(err)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// Note-app substitutions the parser knows how to undo.
var (
	curlyDouble = strings.NewReplacer("\u201c", `"`, "\u201d", `"`, "\u201e", `"`, "\u2033", `"`)
	curlySingle = strings.NewReplacer("\u2018", "'", "\u2019", "'", "\u201a", "'", "\u2032", "'")
	oddSpaces   = strings.NewReplacer("\u00a0", " ", "\u202f", " ", "\u2007", " ")
	crlf        = strings.NewReplacer("\r\n", "\n")
)

// stripWrapping removes surrounding whitespace, a byte-order mark and a
// Markdown code fence (``` or ```json) from pasted text.
func stripWrapping(text string) string {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "\ufeff"))
	if strings.HasPrefix(t, "```") {
		if i := strings.IndexByte(t, '\n'); i >= 0 {
			t = t[i+1:]
		} else {
			t = strings.TrimPrefix(t, "```")
		}
		t = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(t), "```"))
	}
	return t
}

// LooksLikeRecord reports whether text is a game record rather than a
// battle log: a JSON object carrying the record marker. A TCG Live battle
// log never starts with "{".
func LooksLikeRecord(text string) bool {
	t := stripWrapping(text)
	if !strings.HasPrefix(t, "{") {
		return false
	}
	return strings.Contains(t, `"`+recordMarker+`"`) || strings.Contains(curlyDouble.Replace(t), `"`+recordMarker+`"`)
}

// ParseRecord reports whether text is a game record and, if so, returns it
// verified. A record reformatted in any way that keeps its values is
// accepted unchanged; one whose values were altered only by the known
// note-app substitutions (curly quotes, non-breaking spaces, Windows line
// ends) is repaired and accepted when its check then matches. Any other
// mismatch is ErrRecordDamaged, and a record newer than RecordVersion is
// refused: a record is never returned unless its check matches.
func ParseRecord(text string) (rec Record, isRecord bool, err error) {
	if !LooksLikeRecord(text) {
		return Record{}, false, nil
	}
	t := stripWrapping(text)
	decoded := false
	// Try the text as is, then with structural quotes and spaces a note
	// app may have changed put back.
	for _, candidate := range []string{t, curlyDouble.Replace(t), oddSpaces.Replace(curlyDouble.Replace(t))} {
		var r Record
		if json.Unmarshal([]byte(candidate), &r) == nil {
			rec, decoded = r, true
			break
		}
	}
	if !decoded {
		return Record{}, true, ErrRecordDamaged
	}
	if rec.Version > RecordVersion {
		return Record{}, true, fmt.Errorf("this game record is version %d; this playmat reads version %d or older, so update the playmat to restore it", rec.Version, RecordVersion)
	}
	if rec.Version < 1 {
		return Record{}, true, ErrRecordDamaged
	}
	if rec.Check == rec.sum() {
		return rec, true, nil
	}
	if r, ok := repair(rec); ok {
		return r, true, nil
	}
	return Record{}, true, ErrRecordDamaged
}

// repair tries every combination of the known substitutions on the
// record's text fields and returns the first candidate whose check
// matches, with its check intact.
func repair(rec Record) (Record, bool) {
	sets := []*strings.Replacer{curlySingle, curlyDouble, oddSpaces, crlf}
	for mask := 1; mask < 1<<len(sets); mask++ {
		fix := func(s string) string {
			for i, set := range sets {
				if mask&(1<<i) != 0 {
					s = set.Replace(s)
				}
			}
			return s
		}
		c := rec
		c.Me, c.Deck, c.OppDeck, c.Log = fix(c.Me), fix(c.Deck), fix(c.OppDeck), fix(c.Log)
		if c.Check == c.sum() {
			return c, true
		}
	}
	return Record{}, false
}
