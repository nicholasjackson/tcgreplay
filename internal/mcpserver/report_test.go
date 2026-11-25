package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const brokenLog = "Turn # 1 - Ann's Turn\nAnn drew a card.\n"

// TestReportImport: with reports kept, a failed battle-log import offers
// to send it, and report_import hands the log, lists, error and the
// session's assistant to the hook and returns the report's ID. A record
// that fails is not offered.
func TestReportImport(t *testing.T) {
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			var got ImportReport
			s := &Server{Version: "test", ReportImport: func(_ context.Context, _ *mcp.RequestExtra, r ImportReport) (string, error) {
				got = r
				return "ir-20261125-0123abcd", nil
			}}
			cs, _ := connectServer(t, tr, s)
			out, isErr := call(t, cs, "import_log", map[string]any{"log": brokenLog, "deck": "4 Riolu"})
			if !isErr || !strings.Contains(out, "Setup section. If you cannot fix this") || !strings.Contains(out, "call report_import") {
				t.Fatalf("failed import: error %v: %s", isErr, out)
			}
			out, isErr = call(t, cs, "report_import", map[string]any{"log": brokenLog, "deck": "4 Riolu", "error": "line 1: before the Setup section"})
			if isErr || !strings.Contains(out, "ir-20261125-0123abcd") {
				t.Fatalf("report_import: error %v: %s", isErr, out)
			}
			if got.Log != brokenLog || got.Deck != "4 Riolu" || got.Error != "line 1: before the Setup section" || got.Assistant != "Codex" {
				t.Fatalf("hook got %+v", got)
			}
			if out, isErr := call(t, cs, "report_import", map[string]any{"log": " ", "error": "x"}); !isErr {
				t.Fatalf("report_import with no log: %s", out)
			}
			// A record is the player's own saved game, never offered.
			out, isErr = call(t, cs, "import_log", map[string]any{"log": `{"tcgreplay_record": 1, "log": "x"}`})
			if !isErr || strings.Contains(out, "report_import") {
				t.Fatalf("failed record import: error %v: %s", isErr, out)
			}
		})
	}
}

// TestNoReportOfferWithoutHook: the desktop neither offers nor lists
// report_import.
func TestNoReportOfferWithoutHook(t *testing.T) {
	cs, _ := connect(t, transports[0])
	out, isErr := call(t, cs, "import_log", map[string]any{"log": brokenLog, "deck": "4 Riolu"})
	if !isErr || strings.Contains(out, "report_import") {
		t.Fatalf("failed import on the desktop: error %v: %s", isErr, out)
	}
	for tool, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if tool.Name == "report_import" {
			t.Fatal("the desktop lists report_import")
		}
	}
}
