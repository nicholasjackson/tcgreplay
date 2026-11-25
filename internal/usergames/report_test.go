package usergames

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/importreports"
	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// TestReportImport: a signed-in account's report is saved privately in
// the shared store, under its player ID with the players' names replaced,
// and records import_reported with the report's ID and no email; it takes
// no place. Without a token it is refused.
func TestReportImport(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)}
	r := newRegistry(t, t.TempDir(), clock.Now)
	rec, buf := withRecorder(t, r)
	store := spacestest.New(t)
	r.cfg.Reports = &importreports.Service{Store: store.Client(t), Now: clock.Now}

	in := mcpserver.ImportReport{
		Log:   "Setup\nAliceTCG chose heads for the opening coin flip.\nBobTCG won the coin toss.\n",
		Deck:  "4 Riolu",
		Error: "expected two players in Setup, found [AliceTCG]",
		Me:    "AliceTCG", Assistant: "Claude",
	}
	id, err := r.ReportImport(context.Background(), tokenExtra(alice.ID, alice.Email), in)
	if err != nil {
		t.Fatal(err)
	}
	keys := store.Keys(importreports.Prefix)
	if len(keys) != 1 || !strings.HasSuffix(keys[0], id+".json") {
		t.Fatalf("stored keys %v for report %s", keys, id)
	}
	if store.Public(keys[0]) {
		t.Fatal("report stored public")
	}
	body, _ := store.Object(keys[0])
	if strings.Contains(string(body), "AliceTCG") || strings.Contains(string(body), "BobTCG") || strings.Contains(string(body), alice.Email) {
		t.Fatalf("report names a player:\n%s", body)
	}
	if !strings.Contains(string(body), rec.ID(alice.PlayerKey())) {
		t.Fatalf("report lacks the player ID:\n%s", body)
	}
	var got record
	for _, rc := range records(t, buf) {
		if rc["event"] == "import_reported" {
			got = rc
		}
	}
	if got["report"] != id || got["assistant"] != "Claude" || got["player"] != rec.ID(alice.PlayerKey()) {
		t.Fatalf("import_reported record %v", got)
	}
	if strings.Contains(buf.String(), alice.Email) {
		t.Fatal("a record names the email")
	}
	if _, err := r.ReportImport(context.Background(), nil, in); err == nil {
		t.Fatal("report without a token was saved")
	}
}
