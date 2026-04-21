package main

// boolPtr returns a pointer to the given bool value.
// Used for ToolAnnotations fields that are *bool (DestructiveHint, OpenWorldHint).
func boolPtr(b bool) *bool { return &b }
