package guides

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The six guides, in the order players meet them.
var wantNames = []string{"import-game", "opponent-deck", "review-game", "replay-drill", "before-you-move", "numbers-drill"}

// rawGuides returns every embedded guide file's contents, keyed by file name.
func rawGuides(t *testing.T) map[string]string {
	t.Helper()
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		b, err := fs.ReadFile(files, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(b)
	}
	return out
}

// Each file has a frontmatter whose name matches the file stem (lowercase,
// digits and hyphens, at most 64 characters) and a one-line description.
func TestGuidesFollowTheFormat(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	for file, text := range rawGuides(t) {
		head, _, ok := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
		if !ok || !strings.HasPrefix(text, "---\n") {
			t.Errorf("%s: no frontmatter", file)
			continue
		}
		fields := map[string]string{}
		for _, line := range strings.Split(head, "\n") {
			k, v, _ := strings.Cut(line, ": ")
			fields[k] = v
		}
		stem := strings.TrimSuffix(file, ".md")
		if name := fields["name"]; name != stem || len(name) > 64 || !valid.MatchString(name) {
			t.Errorf("%s: bad name %q", file, name)
		}
		// The description is the "use when" line an assistant picks a guide by.
		if desc := fields["description"]; desc == "" || strings.Contains(desc, "\n") || len(desc) > 200 {
			t.Errorf("%s: description is %d characters: %q", file, len(desc), desc)
		}
		// Guides serve any assistant and any player.
		for _, word := range []string{"Claude", "Nic "} {
			if strings.Contains(text, word) {
				t.Errorf("%s mentions %q", file, word)
			}
		}
	}
}

// The guides must not send the assistant to tools or arguments the playmat
// no longer has: games are not stored, so none can be listed or loaded, and
// set_opponent_deck cannot build a separate game.
func TestGuidesUseCurrentTools(t *testing.T) {
	for file, text := range rawGuides(t) {
		for _, word := range []string{"list_games", "load_game", "new_id"} {
			if strings.Contains(text, word) {
				t.Errorf("%s mentions the removed %q", file, word)
			}
		}
	}
}

// Guides are no longer skills: they never call themselves or each other
// skills, and never use the old tcg- skill names.
func TestGuidesDoNotMentionSkills(t *testing.T) {
	for file, text := range rawGuides(t) {
		if strings.Contains(strings.ToLower(text), "skill") {
			t.Errorf("%s mentions a skill", file)
		}
		if strings.Contains(text, "tcg-") {
			t.Errorf("%s uses a tcg- name", file)
		}
	}
}

// Every "`<name>` guide" a guide sends the assistant to must exist.
func TestGuideReferencesExist(t *testing.T) {
	ref := regexp.MustCompile("(?i)`([a-z0-9-]+)` guide")
	for file, text := range rawGuides(t) {
		for _, m := range ref.FindAllStringSubmatch(text, -1) {
			if _, err := Get(m[1]); err != nil {
				t.Errorf("%s refers to the %q guide, which does not exist", file, m[1])
			}
		}
	}
}

var (
	// Per-player ordinals ("your 2nd turn") and the old "name turns both
	// ways" rule: turns are named by the combined number only.
	translateRule = regexp.MustCompile(`(?i)both ways|your \d+(st|nd|rd|th) turn`)
	codeSpan      = regexp.MustCompile("`[^`\n]+`")
	turnToken     = regexp.MustCompile(`-t(\d+|N|<N>)`)
	seatSuffix    = regexp.MustCompile(`^-(<seat>|p[12])`)
)

// turnNumberingProblems reports text that translates between per-player and
// combined turn numbers, and backtick-quoted turn branch names with no seat.
func turnNumberingProblems(text string) []string {
	var out []string
	if m := translateRule.FindString(text); m != "" {
		out = append(out, "translates turn numbers: "+m)
	}
	for _, span := range codeSpan.FindAllString(text, -1) {
		for _, loc := range turnToken.FindAllStringIndex(span, -1) {
			if !seatSuffix.MatchString(span[loc[1]:]) {
				out = append(out, "turn branch without a seat: "+span)
				break
			}
		}
	}
	return out
}

