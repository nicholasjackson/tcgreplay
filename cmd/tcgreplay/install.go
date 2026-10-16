package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nicholasjackson/tcgreplay/skills"
)

// serverName is the MCP server name each client registers.
const serverName = "tcgreplay"

// installTarget says where one client keeps skills and MCP servers.
type installTarget struct {
	skillsDir string
	// register adds the MCP server to the client's configuration.
	register func(bin string, args []string) error
}

// installer installs the skills and registers the server for one client.
// Paths are injectable for tests.
type installer struct {
	home, project string
	out           io.Writer
	// run runs a client's own CLI (claude mcp add); nil means exec.
	run func(name string, args ...string) error
}

// runInstall is "tcgreplay install": copy the bundled skills into a
// client's skills folder and register this binary as its MCP server.
func runInstall(argv []string, out io.Writer) error {
	fl := flag.NewFlagSet("install", flag.ContinueOnError)
	fl.SetOutput(out)
	client := fl.String("client", "", "claude, codex or gemini")
	scope := fl.String("scope", "user", "user (every project) or project (the current directory)")
	data := fl.String("data", "", "game data directory to pass to the server (default: the server's own default)")
	if err := fl.Parse(argv); err != nil {
		return err
	}
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if bin, err = filepath.EvalSymlinks(bin); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	var args []string
	if *data != "" {
		abs, err := filepath.Abs(*data)
		if err != nil {
			return err
		}
		args = []string{"-data", abs}
	}
	in := installer{home: home, project: wd, out: out}
	return in.install(*client, *scope, bin, args)
}

func (in installer) install(client, scope, bin string, args []string) error {
	if scope != "user" && scope != "project" {
		return fmt.Errorf("scope must be user or project, not %q", scope)
	}
	t, err := in.target(client, scope)
	if err != nil {
		return err
	}
	names, err := copySkills(t.skillsDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(in.out, "installed skills %s in %s\n", strings.Join(names, ", "), t.skillsDir)
	if err := t.register(bin, args); err != nil {
		return err
	}
	fmt.Fprintf(in.out, "restart %s to pick up the skills and the %s server\n", client, serverName)
	return nil
}

func (in installer) target(client, scope string) (installTarget, error) {
	base := in.home
	if scope == "project" {
		base = in.project
	}
	switch client {
	case "claude":
		return installTarget{
			skillsDir: filepath.Join(base, ".claude", "skills"),
			register: func(bin string, args []string) error {
				if scope == "project" {
					return in.mergeJSON(filepath.Join(in.project, ".mcp.json"), bin, args)
				}
				return in.claudeAdd(bin, args)
			},
		}, nil
	case "codex":
		return installTarget{
			skillsDir: filepath.Join(base, ".agents", "skills"),
			register: func(bin string, args []string) error {
				// Codex reads MCP servers from the user config only.
				return in.codexTOML(filepath.Join(in.home, ".codex", "config.toml"), bin, args)
			},
		}, nil
	case "gemini":
		return installTarget{
			skillsDir: filepath.Join(base, ".gemini", "skills"),
			register: func(bin string, args []string) error {
				return in.mergeJSON(filepath.Join(base, ".gemini", "settings.json"), bin, args)
			},
		}, nil
	}
	return installTarget{}, fmt.Errorf("client must be claude, codex or gemini, not %q", client)
}

// copySkills writes every bundled skill folder into dir, replacing older
// copies of the same skills and leaving other skills alone.
func copySkills(dir string) ([]string, error) {
	tops, err := fs.ReadDir(skills.FS, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, d := range tops {
		if !d.IsDir() {
			continue
		}
		dst := filepath.Join(dir, d.Name())
		if err := os.RemoveAll(dst); err != nil {
			return nil, err
		}
		err := fs.WalkDir(skills.FS, d.Name(), func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			target := filepath.Join(dir, filepath.FromSlash(p))
			if e.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			b, err := fs.ReadFile(skills.FS, p)
			if err != nil {
				return err
			}
			return os.WriteFile(target, b, 0o644)
		})
		if err != nil {
			return nil, err
		}
		names = append(names, d.Name())
	}
	return names, nil
}

// mergeJSON sets mcpServers.tcgreplay in a JSON config (Claude's .mcp.json,
// Gemini's settings.json), keeping everything else in it.
func (in installer) mergeJSON(path, bin string, args []string) error {
	cfg := map[string]any{}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(b, &cfg); err != nil {
			return fmt.Errorf("%s is not plain JSON (%v); add this under mcpServers yourself:\n%s", path, err, serverJSON(bin, args))
		}
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[serverName] = map[string]any{"command": bin, "args": nonNil(args)}
	cfg["mcpServers"] = servers
	b, err = json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(in.out, "registered the %s MCP server in %s\n", serverName, path)
	return nil
}

func serverJSON(bin string, args []string) string {
	b, _ := json.MarshalIndent(map[string]any{serverName: map[string]any{"command": bin, "args": nonNil(args)}}, "", "  ")
	return string(b)
}

func nonNil(args []string) []string {
	if args == nil {
		return []string{}
	}
	return args
}

// codexSection matches the [mcp_servers.tcgreplay] table up to the next
// table header.
var codexSection = regexp.MustCompile(`(?ms)^\[mcp_servers\.` + serverName + `\]\n.*?(?:^\[|\z)`)

// codexTOML sets [mcp_servers.tcgreplay] in Codex's config.toml, replacing
// an earlier entry and keeping the rest of the file as written.
func (in installer) codexTOML(path, bin string, args []string) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = tomlString(a)
	}
	section := fmt.Sprintf("[mcp_servers.%s]\ncommand = %s\nargs = [%s]\n", serverName, tomlString(bin), strings.Join(quoted, ", "))
	s := string(b)
	if loc := codexSection.FindStringIndex(s); loc != nil {
		end := loc[1]
		if strings.HasSuffix(s[loc[0]:end], "[") {
			end-- // keep the next table's header
			section += "\n"
		}
		s = s[:loc[0]] + section + s[end:]
	} else {
		if s != "" && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		if s != "" {
			s += "\n"
		}
		s += section
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(in.out, "registered the %s MCP server in %s\n", serverName, path)
	return nil
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	b, _ := json.Marshal(s) // JSON string escapes are valid TOML basic-string escapes
	return string(b)
}

// claudeAdd registers the server for every project with Claude Code's own
// CLI, which owns its user config.
func (in installer) claudeAdd(bin string, args []string) error {
	run := in.run
	if run == nil {
		run = func(name string, a ...string) error {
			cmd := exec.Command(name, a...)
			cmd.Stdout, cmd.Stderr = in.out, in.out
			return cmd.Run()
		}
	}
	cmdArgs := append([]string{"mcp", "add", "--scope", "user", serverName, "--", bin}, args...)
	_ = run("claude", "mcp", "remove", "--scope", "user", serverName) // replace an older entry; absent is fine
	if err := run("claude", cmdArgs...); err != nil {
		return fmt.Errorf("could not run claude to register the server (%v); run this yourself:\n  claude %s", err, strings.Join(cmdArgs, " "))
	}
	fmt.Fprintf(in.out, "registered the %s MCP server with Claude Code (user scope)\n", serverName)
	return nil
}
