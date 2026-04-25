package phase

// BuiltinSkills returns the phase configs for all skills that ship with
// tool_permissions metadata. These are transcribed from the tool_permissions
// YAML in each skill's SKILL.md frontmatter.
//
// Tool names use the unprefixed form (e.g., "apply_edit" not "mcp__lsp__apply_edit")
// because that is what agent-lsp sees in incoming CallToolRequests. External tool
// names like "Edit", "Write", "Bash" are included in forbidden lists so they
// appear in get_skill_phase output (informational for the agent), but agent-lsp
// cannot enforce them since those tools bypass MCP.
func BuiltinSkills() []*SkillPhaseConfig {
	return []*SkillPhaseConfig{
		skillRename(),
		skillRefactor(),
		skillSafeEdit(),
		skillVerify(),
	}
}

// lsp-rename: 3 phases (prerequisites -> preview -> execute)
func skillRename() *SkillPhaseConfig {
	return &SkillPhaseConfig{
		SkillName: "lsp-rename",
		Phases: []PhaseDefinition{
			{
				Name:        "prerequisites",
				Description: "Initialize LSP if needed",
				Allowed:     []string{"start_lsp"},
				Forbidden:   nil,
			},
			{
				Name:        "preview",
				Description: "Locate symbol, validate rename, enumerate references, dry-run",
				Allowed: []string{
					"go_to_symbol",
					"prepare_rename",
					"get_references",
					"rename_symbol", // dry_run=true only (arg-level enforcement is future work)
				},
				Forbidden: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
			},
			{
				Name:        "execute",
				Description: "Capture pre-rename diagnostics, execute rename, apply, verify",
				Allowed: []string{
					"get_diagnostics",
					"rename_symbol", // dry_run=false
					"apply_edit",
				},
				Forbidden: []string{
					"simulate_*",
					"run_build",
				},
			},
		},
		GlobalForbidden: []string{
			"format_document", // rename does not format
			"run_tests",       // rename does not run tests
		},
	}
}

// lsp-refactor: 5 phases (blast_radius -> speculative_preview -> apply -> build_verification -> test_execution)
func skillRefactor() *SkillPhaseConfig {
	return &SkillPhaseConfig{
		SkillName: "lsp-refactor",
		Phases: []PhaseDefinition{
			{
				Name:        "blast_radius",
				Description: "Phase 1: analyze impact before any edits",
				Allowed: []string{
					"get_change_impact",
					"go_to_symbol",
					"get_references",
				},
				Forbidden: []string{
					"apply_edit",
					"simulate_*",
					"Edit",
					"Write",
				},
			},
			{
				Name:        "speculative_preview",
				Description: "Phase 2: simulate edits in memory, compare diagnostics",
				Allowed: []string{
					"open_document",
					"get_diagnostics",
					"simulate_edit_atomic",
					"simulate_chain",
				},
				Forbidden: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
			},
			{
				Name:        "apply",
				Description: "Phase 3: write changes to disk and format",
				Allowed: []string{
					"apply_edit",
					"format_document",
					"Edit",
					"Write",
				},
				Forbidden: []string{
					"simulate_*",
					"rename_symbol",
				},
			},
			{
				Name:        "build_verification",
				Description: "Phase 4: check diagnostics and run the build",
				Allowed: []string{
					"get_diagnostics",
					"run_build",
				},
				Forbidden: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
			},
			{
				Name:        "test_execution",
				Description: "Phase 5: find and run affected tests",
				Allowed: []string{
					"get_tests_for_file",
					"run_tests",
				},
				Forbidden: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
			},
		},
		GlobalForbidden: []string{
			"rename_symbol", // refactor uses edit, not rename
		},
	}
}

// lsp-safe-edit: 4 phases (setup -> speculative_preview -> apply -> verify_and_fix)
func skillSafeEdit() *SkillPhaseConfig {
	return &SkillPhaseConfig{
		SkillName: "lsp-safe-edit",
		Phases: []PhaseDefinition{
			{
				Name:        "setup",
				Description: "Open files and capture baseline diagnostics",
				Allowed: []string{
					"start_lsp",
					"open_document",
					"get_diagnostics",
				},
				Forbidden: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
			},
			{
				Name:        "speculative_preview",
				Description: "Simulate the edit in memory before touching disk",
				Allowed: []string{
					"simulate_edit_atomic",
					"simulate_chain",
				},
				Forbidden: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
			},
			{
				Name:        "apply",
				Description: "Write the change to disk",
				Allowed: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
				Forbidden: []string{
					"simulate_*",
				},
			},
			{
				Name:        "verify_and_fix",
				Description: "Collect post-edit diagnostics, surface code actions, format",
				Allowed: []string{
					"get_diagnostics",
					"get_code_actions",
					"apply_edit", // for applying code action fixes
					"format_document",
				},
				Forbidden: []string{
					"simulate_*",
					"run_build",
					"run_tests",
				},
			},
		},
		GlobalForbidden: []string{
			"rename_symbol",     // safe-edit uses direct edits
			"get_change_impact", // blast radius is lsp-impact's job
		},
	}
}

// lsp-verify: 5 phases (test_correlation -> diagnostics -> build -> tests -> fix_and_format)
func skillVerify() *SkillPhaseConfig {
	return &SkillPhaseConfig{
		SkillName: "lsp-verify",
		Phases: []PhaseDefinition{
			{
				Name:        "test_correlation",
				Description: "Pre-step: map changed source files to their test files",
				Allowed: []string{
					"get_tests_for_file",
				},
				Forbidden: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
			},
			{
				Name:        "diagnostics",
				Description: "Layer 1: collect LSP diagnostics for changed files",
				Allowed: []string{
					"start_lsp",
					"get_diagnostics",
				},
				Forbidden: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
			},
			{
				Name:        "build",
				Description: "Layer 2: run compiler build",
				Allowed: []string{
					"run_build",
				},
				Forbidden: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
			},
			{
				Name:        "tests",
				Description: "Layer 3: run test suite",
				Allowed: []string{
					"run_tests",
					"Bash", // scoped test commands for large repos
				},
				Forbidden: []string{
					"apply_edit",
					"Edit",
					"Write",
				},
			},
			{
				Name:        "fix_and_format",
				Description: "Post-verification: apply code action fixes and format",
				Allowed: []string{
					"get_code_actions",
					"apply_edit",
					"format_document",
					"get_diagnostics", // re-check after fixes
				},
				Forbidden: []string{
					"simulate_*",
					"run_build",  // re-run full verify instead
					"run_tests",  // re-run full verify instead
				},
			},
		},
		GlobalForbidden: []string{
			"simulate_*",    // verify is post-edit, not speculative
			"rename_symbol", // verify does not make semantic changes
		},
	}
}
