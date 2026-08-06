package skills

import "embed"

// FS contains the built-in skills shipped with the easy binary.
//
//go:embed */SKILL.md
var FS embed.FS
