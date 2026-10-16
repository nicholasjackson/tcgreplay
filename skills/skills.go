// Package skills holds the playmat's Agent Skills (SKILL.md folders) so the
// binary can install the copy that matches its own tools. The folders are
// the source; .claude/skills and .agents/skills link here for this repo.
package skills

import "embed"

// FS holds every tcg-* skill folder.
//
//go:embed tcg-*
var FS embed.FS
