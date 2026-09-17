// Package skill embeds the Agent Skills (agentskills.io) definition for ward
// so the CLI can print it for installation into an agent's skills directory.
package skill

import _ "embed"

// Name is the skill's directory name, which the spec requires to match the
// frontmatter name field.
const Name = "gitward"

// Markdown is the full SKILL.md content, frontmatter included.
//
//go:embed gitward/SKILL.md
var Markdown string
