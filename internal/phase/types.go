package phase

// EnforcementMode controls how phase violations are handled.
type EnforcementMode string

const (
	// ModeWarn logs the violation but allows the tool call to proceed.
	ModeWarn EnforcementMode = "warn"
	// ModeBlock returns an error to the agent with recovery guidance.
	ModeBlock EnforcementMode = "block"
)

// PhaseDefinition describes a single phase within a skill's workflow.
type PhaseDefinition struct {
	// Name is the phase identifier (e.g., "preview", "execute").
	Name string
	// Description is a human-readable summary of what happens in this phase.
	Description string
	// Allowed lists glob patterns for tools permitted in this phase.
	Allowed []string
	// Forbidden lists glob patterns for tools explicitly blocked in this phase.
	Forbidden []string
}

// SkillPhaseConfig describes the complete phase enforcement rules for one skill.
type SkillPhaseConfig struct {
	// SkillName matches the skill's name field in SKILL.md (e.g., "lsp-rename").
	SkillName string
	// Phases is an ordered list of phases. The agent starts in Phases[0]
	// and advances forward as tool calls match later phases.
	Phases []PhaseDefinition
	// GlobalForbidden lists glob patterns for tools that are always blocked
	// regardless of the current phase.
	GlobalForbidden []string
}

// PhaseViolation describes why a tool call was denied or warned.
type PhaseViolation struct {
	// ToolName is the tool that triggered the violation.
	ToolName string
	// SkillName is the active skill.
	SkillName string
	// CurrentPhase is the name of the phase the agent is currently in.
	CurrentPhase string
	// Reason explains why the tool call is not allowed.
	Reason string
	// Recovery suggests what the agent should do to proceed.
	Recovery string
	// Blocked is true when the tool call was denied (ModeBlock).
	// False when it was allowed with a warning (ModeWarn).
	Blocked bool
}

// PhaseStatus represents the current state of skill phase enforcement.
type PhaseStatus struct {
	// Active is true when a skill is currently being enforced.
	Active bool `json:"active"`
	// SkillName is the name of the active skill, empty if none.
	SkillName string `json:"skill_name,omitempty"`
	// CurrentPhase is the name of the current phase.
	CurrentPhase string `json:"current_phase,omitempty"`
	// PhaseIndex is the zero-based index of the current phase.
	PhaseIndex int `json:"phase_index,omitempty"`
	// TotalPhases is the total number of phases in the active skill.
	TotalPhases int `json:"total_phases,omitempty"`
	// Mode is the enforcement mode (warn or block).
	Mode EnforcementMode `json:"mode,omitempty"`
	// AllowedTools lists the tools allowed in the current phase.
	AllowedTools []string `json:"allowed_tools,omitempty"`
	// ForbiddenTools lists the tools forbidden in the current phase.
	ForbiddenTools []string `json:"forbidden_tools,omitempty"`
	// ToolHistory lists tool calls made since skill activation.
	ToolHistory []string `json:"tool_history,omitempty"`
}
