package phase

import "testing"

// testSkill creates a minimal skill config for testing.
func testSkill() *SkillPhaseConfig {
	return &SkillPhaseConfig{
		SkillName: "test-skill",
		Phases: []PhaseDefinition{
			{
				Name:      "phase1",
				Allowed:   []string{"tool_a", "tool_b"},
				Forbidden: []string{"tool_x"},
			},
			{
				Name:      "phase2",
				Allowed:   []string{"tool_c", "tool_d"},
				Forbidden: []string{"tool_a"},
			},
			{
				Name:      "phase3",
				Allowed:   []string{"tool_e", "simulate_*"},
				Forbidden: []string{"tool_a", "tool_c"},
			},
		},
		GlobalForbidden: []string{"tool_forbidden"},
	}
}

func TestTracker_NoActiveSkill(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)

	// No skill active: all calls should be allowed.
	v := tracker.CheckAndRecord("tool_a")
	if v != nil {
		t.Fatalf("expected nil violation with no active skill, got %+v", v)
	}

	status := tracker.Status()
	if status.Active {
		t.Fatal("expected status.Active to be false")
	}
}

func TestTracker_ActivateDeactivate(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)

	// Activate unknown skill.
	if err := tracker.ActivateSkill("nonexistent", ModeWarn); err == nil {
		t.Fatal("expected error for unknown skill")
	}

	// Activate valid skill.
	if err := tracker.ActivateSkill("test-skill", ModeWarn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := tracker.Status()
	if !status.Active {
		t.Fatal("expected active after activate")
	}
	if status.SkillName != "test-skill" {
		t.Fatalf("expected skill name test-skill, got %s", status.SkillName)
	}
	if status.CurrentPhase != "phase1" {
		t.Fatalf("expected phase1, got %s", status.CurrentPhase)
	}

	// Double activate should error.
	if err := tracker.ActivateSkill("test-skill", ModeWarn); err == nil {
		t.Fatal("expected error for double activate")
	}

	// Deactivate.
	tracker.DeactivateSkill()
	status = tracker.Status()
	if status.Active {
		t.Fatal("expected inactive after deactivate")
	}

	// Deactivate again is safe.
	tracker.DeactivateSkill()
}

func TestTracker_AllowedInCurrentPhase(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	tracker.ActivateSkill("test-skill", ModeBlock)

	// tool_a is allowed in phase1.
	v := tracker.CheckAndRecord("tool_a")
	if v != nil {
		t.Fatalf("expected tool_a to be allowed in phase1, got violation: %s", v.Reason)
	}
}

func TestTracker_ForbiddenInCurrentPhase(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	tracker.ActivateSkill("test-skill", ModeBlock)

	// tool_x is forbidden in phase1.
	v := tracker.CheckAndRecord("tool_x")
	if v == nil {
		t.Fatal("expected violation for forbidden tool_x in phase1")
	}
	if !v.Blocked {
		t.Fatal("expected Blocked=true in ModeBlock")
	}
	if v.CurrentPhase != "phase1" {
		t.Fatalf("expected current phase phase1, got %s", v.CurrentPhase)
	}
}

func TestTracker_GlobalForbidden(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	tracker.ActivateSkill("test-skill", ModeBlock)

	v := tracker.CheckAndRecord("tool_forbidden")
	if v == nil {
		t.Fatal("expected violation for globally forbidden tool")
	}
	if !v.Blocked {
		t.Fatal("expected Blocked=true")
	}
}

func TestTracker_WarnMode(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	tracker.ActivateSkill("test-skill", ModeWarn)

	// tool_x is forbidden in phase1 but warn mode should not block.
	v := tracker.CheckAndRecord("tool_x")
	if v == nil {
		t.Fatal("expected a violation to be returned")
	}
	if v.Blocked {
		t.Fatal("expected Blocked=false in ModeWarn")
	}
}

func TestTracker_PhaseAdvance(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	tracker.ActivateSkill("test-skill", ModeBlock)

	// Start in phase1.
	if s := tracker.Status(); s.CurrentPhase != "phase1" {
		t.Fatalf("expected phase1, got %s", s.CurrentPhase)
	}

	// Call tool_c (allowed in phase2) -> should auto-advance to phase2.
	v := tracker.CheckAndRecord("tool_c")
	if v != nil {
		t.Fatalf("expected auto-advance, got violation: %s", v.Reason)
	}
	if s := tracker.Status(); s.CurrentPhase != "phase2" {
		t.Fatalf("expected phase2 after advance, got %s", s.CurrentPhase)
	}

	// Call tool_e (allowed in phase3) -> should advance to phase3.
	v = tracker.CheckAndRecord("tool_e")
	if v != nil {
		t.Fatalf("expected auto-advance to phase3, got violation: %s", v.Reason)
	}
	if s := tracker.Status(); s.CurrentPhase != "phase3" {
		t.Fatalf("expected phase3, got %s", s.CurrentPhase)
	}
}

func TestTracker_WildcardAdvance(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	tracker.ActivateSkill("test-skill", ModeBlock)

	// simulate_edit matches "simulate_*" in phase3 -> should advance through.
	v := tracker.CheckAndRecord("simulate_edit")
	if v != nil {
		t.Fatalf("expected wildcard advance, got violation: %s", v.Reason)
	}
	if s := tracker.Status(); s.CurrentPhase != "phase3" {
		t.Fatalf("expected phase3 via wildcard, got %s", s.CurrentPhase)
	}
}

