package phase

import (
	"fmt"
	"sync"

	"github.com/blackwell-systems/agent-lsp/internal/audit"
	"github.com/blackwell-systems/agent-lsp/internal/logging"
)

// Tracker monitors the active skill and enforces phase-ordered tool permissions.
// All public methods are thread-safe.
type Tracker struct {
	mu           sync.RWMutex
	configs      map[string]*SkillPhaseConfig
	activeSkill  string
	currentPhase int // index into active config's Phases
	mode         EnforcementMode
	toolHistory  []string
	auditLogger  *audit.Logger
}

// NewTracker creates a Tracker pre-loaded with the given skill configs.
// auditLogger may be nil (phase transitions won't be logged).
func NewTracker(configs []*SkillPhaseConfig, auditLogger *audit.Logger) *Tracker {
	m := make(map[string]*SkillPhaseConfig, len(configs))
	for _, c := range configs {
		m[c.SkillName] = c
	}
	return &Tracker{
		configs:     m,
		mode:        ModeWarn,
		auditLogger: auditLogger,
	}
}

// ActivateSkill starts phase enforcement for the named skill.
// Returns an error if the skill is unknown or another skill is already active.
func (t *Tracker) ActivateSkill(skillName string, mode EnforcementMode) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeSkill != "" {
		return fmt.Errorf("skill %q is already active; call deactivate_skill first", t.activeSkill)
	}
	cfg, ok := t.configs[skillName]
	if !ok {
		return fmt.Errorf("unknown skill %q; available skills: %v", skillName, t.skillNames())
	}
	if len(cfg.Phases) == 0 {
		return fmt.Errorf("skill %q has no phases defined", skillName)
	}

	t.activeSkill = skillName
	t.currentPhase = 0
	t.mode = mode
	t.toolHistory = nil

	logging.Log(logging.LevelInfo, fmt.Sprintf("phase.activate: skill=%s mode=%s phase=%s",
		skillName, mode, cfg.Phases[0].Name))

	t.logAuditEvent("activate_skill", skillName, cfg.Phases[0].Name, "")
	return nil
}

// DeactivateSkill stops phase enforcement. Safe to call when no skill is active.
func (t *Tracker) DeactivateSkill() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeSkill == "" {
		return
	}

	logging.Log(logging.LevelInfo, fmt.Sprintf("phase.deactivate: skill=%s tools_called=%d",
		t.activeSkill, len(t.toolHistory)))

	t.logAuditEvent("deactivate_skill", t.activeSkill, "", "")

	t.activeSkill = ""
	t.currentPhase = 0
	t.toolHistory = nil
}

// CheckAndRecord checks whether toolName is permitted under the current phase
// enforcement, records the call, and advances the phase if appropriate.
//
// Returns nil if no skill is active or the call is allowed.
// Returns a *PhaseViolation if the call violates phase rules.
// In ModeWarn, the violation is returned but the call proceeds (caller should log).
// In ModeBlock, the caller should return the violation as an error to the agent.
func (t *Tracker) CheckAndRecord(toolName string) *PhaseViolation {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeSkill == "" {
		return nil
	}

	cfg := t.configs[t.activeSkill]

	// Record the tool call.
	t.toolHistory = append(t.toolHistory, toolName)

	// Check global forbidden first.
	if MatchesAny(cfg.GlobalForbidden, toolName) {
		v := &PhaseViolation{
			ToolName:     toolName,
			SkillName:    t.activeSkill,
			CurrentPhase: cfg.Phases[t.currentPhase].Name,
			Reason:       fmt.Sprintf("%s is globally forbidden during the %s skill", toolName, t.activeSkill),
			Recovery:     "This tool is not used in the " + t.activeSkill + " workflow.",
			Blocked:      t.mode == ModeBlock,
		}
		logging.Log(logging.LevelWarning, fmt.Sprintf("phase.violation: global_forbidden tool=%s skill=%s", toolName, t.activeSkill))
		t.logAuditEvent("phase_violation", t.activeSkill, cfg.Phases[t.currentPhase].Name, "global_forbidden: "+toolName)
		return v
	}

	currentPhaseDef := cfg.Phases[t.currentPhase]

	// Check forbidden in current phase.
	if MatchesAny(currentPhaseDef.Forbidden, toolName) {
		recovery := t.buildRecovery(cfg, toolName)
		v := &PhaseViolation{
			ToolName:     toolName,
			SkillName:    t.activeSkill,
			CurrentPhase: currentPhaseDef.Name,
			Reason:       fmt.Sprintf("%s is forbidden in the %q phase", toolName, currentPhaseDef.Name),
			Recovery:     recovery,
			Blocked:      t.mode == ModeBlock,
		}
		logging.Log(logging.LevelWarning, fmt.Sprintf("phase.violation: forbidden tool=%s skill=%s phase=%s",
			toolName, t.activeSkill, currentPhaseDef.Name))
		t.logAuditEvent("phase_violation", t.activeSkill, currentPhaseDef.Name, "forbidden: "+toolName)
		return v
	}

	// Check if tool is allowed in current phase.
	if MatchesAny(currentPhaseDef.Allowed, toolName) {
		return nil
	}

	// Tool is not in current phase's allowed or forbidden list.
	// Check if it belongs to a later phase (auto-advance).
	for i := t.currentPhase + 1; i < len(cfg.Phases); i++ {
		if MatchesAny(cfg.Phases[i].Allowed, toolName) {
			prevPhase := currentPhaseDef.Name
			t.currentPhase = i
			logging.Log(logging.LevelInfo, fmt.Sprintf("phase.advance: skill=%s %s->%s triggered_by=%s",
				t.activeSkill, prevPhase, cfg.Phases[i].Name, toolName))
			t.logAuditEvent("phase_advance", t.activeSkill, cfg.Phases[i].Name, "from="+prevPhase+" triggered_by="+toolName)
			return nil
		}
	}

	// Tool is not recognized in any phase. Allow it (no enforcement for
	// tools outside the skill's scope, e.g., Read, Grep, Bash).
	return nil
}

