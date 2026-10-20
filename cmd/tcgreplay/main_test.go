package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMainHelper is not a test: the command-line tests below re-run this
// test binary with TCGREPLAY_MAIN_HELPER=1 so it calls main() with exactly
// the arguments in HELPER_ARGS.
func TestMainHelper(t *testing.T) {
	if os.Getenv("TCGREPLAY_MAIN_HELPER") != "1" {
		t.Skip("helper process only")
	}
	os.Args = append([]string{"tcgreplay"}, strings.Fields(os.Getenv("HELPER_ARGS"))...)
	main()
	os.Exit(0)
}

// runMain runs main in a subprocess with args, giving up after a few
// seconds so a regression that starts the shim cannot hang the test. It
// returns the exit code, stdout and stderr.
func runMain(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestMainHelper$")
	cmd.Env = append(os.Environ(), "TCGREPLAY_MAIN_HELPER=1", "HELPER_ARGS="+strings.Join(args, " "))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("main %q still running after 10s; stderr:\n%s", args, stderr.String())
	}
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return cmd.ProcessState.ExitCode(), stdout.String(), stderr.String()
}

// TestUnknownCommandRefused checks that a stray word, the removed install
// subcommand among them, fails with usage and starts nothing: the data dir
// (which logging would create) is never made and nothing speaks MCP on
// stdout.
func TestUnknownCommandRefused(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"install", []string{"install"}, `unknown command "install"`},
		{"install with client", []string{"install", "-client", "claude"}, `unknown command "install"`},
		{"other word", []string{"serve"}, `unknown command "serve"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			data := filepath.Join(tmp, "data")
			// Flags go first: the flag package stops at the first non-flag.
			args := append([]string{"-data", data, "-cache", filepath.Join(tmp, "cache"), "-offline"}, tt.args...)
			code, stdout, stderr := runMain(t, args...)
			if code != 2 {
				t.Errorf("exit code %d, want 2; stderr:\n%s", code, stderr)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr lacks %q:\n%s", tt.want, stderr)
			}
			if !strings.Contains(stderr, "Usage of tcgreplay") {
				t.Errorf("stderr lacks the usage:\n%s", stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("data dir %s was created (err %v), want it untouched", data, err)
			}
		})
	}
}

// TestHelpOmitsInstall checks that -h lists the flags but says nothing of
// installing or skills.
func TestHelpOmitsInstall(t *testing.T) {
	_, _, stderr := runMain(t, "-h")
	if !strings.Contains(stderr, "-hosted") {
		t.Fatalf("help lacks the flags:\n%s", stderr)
	}
	lower := strings.ToLower(stderr)
	for _, word := range []string{"install", "skill"} {
		if strings.Contains(lower, word) {
			t.Errorf("help mentions %q:\n%s", word, stderr)
		}
	}
}
