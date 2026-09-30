package application

import (
	"context"
	"testing"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

type fakeRegistry struct {
	records []ports.InstanceRecord
	err     error
}

func (f *fakeRegistry) List(context.Context) ([]ports.InstanceRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.records, nil
}
func (f *fakeRegistry) Get(context.Context, domain.InstanceID) (ports.InstanceRecord, error) {
	return ports.InstanceRecord{}, ports.ErrInstanceUnknown
}
func (f *fakeRegistry) Add(context.Context, ports.InstanceRecord) error { return nil }
func (f *fakeRegistry) Remove(context.Context, domain.InstanceID) error { return nil }

// The multi-instance listing merges the registry with the live state of the
// active instance and marks exactly one row active.
func TestListInstancesMergesRegistryAndLiveState(t *testing.T) {
	h := newHarness(t)
	h.service.deps.Registry = &fakeRegistry{records: []ports.InstanceRecord{
		{ID: "pz_01", TemplateID: "project_zomboid", Name: "PZ"},
		{ID: "valheim_01", TemplateID: "valheim", Name: "Valheim"},
	}}
	h.status.status = ports.ProcessStatus{Running: true, Status: "RUNNING", PID: 42}

	views, err := h.service.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("expected both instances, got %+v", views)
	}
	if views[0].Record.ID != "pz_01" || views[1].Record.ID != "valheim_01" {
		t.Fatalf("rows must be sorted by id: %+v", views)
	}
	var active int
	for _, view := range views {
		if view.Active {
			active++
			if view.Record.ID != "pz_01" {
				t.Fatalf("the harness instance must be the active row: %+v", view)
			}
			if !view.Running || view.Status != "RUNNING" || view.Ports["SERVER_PORT"] != 16261 {
				t.Fatalf("active row must carry live state: %+v", view)
			}
		} else if view.Status != "registered" {
			t.Fatalf("inactive rows report the registry record only: %+v", view)
		}
	}
	if active != 1 {
		t.Fatalf("exactly one row is active, got %d", active)
	}
}

// Without a registry the listing still answers for the active instance, so a
// single-instance deployment keeps working.
func TestListInstancesWithoutRegistry(t *testing.T) {
	h := newHarness(t)
	views, err := h.service.ListInstances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Record.ID != "pz_01" || !views[0].Active {
		t.Fatalf("fallback listing wrong: %+v", views)
	}
}
