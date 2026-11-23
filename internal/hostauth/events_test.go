package hostauth

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// eventBuffer is a bytes.Buffer safe to share with a running server.
type eventBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *eventBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *eventBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// recordTo gives a a recorder writing to the returned buffer.
func recordTo(t *testing.T, a *Auth) (*telemetry.Recorder, *eventBuffer) {
	t.Helper()
	ids, err := telemetry.NewIDs("hostauth-test-player-id-key-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	buf := &eventBuffer{}
	rec := telemetry.NewRecorder(buf, ids, nil)
	a.cfg.Recorder = rec
	return rec, buf
}

// wantRecord fails unless buf holds a record of event for player, with
// fields (a JSON fragment) when not empty.
func wantRecord(t *testing.T, buf *eventBuffer, event, player, fields string) {
	t.Helper()
	want := `"event":"` + event + `","player":"` + player + `"` + fields
	if !strings.Contains(buf.String(), want) {
		t.Errorf("records lack %s:\n%s", want, buf)
	}
}

// noEmails fails if an email address of the test accounts appears.
func noEmails(t *testing.T, buf *eventBuffer) {
	t.Helper()
	if strings.Contains(buf.String(), "example.com") {
		t.Errorf("records contain an email:\n%s", buf)
	}
}

// TestRecordsRateLimit: a request over the limit writes a limited record
// (reason rate) with the player's ID and the limit message.
func TestRecordsRateLimit(t *testing.T) {
	a, _ := newLimited(t, 2)
	rec, buf := recordTo(t, a)
	spend(t, a, limitAccount, 2)
	if err := a.allow(context.Background(), limitAccount); err == nil {
		t.Fatal("third request allowed at a limit of 2")
	}
	wantRecord(t, buf, telemetry.Limited, rec.ID(limitAccount.Email), `,"message":"too many requests: this account is limited to 2 requests a minute`)
	if !strings.Contains(buf.String(), `"reason":"rate"`) {
		t.Errorf("no rate reason:\n%s", buf)
	}
	noEmails(t, buf)
}

// TestRecordsSignInAndOut: a board sign-in writes sign_in, and signing out
// writes sign_out, both with the player's ID.
func TestRecordsSignInAndOut(t *testing.T) {
	b := newAdminBoard(t)
	rec, buf := recordTo(t, b.a)
	_, cookie := b.signInBoard(t, "/")
	id := rec.ID(hostauthtest.DefaultAccount.Email)
	wantRecord(t, buf, telemetry.SignIn, id, "")
	resp, _ := b.send(t, "POST", "/auth/logout", cookie, b.fromPlaymat)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-out: %d", resp.StatusCode)
	}
	wantRecord(t, buf, telemetry.SignOut, id, "")
	noEmails(t, buf)
}

// TestRecordsAdminRefusals: a non-admin reaching /admin, and an admin's
// change from another site, each write admin_refused with the caller's ID
// and why.
func TestRecordsAdminRefusals(t *testing.T) {
	b := newAdminBoard(t, "one@example.com")
	rec, buf := recordTo(t, b.a)
	_, adminCookie := b.signInBoard(t, "/")
	b.send(t, "POST", "/admin/clear", adminCookie, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") })
	wantRecord(t, buf, telemetry.AdminRefused, rec.ID("one@example.com"), `,"reason":"not sent from the playmat"`)

	two := hostauthtest.Account{ID: "sub-2", Email: "two@example.com"}
	b.g.SetAccount(two)
	br := b.browser(t)
	get(t, br, b.ts.URL+"/")
	b.send(t, "GET", "/admin", b.sessionCookieIn(br), nil)
	wantRecord(t, buf, telemetry.AdminRefused, rec.ID(two.Email), `,"reason":"not admin"`)
	noEmails(t, buf)
}
