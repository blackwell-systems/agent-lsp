// Package phase implements skill-level phase enforcement for agent-lsp.
//
// When an agent activates a skill, the phase tracker monitors incoming tool
// calls and enforces ordering constraints defined in the skill's
// tool_permissions metadata. Phases advance automatically based on tool call
// patterns; violations are either warned or blocked depending on the
// enforcement mode.
package phase
