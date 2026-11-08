package cardset

import (
	"html"
	"regexp"
	"strings"
)

var (
	brRe    = regexp.MustCompile(`(?i)<br\s*/?>`)
	tagRe   = regexp.MustCompile(`<[^>]+>`)
	spaceRe = regexp.MustCompile(`[ \t\x{00a0}]+`)
	blankRe = regexp.MustCompile(`\n{3,}`)
	anyWSRe = regexp.MustCompile(`\s+`)
)

// cleanText turns upstream card text into plain text: line breaks for
// <br>, every other tag stripped (an energy span keeps its inner word, so
// `<span class="energy-symbol Fire" title="Fire">Fire</span>` reads
// "Fire"), entities unescaped, runs of spaces collapsed, each line trimmed
// and at most one blank line between paragraphs. Energy symbols such as
// {R} are text, not markup, and pass through unchanged.
func cleanText(s string) string {
	if s == "" {
		return ""
	}
	s = brRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(spaceRe.ReplaceAllString(l, " "))
	}
	s = strings.Join(lines, "\n")
	s = blankRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// energyWords maps energy symbols to the words the other source writes.
var energyWords = strings.NewReplacer(
	"{G}", "Grass", "{R}", "Fire", "{W}", "Water", "{L}", "Lightning",
	"{P}", "Psychic", "{F}", "Fighting", "{D}", "Darkness", "{M}", "Metal",
	"{C}", "Colorless", "{N}", "Dragon", "{Y}", "Fairy",
	// Some TCGdex text writes the symbols in square brackets.
	"[G]", "Grass", "[R]", "Fire", "[W]", "Water", "[L]", "Lightning",
	"[P]", "Psychic", "[F]", "Fighting", "[D]", "Darkness", "[M]", "Metal",
	"[C]", "Colorless", "[N]", "Dragon", "[Y]", "Fairy",
	"’", "'", "‘", "'", "“", `"`, "”", `"`,
)

// compareKey reduces text to what matters when comparing two sources:
// clean text, energy symbols as words, straight quotes and single spaces.
// Two texts with the same key differ only cosmetically.
func compareKey(s string) string {
	s = energyWords.Replace(cleanText(s))
	return strings.TrimSpace(anyWSRe.ReplaceAllString(s, " "))
}

// nameKey is compareKey for card names, also ignoring a leading "Basic":
// TCG Live names Basic Energy "Basic {G} Energy" where TCGdex writes
// "Grass Energy".
func nameKey(s string) string {
	k := compareKey(s)
	if rest, ok := strings.CutPrefix(k, "Basic "); ok && strings.HasSuffix(k, " Energy") {
		return rest
	}
	return k
}

// hasMarkup reports whether s holds an HTML tag.
func hasMarkup(s string) bool { return tagRe.MatchString(s) }
