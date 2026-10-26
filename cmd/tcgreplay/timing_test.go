package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
)

// TestValidTiming: the cap must be above 0 and at most 50s; the check-in
// and lock-release times may be 0 (the default) but not negative.
func TestValidTiming(t *testing.T) {
	ok := game.Timing{WaitCap: 30 * time.Second, CheckIn: 5 * time.Minute, LockRelease: 2 * time.Minute}
	for _, tt := range []struct {
		name   string
		timing game.Timing
		want   string // "" for valid
	}{
		{"defaults", ok, ""},
		{"cap at the limit", game.Timing{WaitCap: 50 * time.Second}, ""},
		{"zero check-in and lock release", game.Timing{WaitCap: time.Second}, ""},
		{"zero cap", game.Timing{WaitCap: 0, CheckIn: time.Minute, LockRelease: time.Minute}, "-wait-cap 0s"},
		{"negative cap", game.Timing{WaitCap: -time.Second}, "-wait-cap -1s"},
		{"cap over 50s", game.Timing{WaitCap: 51 * time.Second}, "-wait-cap 51s: must be above 0 and at most 50s"},
		{"negative check-in", game.Timing{WaitCap: time.Second, CheckIn: -time.Second}, "-check-in -1s: must not be negative"},
		{"negative lock release", game.Timing{WaitCap: time.Second, LockRelease: -time.Second}, "-lock-release -1s: must not be negative"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validTiming(config{timing: tt.timing})
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validTiming(%+v) = %v, want nil", tt.timing, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validTiming(%+v) = %v, want an error containing %q", tt.timing, err, tt.want)
			}
		})
	}
}

// TestConfigArgsCarryTiming: the daemon the shim spawns gets the wait
// timings as flags; unset ones are left to its defaults.
func TestConfigArgsCarryTiming(t *testing.T) {
	c := config{dataDir: "/d", cacheDir: "/c", seat: "p2", uiAddr: "127.0.0.1:7777",
		timing: game.Timing{WaitCap: 20 * time.Second, CheckIn: 3 * time.Minute, LockRelease: 90 * time.Second}}
	args := c.args()
	for _, pair := range [][2]string{{"-wait-cap", "20s"}, {"-check-in", "3m0s"}, {"-lock-release", "1m30s"}} {
		i := slices.Index(args, pair[0])
		if i < 0 || i+1 >= len(args) || args[i+1] != pair[1] {
			t.Errorf("args %q do not pass %s %s", args, pair[0], pair[1])
		}
	}
	c.timing = game.Timing{}
	for _, a := range c.args() {
		if a == "-wait-cap" || a == "-check-in" || a == "-lock-release" {
			t.Errorf("args with no timing set pass %s: %q", a, c.args())
		}
	}
}

// TestManagersUseTiming: the desktop's manager (daemon and in-process,
// both built by newServer) and every hosted account's manager carry the
// configured timings.
func TestManagersUseTiming(t *testing.T) {
	timing := game.Timing{WaitCap: 7 * time.Second, CheckIn: 4 * time.Minute, LockRelease: time.Minute}
	m, _ := newServer(config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true, timing: timing}, "")
	if m.Timing != timing {
		t.Errorf("desktop manager timing %+v, want %+v", m.Timing, timing)
	}
	hs := startHostedWith(t, "https://play.example.com", func(c *config) { c.timing = timing })
	for _, acct := range []hostauth.Account{{ID: "a", Email: "a@example.com"}, {ID: "b", Email: "b@example.com"}} {
		if got := hs.reg.Get(acct).M.Timing; got != timing {
			t.Errorf("hosted manager for %s has timing %+v, want %+v", acct.ID, got, timing)
		}
	}
}

// TestMainRejectsBadTiming: the binary exits with status 2 and a message
// naming the setting when a wait timing is out of range or its variable is
// invalid, and a flag overrides an invalid variable.
func TestMainRejectsBadTiming(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"zero cap", nil, []string{"-wait-cap", "0"}, "-wait-cap 0s"},
		{"negative cap", nil, []string{"-wait-cap", "-1s"}, "-wait-cap -1s"},
		{"cap over 50s", nil, []string{"-wait-cap", "51s"}, "at most 50s"},
		{"cap over 50s from env", map[string]string{"TCGREPLAY_WAIT_CAP": "1m"}, nil, "-wait-cap 1m0s"},
		{"negative check-in", nil, []string{"-check-in", "-1s"}, "-check-in -1s"},
		{"negative lock release", nil, []string{"-lock-release", "-1s"}, "-lock-release -1s"},
		{"invalid env", map[string]string{"TCGREPLAY_CHECK_IN": "soon"}, nil, `TCGREPLAY_CHECK_IN: invalid duration "soon"`},
		{"negative env", map[string]string{"TCGREPLAY_LOCK_RELEASE": "-2m"}, nil, `TCGREPLAY_LOCK_RELEASE: invalid duration "-2m"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"TCGREPLAY_WAIT_CAP", "TCGREPLAY_CHECK_IN", "TCGREPLAY_LOCK_RELEASE"} {
				t.Setenv(k, tt.env[k])
			}
			code, stdout, stderr := runMain(t, append(tt.args, "-version")...)
			if code != 2 || !strings.Contains(stderr, tt.want) {
				t.Fatalf("exit %d, stderr %q; want 2 and %q", code, stderr, tt.want)
			}
			if stdout != "" {
				t.Errorf("printed %q before refusing", stdout)
			}
		})
	}
	t.Run("flag over invalid env", func(t *testing.T) {
		t.Setenv("TCGREPLAY_WAIT_CAP", "abc")
		if code, stdout, stderr := runMain(t, "-wait-cap", "10s", "-version"); code != 0 || stdout == "" {
			t.Fatalf("exit %d, stdout %q, stderr %q; want the version", code, stdout, stderr)
		}
	})
}

// TestMainTimingSettings: the binary uses the defaults when nothing is
// set, follows the variables, and a flag over its variable, each named in
// the start-up log.
func TestMainTimingSettings(t *testing.T) {
	clear := []string{"TCGREPLAY_WAIT_CAP=", "TCGREPLAY_CHECK_IN=", "TCGREPLAY_LOCK_RELEASE="}
	for _, tt := range []struct {
		name string
		env  []string
		want string
	}{
		{"default", nil, "turn waits up to 30s a call, check-in after 5m0s, lock release after 2m0s"},
		{"env", []string{"TCGREPLAY_WAIT_CAP=20s", "TCGREPLAY_CHECK_IN=10m", "TCGREPLAY_LOCK_RELEASE=1m"},
			"turn waits up to 20s a call, check-in after 10m0s, lock release after 1m0s"},
		{"flag over env", []string{"TCGREPLAY_WAIT_CAP=20s", "TCGREPLAY_CHECK_IN=10m", "TCGREPLAY_LOCK_RELEASE=1m",
			"HELPER_ARGS=-wait-cap 40s -check-in 3m -lock-release 90s"},
			"turn waits up to 40s a call, check-in after 3m0s, lock release after 1m30s"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			port := freePort(t)
			env := append([]string{"PORT=" + port, "TCGREPLAY_PUBLIC_URL=https://play.example.com",
				"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret"}, clear...)
			cmd, stderr, _ := helperCmd(t, append(env, tt.env...)...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			t.Cleanup(func() { cmd.Process.Kill() })
			waitHealthy(t, "http://127.0.0.1:"+port, 10*time.Second, exited, stderr)
			if out := stderr.String(); !strings.Contains(out, tt.want) {
				t.Errorf("start-up log does not say %q:\n%s", tt.want, out)
			}
		})
	}
}