// Status returns the current phase enforcement state.
func (t *Tracker) Status() PhaseStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.activeSkill == "" {
		return PhaseStatus{Active: false}
	}

	cfg := t.configs[t.activeSkill]
	phase := cfg.Phases[t.currentPhase]

	historyCopy := make([]string, len(t.toolHistory))
	copy(historyCopy, t.toolHistory)

	// Combine current phase forbidden + global forbidden for the response.
	forbidden := make([]string, 0, len(phase.Forbidden)+len(cfg.GlobalForbidden))
	forbidden = append(forbidden, phase.Forbidden...)
	forbidden = append(forbidden, cfg.GlobalForbidden...)

	return PhaseStatus{
		Active:         true,
		SkillName:      t.activeSkill,
		CurrentPhase:   phase.Name,
		PhaseIndex:     t.currentPhase,
		TotalPhases:    len(cfg.Phases),
		Mode:           t.mode,
		AllowedTools:   phase.Allowed,
		ForbiddenTools: forbidden,
		ToolHistory:    historyCopy,
	}
}

// AvailableSkills returns the names of all registered skills.
func (t *Tracker) AvailableSkills() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.skillNames()
}

// skillNames returns skill names (caller must hold at least RLock).
func (t *Tracker) skillNames() []string {
	names := make([]string, 0, len(t.configs))
	for name := range t.configs {
		names = append(names, name)
	}
	return names
}

// buildRecovery generates recovery guidance when a tool is forbidden in the
// current phase. It looks for the next phase where the tool is allowed
// and suggests which tools the agent should call to advance there.
func (t *Tracker) buildRecovery(cfg *SkillPhaseConfig, toolName string) string {
	// Find the phase where this tool becomes allowed.
	for i := t.currentPhase + 1; i < len(cfg.Phases); i++ {
		if MatchesAny(cfg.Phases[i].Allowed, toolName) {
			// The next phase is the immediate successor.
			nextPhase := cfg.Phases[t.currentPhase+1]
			if i == t.currentPhase+1 {
				return fmt.Sprintf("Complete the %q phase first. Allowed tools: %v",
					nextPhase.Name, nextPhase.Allowed)
			}
			return fmt.Sprintf("%s becomes available in the %q phase. "+
				"Next phase is %q. Allowed tools: %v",
				toolName, cfg.Phases[i].Name, nextPhase.Name, nextPhase.Allowed)
		}
	}
	return fmt.Sprintf("Complete the current %q phase using: %v",
		cfg.Phases[t.currentPhase].Name, cfg.Phases[t.currentPhase].Allowed)
}

// logAuditEvent writes a phase event to the audit trail.
func (t *Tracker) logAuditEvent(event, skill, phase, detail string) {
	if t.auditLogger == nil {
		return
	}
	msg := fmt.Sprintf("%s: skill=%s", event, skill)
	if phase != "" {
		msg += " phase=" + phase
	}
	if detail != "" {
		msg += " " + detail
	}
	t.auditLogger.Log(audit.Record{
		Tool:    event,
		Success: event != "phase_violation",
		EditSummary: &audit.EditSummary{
			Mode:   "phase_enforcement",
			Target: msg,
		},
	})
}
