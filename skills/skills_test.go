package skills

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// The Agent Skills format: a name matching the folder (lowercase, digits
// and hyphens, at most 64 characters) and a description of at most 1024.
func TestSkillsFollowTheFormat(t *testing.T) {
	dirs, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	valid := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	for _, d := range dirs {
		b, err := fs.ReadFile(FS, d.Name()+"/SKILL.md")
		if err != nil {
			t.Errorf("%s: %v", d.Name(), err)
			continue
		}
		head, _, ok := strings.Cut(strings.TrimPrefix(string(b), "---\n"), "\n---\n")
		if !ok || !strings.HasPrefix(string(b), "---\n") {
			t.Errorf("%s: no frontmatter", d.Name())
			continue
		}
		fields := map[string]string{}
		for _, line := range strings.Split(head, "\n") {
			k, v, _ := strings.Cut(line, ": ")
			fields[k] = v
		}
		if name := fields["name"]; name != d.Name() || len(name) > 64 || !valid.MatchString(name) {
			t.Errorf("%s: bad name %q", d.Name(), name)
		}
		if desc := fields["description"]; desc == "" || len(desc) > 1024 {
			t.Errorf("%s: description is %d characters", d.Name(), len(desc))
		}
		// Skills serve any assistant and any player.
		for _, word := range []string{"Claude", "Nic "} {
			if strings.Contains(string(b), word) {
				t.Errorf("%s mentions %q", d.Name(), word)
			}
		}
	}
}

// The bundled skills must not send the assistant to tools or arguments the
// playmat no longer has: games are not stored, so none can be listed or
// loaded, and set_opponent_deck cannot build a separate game.
func TestSkillsUseCurrentTools(t *testing.T) {
	dirs, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		b, err := fs.ReadFile(FS, d.Name()+"/SKILL.md")
		if err != nil {
			t.Errorf("%s: %v", d.Name(), err)
			continue
		}
		for _, word := range []string{"list_games", "load_game", "new_id"} {
			if strings.Contains(string(b), word) {
				t.Errorf("%s mentions the removed %q", d.Name(), word)
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

// Coaching skills name turns by the combined number and always put the seat
// in the branch name, so a turn is never ambiguous between the two players.
func TestCoachingSkillsNameTurnsByRoundAndSeat(t *testing.T) {
	dirs, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if !strings.HasPrefix(d.Name(), "tcg-") {
			continue
		}
		b, err := fs.ReadFile(FS, d.Name()+"/SKILL.md")
		if err != nil {
			t.Errorf("%s: %v", d.Name(), err)
			continue
		}
		for _, p := range turnNumberingProblems(string(b)) {
			t.Errorf("%s: %s", d.Name(), p)
		}
	}
}