func TestTracker_UnrecognizedToolAllowed(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	tracker.ActivateSkill("test-skill", ModeBlock)

	// "unknown_tool" is not in any phase's allowed or forbidden list.
	// It should be allowed (pass-through for tools outside the skill's scope).
	v := tracker.CheckAndRecord("unknown_tool")
	if v != nil {
		t.Fatalf("expected unrecognized tool to be allowed, got violation: %s", v.Reason)
	}
}

func TestTracker_ToolHistory(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	tracker.ActivateSkill("test-skill", ModeWarn)

	tracker.CheckAndRecord("tool_a")
	tracker.CheckAndRecord("tool_b")
	tracker.CheckAndRecord("tool_x") // forbidden but warn mode

	status := tracker.Status()
	if len(status.ToolHistory) != 3 {
		t.Fatalf("expected 3 tool history entries, got %d", len(status.ToolHistory))
	}
}

func TestTracker_AvailableSkills(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	skills := tracker.AvailableSkills()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0] != "test-skill" {
		t.Fatalf("expected test-skill, got %s", skills[0])
	}
}

func TestTracker_ForbiddenAfterAdvance(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	tracker.ActivateSkill("test-skill", ModeBlock)

	// Advance to phase2 by calling tool_c.
	tracker.CheckAndRecord("tool_c")
	if s := tracker.Status(); s.CurrentPhase != "phase2" {
		t.Fatalf("expected phase2, got %s", s.CurrentPhase)
	}

	// tool_a is now forbidden in phase2.
	v := tracker.CheckAndRecord("tool_a")
	if v == nil {
		t.Fatal("expected tool_a to be forbidden in phase2")
	}
}

func TestTracker_Recovery(t *testing.T) {
	tracker := NewTracker([]*SkillPhaseConfig{testSkill()}, nil)
	tracker.ActivateSkill("test-skill", ModeBlock)

	// tool_x is forbidden in phase1; check that recovery guidance is set.
	v := tracker.CheckAndRecord("tool_x")
	if v == nil {
		t.Fatal("expected violation")
	}
	if v.Recovery == "" {
		t.Fatal("expected non-empty recovery guidance")
	}
}

func TestBuiltinSkills_Loaded(t *testing.T) {
	skills := BuiltinSkills()
	if len(skills) != 4 {
		t.Fatalf("expected 4 builtin skills, got %d", len(skills))
	}

	names := map[string]bool{}
	for _, s := range skills {
		names[s.SkillName] = true
		if len(s.Phases) == 0 {
			t.Fatalf("skill %s has no phases", s.SkillName)
		}
	}

	for _, expected := range []string{"lsp-rename", "lsp-refactor", "lsp-safe-edit", "lsp-verify"} {
		if !names[expected] {
			t.Fatalf("missing expected skill: %s", expected)
		}
	}
}

// TestRenameSkill_FullWorkflow simulates a complete lsp-rename workflow.
func TestRenameSkill_FullWorkflow(t *testing.T) {
	tracker := NewTracker(BuiltinSkills(), nil)
	tracker.ActivateSkill("lsp-rename", ModeBlock)

	// Phase: prerequisites
	assertAllowed(t, tracker, "start_lsp")

	// Phase: preview (auto-advance from prerequisites)
	assertAllowed(t, tracker, "go_to_symbol")
	if s := tracker.Status(); s.CurrentPhase != "preview" {
		t.Fatalf("expected preview phase, got %s", s.CurrentPhase)
	}
	assertAllowed(t, tracker, "prepare_rename")
	assertAllowed(t, tracker, "find_references")
	assertAllowed(t, tracker, "rename_symbol") // dry_run=true

	// apply_edit should be forbidden in preview.
	assertForbidden(t, tracker, "apply_edit")

	// Phase: execute (auto-advance by calling get_diagnostics)
	assertAllowed(t, tracker, "get_diagnostics")
	if s := tracker.Status(); s.CurrentPhase != "execute" {
		t.Fatalf("expected execute phase, got %s", s.CurrentPhase)
	}
	assertAllowed(t, tracker, "apply_edit")

	// Global forbidden: format_document, run_tests.
	assertForbidden(t, tracker, "format_document")
	assertForbidden(t, tracker, "run_tests")

	tracker.DeactivateSkill()
}

// TestRefactorSkill_ApplyBlockedInBlastRadius verifies the key safety property:
// edits are blocked during blast_radius phase.
func TestRefactorSkill_ApplyBlockedInBlastRadius(t *testing.T) {
	tracker := NewTracker(BuiltinSkills(), nil)
	tracker.ActivateSkill("lsp-refactor", ModeBlock)

	// Should start in blast_radius.
	if s := tracker.Status(); s.CurrentPhase != "blast_radius" {
		t.Fatalf("expected blast_radius, got %s", s.CurrentPhase)
	}

	// apply_edit forbidden in blast_radius.
	assertForbidden(t, tracker, "apply_edit")

	// preview_edit forbidden in blast_radius.
	assertForbidden(t, tracker, "preview_edit")

	// blast_radius allowed.
	assertAllowed(t, tracker, "blast_radius")

	tracker.DeactivateSkill()
}

func assertAllowed(t *testing.T, tracker *Tracker, tool string) {
	t.Helper()
	v := tracker.CheckAndRecord(tool)
	if v != nil && v.Blocked {
		t.Fatalf("expected %s to be allowed, got blocked: %s", tool, v.Reason)
	}
}

func assertForbidden(t *testing.T, tracker *Tracker, tool string) {
	t.Helper()
	v := tracker.CheckAndRecord(tool)
	if v == nil {
		t.Fatalf("expected %s to be forbidden, got nil violation", tool)
	}
	if !v.Blocked {
		t.Fatalf("expected %s to be blocked (ModeBlock), but Blocked=false", tool)
	}
}
