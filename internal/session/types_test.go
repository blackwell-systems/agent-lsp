package session

import (
	"errors"
	"testing"
)

func TestIsDirty_False(t *testing.T) {
	s := &SimulationSession{Status: StatusCreated}
	if s.IsDirty() {
		t.Error("expected IsDirty=false for created session")
	}
}

func TestIsDirty_True(t *testing.T) {
	s := &SimulationSession{Status: StatusDirty}
	if !s.IsDirty() {
		t.Error("expected IsDirty=true for dirty session")
	}
}

func TestDirtyError_NotDirty(t *testing.T) {
	s := &SimulationSession{Status: StatusCreated}
	if s.DirtyError() != nil {
		t.Error("expected nil error for non-dirty session")
	}
}

func TestDirtyError_Dirty(t *testing.T) {
	origErr := errors.New("something went wrong")
	s := &SimulationSession{
		Status:   StatusDirty,
		DirtyErr: origErr,
	}
	got := s.DirtyError()
	if got != origErr {
		t.Errorf("got %v, want %v", got, origErr)
	}
}

func TestIsTerminal_NonTerminalStates(t *testing.T) {
	nonTerminal := []SessionStatus{StatusCreated, StatusMutated, StatusEvaluating, StatusEvaluated}
	for _, status := range nonTerminal {
		s := &SimulationSession{Status: status}
		if s.IsTerminal() {
			t.Errorf("expected IsTerminal=false for status %q", status)
		}
	}
}

func TestIsTerminal_TerminalStates(t *testing.T) {
	terminal := []SessionStatus{StatusCommitted, StatusDiscarded, StatusDestroyed, StatusDirty}
	for _, status := range terminal {
		s := &SimulationSession{Status: status}
		if !s.IsTerminal() {
			t.Errorf("expected IsTerminal=true for status %q", status)
		}
	}
}

func TestSetStatus(t *testing.T) {
	s := &SimulationSession{Status: StatusCreated}
	s.SetStatus(StatusMutated)
	if s.Status != StatusMutated {
		t.Errorf("got %q, want %q", s.Status, StatusMutated)
	}
}
