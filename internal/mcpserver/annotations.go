package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

// readOnlyTools never change a game: they read the board, its log, its
// record, card text or a guide, or wait for the player. search_deck and
// look_top only add to the hidden-information audit. Clients such as
// ChatGPT ask the user before any tool not marked read-only, so these are
// marked to spare the user a prompt for every look at the board.
var readOnlyTools = map[string]bool{
	"ping":          true,
	"get_record":    true,
	"get_state":     true,
	"get_log":       true,
	"get_warnings":  true,
	"card_text":     true,
	"wait_for_turn": true,
	"search_deck":   true,
	"look_top":      true,
	"list_branches": true,
	"get_profile":   true,
	"guide":         true,
}

// replacingTools replace the account's current game, losing anything not
// kept in a record. Every other write can be undone or rewound on the
// playmat, so it is not destructive.
var replacingTools = map[string]bool{
	"new_game":          true,
	"import_log":        true,
	"set_opponent_deck": true,
}

// annotations are the MCP behaviour hints for the tool called name. No tool
// reaches beyond the playmat, so none is open-world.
func annotations(name string) *mcp.ToolAnnotations {
	closed := false
	if readOnlyTools[name] {
		return &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closed}
	}
	destructive := replacingTools[name]
	return &mcp.ToolAnnotations{DestructiveHint: &destructive, OpenWorldHint: &closed}
}
