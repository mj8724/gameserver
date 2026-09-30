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
func (f *fakeRegistry) Add(_ context.Context, record ports.InstanceRecord) error {
	for _, existing := range f.records {
		if existing.ID == record.ID {
			return ports.ErrInstanceExists
		}
	}
	f.records = append(f.records, record)
	return nil
}
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

// Creating an instance allocates a port pair and registers it; duplicates and
// unknown templates are refused without touching existing data.
func TestCreateInstanceAllocatesPortsAndRefusesDuplicates(t *testing.T) {
	h := newHarness(t)
	registry := &fakeRegistry{}
	h.service.deps.Registry = registry
	h.service.deps.Ports = &fakePorts{}

	view, err := h.service.CreateInstance(context.Background(), "valheim_01", "project_zomboid")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if view.Record.ID != "valheim_01" || view.Ports["SERVER_PORT"] == 0 {
		t.Fatalf("allocation/registration incomplete: %+v", view)
	}
	if _, err := h.service.CreateInstance(context.Background(), "valheim_01", "project_zomboid"); err == nil {
		t.Fatal("duplicate instance must be refused")
	}
	if _, err := h.service.CreateInstance(context.Background(), "x_02", "unknown_template"); err == nil {
		t.Fatal("unknown template must be refused")
	}
}

// The active instance cannot unregister itself; other ids round-trip.
func TestRemoveInstanceRefusesActive(t *testing.T) {
	h := newHarness(t)
	h.service.deps.Registry = &fakeRegistry{}
	if err := h.service.RemoveInstance(context.Background(), "pz_01"); err == nil {
		t.Fatal("the active instance must not be removable")
	}
}

type fakePorts struct{}

func (fakePorts) Allocate(_ context.Context, primaryKey, directKey string) (map[string]int, error) {
	return map[string]int{primaryKey: 27015, directKey: 27016}, nil
}
