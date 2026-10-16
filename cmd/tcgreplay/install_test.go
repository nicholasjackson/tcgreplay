package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newInstaller(t *testing.T) installer {
	t.Helper()
	return installer{home: t.TempDir(), project: t.TempDir(), out: io.Discard}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallCopiesSkills(t *testing.T) {
	in := newInstaller(t)
	stale := filepath.Join(in.home, ".gemini", "skills", "tcg-import-game", "old.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(stale, []byte("old"), 0o644)
	other := filepath.Join(in.home, ".gemini", "skills", "mine", "SKILL.md")
	os.MkdirAll(filepath.Dir(other), 0o755)
	os.WriteFile(other, []byte("keep"), 0o644)

	if err := in.install("gemini", "user", "/bin/tcgreplay", nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tcg-import-game", "tcg-opponent-deck", "tcg-review-game"} {
		s := readFile(t, filepath.Join(in.home, ".gemini", "skills", name, "SKILL.md"))
		if !strings.HasPrefix(s, "---\nname: "+name+"\n") {
			t.Errorf("%s: bad SKILL.md head %q", name, s[:min(len(s), 40)])
		}
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("an older copy of a bundled skill was not replaced")
	}
	if readFile(t, other) != "keep" {
		t.Error("another skill was touched")
	}
}

func TestInstallGeminiMergesSettings(t *testing.T) {
	in := newInstaller(t)
	path := filepath.Join(in.project, ".gemini", "settings.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(`{"theme": "dark", "mcpServers": {"other": {"command": "x"}}}`), 0o644)
	if err := in.install("gemini", "project", "/bin/tcgreplay", []string{"-data", "/d"}); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Theme      string
		McpServers map[string]struct {
			Command string
			Args    []string
		}
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "dark" || cfg.McpServers["other"].Command != "x" {
		t.Errorf("lost existing settings: %+v", cfg)
	}
	if s := cfg.McpServers["tcgreplay"]; s.Command != "/bin/tcgreplay" || strings.Join(s.Args, " ") != "-data /d" {
		t.Errorf("server entry %+v", s)
	}
	if _, err := os.Stat(filepath.Join(in.project, ".gemini", "skills", "tcg-review-game", "SKILL.md")); err != nil {
		t.Error("project scope did not install skills in the project")
	}
}

func TestInstallCodexTOML(t *testing.T) {
	in := newInstaller(t)
	path := filepath.Join(in.home, ".codex", "config.toml")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("model = \"x\"\n\n[mcp_servers.tcgreplay]\ncommand = \"/old\"\nargs = []\n\n[mcp_servers.other]\ncommand = \"y\"\n"), 0o644)
	if err := in.install("codex", "user", "/bin/tcgreplay", nil); err != nil {
		t.Fatal(err)
	}
	if err := in.install("codex", "user", "/bin/tcgreplay", nil); err != nil {
		t.Fatal(err)
	}
	s := readFile(t, path)
	if strings.Count(s, "[mcp_servers.tcgreplay]") != 1 || strings.Contains(s, "/old") || !strings.Contains(s, `command = "/bin/tcgreplay"`) {
		t.Errorf("config:\n%s", s)
	}
	if !strings.Contains(s, "model = \"x\"") || !strings.Contains(s, "[mcp_servers.other]\ncommand = \"y\"") {
		t.Errorf("lost other config:\n%s", s)
	}
	if _, err := os.Stat(filepath.Join(in.home, ".agents", "skills", "tcg-import-game", "SKILL.md")); err != nil {
		t.Error("skills not in ~/.agents/skills")
	}
}

func TestInstallClaudeUsesItsCLI(t *testing.T) {
	in := newInstaller(t)
	var calls []string
	in.run = func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}
	if err := in.install("claude", "user", "/bin/tcgreplay", nil); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[1] != "claude mcp add --scope user tcgreplay -- /bin/tcgreplay" {
		t.Errorf("calls %q", calls)
	}
	if _, err := os.Stat(filepath.Join(in.home, ".claude", "skills", "tcg-replay-drill", "SKILL.md")); err != nil {
		t.Error("skills not in ~/.claude/skills")
	}
	if err := in.install("cursor", "user", "/bin/tcgreplay", nil); err == nil {
		t.Error("unknown client accepted")
	}
}
