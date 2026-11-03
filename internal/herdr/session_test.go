package herdr

import "testing"

func TestIDForNamesOnlyASessionsOwnID(t *testing.T) {
	t.Parallel()

	ref := &AgentSessionInfo{Agent: "claude", Kind: SessionRefID, Value: "s-1"}

	if _, ok := (*AgentSessionInfo)(nil).IDFor("claude"); ok {
		t.Error("a nil reference named a session")
	}

	if _, ok := ref.IDFor("codex"); ok {
		t.Error("a reference named another agent's session")
	}

	other := &AgentSessionInfo{Agent: "claude", Kind: "name", Value: "s-1"}
	if _, ok := other.IDFor("claude"); ok {
		t.Error("a reference that is not by id named a session")
	}

	if id, ok := ref.IDFor("claude"); !ok || id != "s-1" {
		t.Errorf("IDFor = %q, %v; want s-1, true", id, ok)
	}
}
