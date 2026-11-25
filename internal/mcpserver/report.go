package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
)

// ImportReport is report_import's input: the battle log that failed to
// import, as it was passed to import_log, and the error it gave.
type ImportReport struct {
	Log     string `json:"log" jsonschema:"the battle log exactly as it was passed to import_log"`
	Deck    string `json:"deck,omitempty" jsonschema:"the decklist passed to import_log, if any"`
	OppDeck string `json:"opp_deck,omitempty" jsonschema:"the opponent list passed to import_log, if any"`
	Me      string `json:"me,omitempty" jsonschema:"the player's name, if known, so it is hidden first"`
	Error   string `json:"error" jsonschema:"the error import_log returned, word for word"`
	// Assistant is filled in by the server from the session, never by the
	// caller.
	Assistant string `json:"-"`
}

// reportOffer is added to a failed battle-log import when reports are
// kept: what to ask the player, and the tool that sends it.
const reportOffer = " If you cannot fix this from the error (a missing decklist, the wrong player, text pasted around the log), ask the player in one line whether they will send this battle log to the TCG Replay developer so the import can be fixed: the players' names are replaced before it is stored, only the developer can read it, and it is deleted after 14 days. Only if they say yes, call report_import with the same log, deck and opp_deck, and this error word for word."

// importFailed is import_log's error, with the offer to report it when
// reports are kept and the input was a battle log rather than a record.
func (s *Server) importFailed(in ImportLog, err error) error {
	if s.ReportImport == nil {
		return err
	}
	if _, isRec, perr := game.ParseRecord(in.Log); perr != nil || isRec || strings.TrimSpace(in.Log) == "" {
		return err
	}
	return errors.New(strings.TrimRight(err.Error(), ". ") + "." + reportOffer)
}

// reportDescription is report_import's description.
const reportDescription = "Use this only after import_log failed on a battle log and the player agreed, in this conversation, to send it to the TCG Replay developer: it saves the log, the decklists and the error so the import can be fixed. The players' names are replaced before it is stored; only the developer can read it, and it is deleted after 14 days. Never call it without asking the player first, and never for a game record or a log that imported."

// registerReport adds report_import, offered only when s.ReportImport is
// set. Like guide, it is registered with mcp.AddTool rather than add: it
// needs no game, and a failed import has none to act on.
func (s *Server) registerReport(srv *mcp.Server) {
	t := s.tool("report_import", "Send a failed import to the developer", reportDescription)
	mcp.AddTool(srv, t, func(ctx context.Context, req *mcp.CallToolRequest, in ImportReport) (*mcp.CallToolResult, any, error) {
		var extra *mcp.RequestExtra
		if req != nil {
			extra = req.Extra
			if req.Session != nil {
				if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
					in.Assistant = AssistantName(p.ClientInfo.Name)
				}
			}
		}
		if strings.TrimSpace(in.Log) == "" {
			return nil, nil, errors.New("log is empty: pass the battle log exactly as it was passed to import_log")
		}
		id, err := s.ReportImport(ctx, extra, in)
		if err != nil {
			return nil, nil, err
		}
		return out(fmt.Sprintf("Saved as report %s. Tell the player in one line that it was sent, with that ID, and that it is deleted after 14 days. Do not retry the import unchanged.", id))
	})
}
