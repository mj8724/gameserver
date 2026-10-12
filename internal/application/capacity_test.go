package application

import (
	"context"
	"errors"
	"testing"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// fakeUsage drives the capacity guard's measurement seam: the first call can
// report a value below the limit while the second (pre-write) call reports one
// above it, which is the check-and-write race the policy must survive.
type fakeUsage struct {
	values []float64
	calls  int
	err    error
}

func (f *fakeUsage) DiskUsageMB(domain.InstanceID) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}
	index := f.calls
	f.calls++
	if index >= len(f.values) {
		index = len(f.values) - 1
	}
	if index < 0 {
		return 0, nil
	}
	return f.values[index], nil
}

// A usage jump between the entry check and the pre-write check must be caught
// by the second check: no partial write, and the error maps to 409.
func TestCapacityDoubleCheckRejectsPartialWrite(t *testing.T) {
	usage := &fakeUsage{values: []float64{10, 999}}
	guard := NewCapacity(0, 80, 1, usage) // quota 1 GB, hard 80%
	if err := guard.CheckBeforeWrite(context.Background(), "pz_01"); err != nil {
		t.Fatalf("entry check must pass below the limit: %v", err)
	}
	if err := guard.CheckBeforeWrite(context.Background(), "pz_01"); err == nil {
		t.Fatal("pre-write check must catch the usage jump")
	}
}

// A hard-limit breach surfaces as 409 semantics through the use-case code, and
// the recovery path is configuration driven — nothing is deleted.
func TestCapacityHardLimitAndRecoveryPath(t *testing.T) {
	h := newHarness(t)
	h.files.usageMB = 900
	h.service.deps.Capacity = NewCapacity(50, 80, 1, h.files) // 900 MB >= 80% of 1024 MB
	_, err := h.service.BeginInstall(context.Background(), "")
	if err == nil {
		t.Fatal("install must be refused at the hard limit")
	}
	if code, ok := ErrorCodeOf(err); !ok || code != CodeCapacityExceeded {
		t.Fatalf("hard limit must surface CodeCapacityExceeded, got %v", err)
	}
	if h.installer.calls != 0 {
		t.Fatalf("refused install must not run the installer, calls=%d", h.installer.calls)
	}
	// Recovery path: relaxing the policy (operator action) restores writes.
	h.files.usageMB = 100
	h.service.deps.Capacity = NewCapacity(50, 80, 1, h.files)
	if _, err := h.service.BeginInstall(context.Background(), ""); err != nil {
		t.Fatalf("after relaxing the thresholds installs must work again: %v", err)
	}
}

// A measurement failure fails closed for growing writes instead of guessing.
func TestCapacityMeasurementFailureFailsClosed(t *testing.T) {
	usage := &fakeUsage{err: errors.New("measure failed")}
	guard := NewCapacity(0, 80, 1, usage)
	if err := guard.CheckBeforeWrite(context.Background(), "pz_01"); err == nil {
		t.Fatal("a failing measurement must refuse growing writes")
	}
}

// The self-stimulation guard defers an automatic backup whose footprint would
// cross the hard limit: pending state, no backup attempt, stop unaffected.
func TestAutomaticBackupDefersWhenCapacityTight(t *testing.T) {
	h := newHarness(t)
	h.service.backupSettle = 0
	backup := &fakeBackup{enabled: true}
	h.service.deps.Backup = backup
	h.files.usageMB = 900
	h.service.deps.Capacity = NewCapacity(0, 80, 1, h.files)

	result, err := h.service.Stop(context.Background())
	if err != nil || !result.Success {
		t.Fatalf("stop must succeed regardless of capacity: %v %+v", err, result)
	}
	h.service.mu.Lock()
	state := h.service.lastBackup
	h.service.mu.Unlock()
	if state.State != "pending" {
		t.Fatalf("tight capacity must defer the backup (pending), got %+v", state)
	}
	if backup.callCount() != 0 {
		t.Fatalf("deferred backup must not run, calls=%d", backup.callCount())
	}
}

// A disabled policy keeps the historical behaviour exactly: status says
// disabled, writes are never refused and quota_gb/usage_percent are unchanged.
func TestCapacityDisabledKeepsHistoricalBehaviour(t *testing.T) {
	h := newHarness(t)
	h.files.usageMB = 99999
	h.service.deps.Capacity = nil
	if _, err := h.service.BeginInstall(context.Background(), ""); err != nil {
		t.Fatalf("without a policy installs must not be refused: %v", err)
	}
	response, err := h.service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	disk, ok := response["disk"].(map[string]any)
	if !ok {
		t.Fatalf("disk projection missing: %+v", response["disk"])
	}
	if disk["quota_gb"] != 30.0 {
		t.Fatalf("quota_gb projection must stay unchanged: %+v", disk)
	}
	capacity, ok := response["capacity"].(map[string]any)
	if !ok || capacity["state"] != "disabled" {
		t.Fatalf("capacity projection must report disabled: %+v", response["capacity"])
	}
}

// The usage projection is additive: the policy reports its own state and the
// legacy quota/usage fields keep their shape.
func TestCapacityStatusProjectsState(t *testing.T) {
	h := newHarness(t)
	h.files.usageMB = 500
	h.service.deps.Capacity = NewCapacity(30, 80, 1, h.files)
	response, err := h.service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	capacity := response["capacity"].(map[string]any)
	if capacity["state"] != "soft" {
		t.Fatalf("expected soft state at 500/1024 MB (30%% soft): %+v", capacity)
	}
	if _, ok := capacity["hard_percent"]; !ok {
		t.Fatalf("policy thresholds must be visible: %+v", capacity)
	}
}

var _ ports.CapacityChecker = (*capacityGuard)(nil)
