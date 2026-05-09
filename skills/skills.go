// Package skills embeds all SKILL.md files for use at runtime.
//
// This package exists at the skills/ directory level so that //go:embed
// can access the SKILL.md files (Go's embed directive can only reference
// files at or below the package directory).
package skills

import "embed"

//go:embed */SKILL.md
var Files embed.FS
