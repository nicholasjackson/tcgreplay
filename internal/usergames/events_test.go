package usergames

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// lockedBuffer is a bytes.Buffer safe to share with a running registry.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// record is one decoded record.
type record map[string]string

// withRecorder gives r a recorder writing to the returned buffer.
func withRecorder(t *testing.T, r *Registry) (*telemetry.Recorder, *lockedBuffer) {
	t.Helper()
	ids, err := telemetry.NewIDs("usergames-test-player-id-key-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	buf := &lockedBuffer{}
	rec := telemetry.NewRecorder(buf, ids, r.cfg.Now)
	r.cfg.Recorder = rec
	return rec, buf
}

// records decodes buf's records, failing on a line that is not JSON.
func records(t *testing.T, buf *lockedBuffer) []record {
	t.Helper()
	var out []record
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		var r record
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("record %q: %v", l, err)
		}
		out = append(out, r)
	}
	return out
}

// only returns the records of event.
func only(recs []record, event string) []record {
	var out []record
	for _, r := range recs {
		if r["event"] == event {
			out = append(out, r)
		}
	}
	return out
}

// noEmail fails if any email of the test accounts appears in buf.
func noEmail(t *testing.T, buf *lockedBuffer) {
	t.Helper()
	out := buf.String()
	for _, email := range []string{alice.Email, bob.Email, "example.com"} {
		if strings.Contains(out, email) {
			t.Errorf("records contain %q:\n%s", email, out)
		}
	}
}

// TestRecordsUserCapRefusal: a sign-in refused at the user cap writes a
// limited record (reason places) with the player's ID, not the email.
func TestRecordsUserCapRefusal(t *testing.T) {
	r := newRegistry(t, t.TempDir(), newClock().Now)
	r.cfg.MaxUsers = 1
	rec, buf := withRecorder(t, r)
	if err := r.Admit(alice); err != nil {
		t.Fatal(err)
	}
	if err := r.Admit(bob); err == nil {
		t.Fatal("bob admitted at a cap of 1")
	}
	got := only(records(t, buf), telemetry.Limited)
	if len(got) != 1 || got[0]["player"] != rec.ID(bob.Email) || got[0]["reason"] != "places" || got[0]["message"] == "" {
		t.Errorf("limited records %v, want one for bob's ID with reason places", got)
	}
	noEmail(t, buf)
}

// TestRecordsSignOutRemoval: Release writes account_removed (reason
// signed_out) with the player's ID.
func TestRecordsSignOutRemoval(t *testing.T) {
	r := newRegistry(t, t.TempDir(), newClock().Now)
	rec, buf := withRecorder(t, r)
	use(t, r, alice)
	r.Release(alice.ID)
	got := only(records(t, buf), telemetry.AccountRemoved)
	if len(got) != 1 || got[0]["player"] != rec.ID(alice.Email) || got[0]["reason"] != "signed_out" {
		t.Errorf("account_removed records %v, want one for alice, signed_out", got)
	}
	noEmail(t, buf)
}

// TestRecordsExpiryAndIdleRemoval: an idle game's expiry writes
// game_expired with the idle time, and the purge of an idle account
// writes account_removed (reason idle), both by player ID.
func TestRecordsExpiryAndIdleRemoval(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := idleRegistry(t, 2*time.Minute, clock.Now)
	rec, buf := withRecorder(t, r)
	startGame(t, use(t, r, alice).M, "a-game")
	clock.Advance(2*time.Minute + time.Second)
	r.Sweep()
	clock.Advance(purgeAfter)
	r.Sweep()
	recs := records(t, buf)
	exp := only(recs, telemetry.GameExpired)
	if len(exp) != 1 || exp[0]["player"] != rec.ID(alice.Email) || exp[0]["idle"] != "2 minutes" {
		t.Errorf("game_expired records %v, want one for alice after 2 minutes", exp)
	}
	rem := only(recs, telemetry.AccountRemoved)
	if len(rem) != 1 || rem[0]["player"] != rec.ID(alice.Email) || rem[0]["reason"] != "idle" {
		t.Errorf("account_removed records %v, want one for alice, idle", rem)
	}
	noEmail(t, buf)
}

// TestRecordsForeignBoardLink: opening another account's board link
// writes board_refused with the opener's ID.
func TestRecordsForeignBoardLink(t *testing.T) {
	reg, ts := boardSite(t, newClock())
	rec, buf := withRecorder(t, reg)
	startGame(t, reg.Get(alice).M, "a-game")
	get(t, ts, "bob", "/?board="+url.QueryEscape(Handle(alice.ID)))
	got := only(records(t, buf), telemetry.BoardRefused)
	if len(got) != 1 || got[0]["player"] != rec.ID(bob.Email) {
		t.Errorf("board_refused records %v, want one for bob", got)
	}
	noEmail(t, buf)
}
