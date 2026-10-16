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
