package domain

import "testing"

func TestInstanceStateKeepsItsIdentifier(t *testing.T) {
	state := InstanceState{ID: InstanceID("pz_01")}
	if got, want := state.ID, InstanceID("pz_01"); got != want {
		t.Fatalf("state.ID = %q, want %q", got, want)
	}
}
