package telemetry

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxRun is the most words one redacted run can span.
const maxRun = 12

// commonWords are the playmat's own words, which a run made only of them
// keeps even when a player's text holds the same words. Matching is
// case-insensitive; a word of digits only is common too.
var commonWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		a an the and or
		in on at to of for from by with into onto before after as
		no not has have is are was already only
		line turn hand deck bench active discard prize prizes card cards
		energy setup section player players log decklist
		current branch game`) {
		commonWords[w] = true
	}
}

// Redact returns msg with the player's own text taken out, keeping the
// message's shape (which check failed and where):
//   - every double-quoted segment (Go %q output) becomes "…";
//   - every run of words also found, word-bounded and case-sensitive, in
//     one of the sensitive strings becomes …, unless all its words are
//     common playmat words (common words at either end of a run stay);
//   - adjacent … are collapsed into one.
//
// Over-redaction is preferred to a leak.
func Redact(msg string, sensitive []string) string {
	msg = redactQuoted(msg)
	words := wordSpans(msg)
	type span struct{ start, end int }
	var cuts []span
	for i := 0; i < len(words); {
		matched := 0
		for n := min(maxRun, len(words)-i); n > 0; n-- {
			if inAny(msg[words[i].start:words[i+n-1].end], sensitive) {
				matched = n
				break
			}
		}
		if matched == 0 || allCommon(msg, words[i:i+matched]) {
			i++
			continue
		}
		// Common words at either end of the run stay: they hold nothing
		// of the player's, and keep the message's shape.
		first, last := i, i+matched-1
		for isCommon(msg, words[first]) {
			first++
		}
		for isCommon(msg, words[last]) {
			last--
		}
		cuts = append(cuts, span{words[first].start, words[last].end})
		i = last + 1
	}
	if len(cuts) > 0 {
		var b strings.Builder
		last := 0
		for _, c := range cuts {
			b.WriteString(msg[last:c.start])
			b.WriteString("…")
			last = c.end
		}
		b.WriteString(msg[last:])
		msg = b.String()
	}
	return adjacent.ReplaceAllString(msg, "…")
}

// adjacent matches … marks separated only by spaces, apostrophes or
// hyphens.
var adjacent = regexp.MustCompile(`…(?:[\s'’\-]*…)+`)

// redactQuoted replaces each double-quoted segment, honouring escaped
// quotes, with "…". An unclosed quote hides everything after it.
func redactQuoted(msg string) string {
	if !strings.Contains(msg, `"`) {
		return msg
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(msg, '"')
		if i < 0 {
			b.WriteString(msg)
			return b.String()
		}
		b.WriteString(msg[:i])
		end := -1
		for j := i + 1; j < len(msg); j++ {
			if msg[j] == '\\' {
				j++
				continue
			}
			if msg[j] == '"' {
				end = j
				break
			}
		}
		if end < 0 {
			b.WriteString(`"…`)
			return b.String()
		}
		b.WriteString(`"…"`)
		msg = msg[end+1:]
	}
}

type wordSpan struct{ start, end int }

// wordSpans returns the byte spans of msg's words: runs of letters and
// digits.
func wordSpans(msg string) []wordSpan {
	var out []wordSpan
	start := -1
	for i, r := range msg {
		if isWord(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			out = append(out, wordSpan{start, i})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, wordSpan{start, len(msg)})
	}
	return out
}

func isWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// allCommon reports whether every word is a common playmat word.
func allCommon(msg string, words []wordSpan) bool {
	for _, w := range words {
		if !isCommon(msg, w) {
			return false
		}
	}
	return true
}

// isCommon reports whether the word is a common playmat word or a number.
func isCommon(msg string, w wordSpan) bool {
	word := msg[w.start:w.end]
	return commonWords[strings.ToLower(word)] || strings.TrimFunc(word, unicode.IsDigit) == ""
}

// inAny reports whether text occurs, word-bounded, in any of ss.
func inAny(text string, ss []string) bool {
	for _, s := range ss {
		for off := 0; ; {
			i := strings.Index(s[off:], text)
			if i < 0 {
				break
			}
			i += off
			end := i + len(text)
			before, _ := utf8.DecodeLastRuneInString(s[:i])
			after, _ := utf8.DecodeRuneInString(s[end:])
			if (i == 0 || !isWord(before)) && (end == len(s) || !isWord(after)) {
				return true
			}
			_, size := utf8.DecodeRuneInString(s[i:])
			off = i + size
		}
	}
	return false
}

// Decklist lines, as internal/deck parses them: a count, the name, and an
// optional set code and number.
var (
	deckLineRe = regexp.MustCompile(`^(\d+)\s+(.+?)\s+([A-Z][A-Z0-9]*(?:-[A-Z0-9]+)?|\d+[A-Za-z]+)\s+([A-Za-z]*\d+[A-Za-z]?)$`)
	deckBareRe = regexp.MustCompile(`^(\d+)\s+(.+)$`)
)

// SensitiveFromJSON returns every string value in raw (a tool call's
// arguments), with multi-line strings split into their lines, and the
// card name of each decklist line added.
func SensitiveFromJSON(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case string:
			for _, l := range strings.Split(v, "\n") {
				l = strings.TrimSpace(strings.TrimPrefix(l, "\ufeff"))
				if l == "" {
					continue
				}
				out = append(out, l)
				if m := deckLineRe.FindStringSubmatch(l); m != nil {
					out = append(out, m[2])
				} else if m := deckBareRe.FindStringSubmatch(l); m != nil {
					out = append(out, m[2])
				}
			}
		case []any:
			for _, e := range v {
				walk(e)
			}
		case map[string]any:
			for _, e := range v {
				walk(e)
			}
		}
	}
	walk(v)
	return out
}
