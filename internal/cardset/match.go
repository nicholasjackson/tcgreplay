package cardset

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// matchMalieSet finds the malie set holding a TCGdex set's prints: first
// one whose abbr is the set's decklist code, then one whose TCG Live log
// codes name the set (cards.LogCodeID). owner maps an upper-case decklist
// code to the TCGdex set it belongs to when several share it (30th and
// 30th-c are both "30C"; the shorter id keeps it, see codeOwners); a nil
// owner treats the set as the code's owner.
func matchMalieSet(set TCGdexSet, index map[string]MalieSetRef, owner map[string]string) (string, MalieSetRef, bool) {
	keys := make([]string, 0, len(index))
	for k := range index {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	code := strings.ToUpper(strings.TrimSpace(set.Abbreviation.Official))
	if o, ok := owner[code]; code != "" && (!ok || o == set.ID) {
		for _, k := range keys {
			if strings.EqualFold(strings.TrimSpace(index[k].Abbr), code) {
				return k, index[k], true
			}
		}
	}
	for _, k := range keys {
		if id, ok := cards.LogCodeID(k + "_1"); ok && setPart(id) == set.ID {
			return k, index[k], true
		}
	}
	return "", MalieSetRef{}, false
}

// codeOwners maps each upper-case decklist code to the TCGdex set that
// keeps it: the one with the shorter id when several share a code (the
// rule the playmat's catalog uses), ties by id.
func codeOwners(sets []TCGdexSet) map[string]string {
	owner := map[string]string{}
	for _, s := range sets {
		code := strings.ToUpper(strings.TrimSpace(s.Abbreviation.Official))
		if code == "" {
			continue
		}
		if old, ok := owner[code]; !ok || len(s.ID) < len(old) || (len(s.ID) == len(old) && s.ID < old) {
			owner[code] = s.ID
		}
	}
	return owner
}

// setPart is the set id of a print id: everything before the last "-".
func setPart(id string) string {
	if i := strings.LastIndex(id, "-"); i > 0 {
		return id[:i]
	}
	return ""
}

// matchPrintID gives a malie entry its TCGdex print id and collector
// number: the TCGdex print with the same collector number, else the id its
// TCG Live log code names if that is in this set, else the set id and the
// number padded like the set's other numbers.
func matchPrintID(set TCGdexSet, e MalieCard) (id, localID string) {
	n := int(e.Number.Numeric)
	if n > 0 {
		for _, c := range set.Cards {
			if v, err := strconv.Atoi(c.LocalID); err == nil && v == n {
				return c.ID, c.LocalID
			}
		}
	}
	if lid, ok := cards.LogCodeID(e.Ext.TCGL.CardID); ok && setPart(lid) == set.ID {
		return lid, lid[len(set.ID)+1:]
	}
	width := 3
	for _, c := range set.Cards {
		if _, err := strconv.Atoi(c.LocalID); err == nil {
			width = len(c.LocalID)
			break
		}
	}
	local := fmt.Sprintf("%0*d", width, n)
	return set.ID + "-" + local, local
}
