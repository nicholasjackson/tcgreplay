package cacheadmin

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/catalog"
	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// TestAdminNamedByPlayerID: with a PlayerID function and a recorder, the
// check and clear log lines and the page name the admin by player ID, each
// action writes an admin_action record keyed by that ID, and the admin's
// email appears in no log line or record.
func TestAdminNamedByPlayerID(t *testing.T) {
	ids, err := telemetry.NewIDs("cacheadmin-test-player-id-key-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	rec := telemetry.NewRecorder(&buf, ids, nil)
	c := &fakeCatalog{result: catalog.CheckResult{Version: "fedcba9876543210"}}
	s, logs := newService(c, &fakeImages{})
	s.PlayerID, s.Recorder = rec.ID, rec
	ts := serve(t, s)
	id := rec.ID("admin@example.com")

	if status, _, _ := postTo(t, ts, "/admin/check", nil); status != http.StatusSeeOther {
		t.Fatalf("check: %d", status)
	}
	if status, _, _ := post(t, ts, url.Values{"all": {"1"}}); status != http.StatusSeeOther {
		t.Fatalf("clear: %d", status)
	}
	s.wait()

	log := logs.all()
	for _, want := range []string{"cache admin: " + id + " checked published card sets", "cache admin: " + id + " started clearing images", "for " + id + " in "} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
	_, body := getPage(t, ts)
	if !strings.Contains(body, "Last check now by "+id) || !strings.Contains(body, "Images for everything by "+id) {
		t.Errorf("page does not name the admin by ID:\n%s", body)
	}
	out := buf.String()
	for _, want := range []string{`"event":"admin_action","player":"` + id + `","action":"check"`, `"event":"admin_action","player":"` + id + `","action":"clear"`} {
		if !strings.Contains(out, want) {
			t.Errorf("records lack %s:\n%s", want, out)
		}
	}
	for _, where := range []string{log, out} {
		if strings.Contains(where, "admin@example.com") {
			t.Errorf("the admin's email was logged or recorded:\n%s", where)
		}
	}
}

// TestPlayerLookup: an admin posting an email gets the ID the injected
// function gives for it, shown on the page; an empty email is refused with
// a message; the lookup is recorded as the admin's admin_action, and the
// looked-up email is neither logged nor recorded.
func TestPlayerLookup(t *testing.T) {
	var buf bytes.Buffer
	ids, _ := telemetry.NewIDs("cacheadmin-test-player-id-key-0123456789")
	rec := telemetry.NewRecorder(&buf, ids, nil)
	s, logs := newService(&fakeCatalog{}, nil)
	s.Recorder = rec
	s.PlayerID = func(email string) string { return "p_id-of-" + strings.ToLower(email) }
	ts := serve(t, s)

	if _, body := getPage(t, ts); !strings.Contains(body, `action="/admin/player"`) || !strings.Contains(body, "Find a player's ID") {
		t.Errorf("page offers no lookup:\n%s", body)
	}
	status, _, body := postTo(t, ts, "/admin/player", url.Values{"email": {"  Player@Example.org "}})
	if status != http.StatusOK || !strings.Contains(body, "<code>p_id-of-player@example.org</code>") || !strings.Contains(body, "The player ID for Player@Example.org is") {
		t.Errorf("lookup = %d:\n%s", status, body)
	}
	status, _, body = postTo(t, ts, "/admin/player", url.Values{"email": {"  "}})
	if status != http.StatusBadRequest || !strings.Contains(body, "Enter the email address the player signed in with.") || strings.Contains(body, "<code>") {
		t.Errorf("empty lookup = %d:\n%s", status, body)
	}
	out := buf.String()
	if !strings.Contains(out, `"event":"admin_action","player":"`+rec.ID("admin@example.com")+`","action":"lookup"`) {
		t.Errorf("records lack the lookup:\n%s", out)
	}
	if strings.Contains(strings.ToLower(out+logs.all()), "player@example.org") {
		t.Errorf("the looked-up email was logged or recorded:\n%s\n%s", out, logs.all())
	}
}

// TestNoLookupWithoutPlayerID: without a PlayerID function the page has no
// lookup form and the lookup route is not there.
func TestNoLookupWithoutPlayerID(t *testing.T) {
	s, _ := newService(&fakeCatalog{}, nil)
	ts := serve(t, s)
	if _, body := getPage(t, ts); strings.Contains(body, "/admin/player") || strings.Contains(body, "Find a player") {
		t.Errorf("page offers a lookup without PlayerID:\n%s", body)
	}
	if status, _, _ := postTo(t, ts, "/admin/player", url.Values{"email": {"player@example.org"}}); status != http.StatusNotFound {
		t.Errorf("lookup without PlayerID = %d, want 404", status)
	}
}