func TestTurnNumberingCheckerCatchesOldText(t *testing.T) {
	bad := []string{
		"Name turns both ways: turn 4 (your 2nd turn)",
		"name turns BOTH WAYS",
		"that was (your 3rd turn)",
		"a branch named `review-t4`",
		"`review-tN`",
		"`review-t<N>`",
		"`bym-t<N>`",
		"`drill-<game>-t<N>`",
		"`drill-<game>-t<N>-2`",
	}
	good := []string{
		"`review-t2-p2`",
		"`review-t<N>-<seat>`",
		"`bym-t3-p1`",
		"`drill-<game>-t<N>-<seat>`",
		`turn 2, your turn`,
	}
	for _, s := range bad {
		if len(turnNumberingProblems(s)) == 0 {
			t.Errorf("not rejected: %q", s)
		}
	}
	for _, s := range good {
		if p := turnNumberingProblems(s); len(p) != 0 {
			t.Errorf("wrongly rejected %q: %v", s, p)
		}
	}
}

// Guides name turns by the combined number and always put the seat in the
// branch name, so a turn is never ambiguous between the two players.
func TestGuidesNameTurnsByRoundAndSeat(t *testing.T) {
	for file, text := range rawGuides(t) {
		for _, p := range turnNumberingProblems(text) {
			t.Errorf("%s: %s", file, p)
		}
	}
}

// List returns exactly the six guides, in order, and every embedded file is
// one of them.
func TestListNamesTheSixGuidesInOrder(t *testing.T) {
	got := List()
	if len(got) != len(wantNames) {
		t.Fatalf("List returned %d guides, want %d", len(got), len(wantNames))
	}
	listed := map[string]bool{}
	for i, g := range got {
		if g.Name != wantNames[i] {
			t.Errorf("guide %d is %q, want %q", i, g.Name, wantNames[i])
		}
		listed[g.Name+".md"] = true
	}
	for file := range rawGuides(t) {
		if !listed[file] {
			t.Errorf("%s is embedded but not listed", file)
		}
	}
}

// Get returns the whole workflow body without its frontmatter.
func TestGetReturnsTheFullText(t *testing.T) {
	g, err := Get("review-game")
	if err != nil {
		t.Fatal(err)
	}
	if g.Description == "" {
		t.Error("review-game has no description")
	}
	if strings.HasPrefix(g.Text, "---") || strings.Contains(g.Text, "name: review-game") {
		t.Errorf("review-game text keeps its frontmatter: %.80q", g.Text)
	}
	for _, want := range []string{"decision points", "Finding ("} {
		if !strings.Contains(g.Text, want) {
			t.Errorf("review-game text lacks %q", want)
		}
	}
}

// An unknown name is an error that tells the assistant every valid name.
func TestGetUnknownListsTheGuides(t *testing.T) {
	_, err := Get("tcg-review")
	if err == nil {
		t.Fatal("Get(tcg-review) did not fail")
	}
	for _, n := range wantNames {
		if !strings.Contains(err.Error(), n) {
			t.Errorf("error %q does not name %q", err, n)
		}
	}
}

// One rule or step per guide that moving it from a skill must not drop.
func TestGuidesKeepTheirRules(t *testing.T) {
	anchors := map[string][]string{
		"import-game":     {"Read the log first", "Triage"},
		"opponent-deck":   {"Name the archetype"},
		"review-game":     {"Ask before judging", "Put the board where you are talking", "Test the alternative"},
		"replay-drill":    {"Set it up without the answer"},
		"before-you-move": {"The three questions"},
		"numbers-drill":   {"Damage order"},
	}
	for name, phrases := range anchors {
		g, err := Get(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, p := range phrases {
			if !strings.Contains(g.Text, p) {
				t.Errorf("%s lacks %q", name, p)
			}
		}
	}
}

// The coaching skills left the repo for good, and Spektacular's own skills
// are still reachable through the assistant skill links.
func TestRepoHoldsNoCoachingSkills(t *testing.T) {
	root := filepath.Join("..", "..")
	entries, err := os.ReadDir(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "tcg-") {
			t.Errorf("skills/%s is still in the repo", e.Name())
		}
	}
	for _, link := range []string{".claude", ".agents"} {
		p := filepath.Join(root, link, "skills", "spek-implement", "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}
