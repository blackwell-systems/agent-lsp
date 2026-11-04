package main

import "errors"

// Tests must never run the real claude CLI: init and uninstall call it to
// edit the user's Claude Code configuration. Tests that need it install
// their own fake.
func init() {
	claudeCLI = func(...string) error { return errors.New("claude CLI disabled in tests") }
}
