package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// fakeIntents is an in-memory ports.IntentLog with an observation hook.
type fakeIntents struct {
	mu       sync.Mutex
	intent   ports.InstallIntent
	present  bool
	writes   []ports.InstallIntent
	cleared  int
	loadErr  error
	writeErr error
}

func (f *fakeIntents) Load(context.Context, domain.InstanceID) (ports.InstallIntent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loadErr != nil {
		return ports.InstallIntent{}, f.loadErr
	}
	if !f.present {
		return ports.InstallIntent{}, ports.ErrIntentAbsent
	}
	return f.intent, nil
}

func (f *fakeIntents) Write(_ context.Context, _ domain.InstanceID, intent ports.InstallIntent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeErr != nil {
		return f.writeErr
	}
	f.writes = append(f.writes, intent)
	f.intent = intent
	f.present = true
	return nil
}

func (f *fakeIntents) Clear(context.Context, domain.InstanceID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleared++
	f.present = false
	return nil
}

func (f *fakeIntents) snapshot() []ports.InstallIntent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ports.InstallIntent(nil), f.writes...)
}

// A phase history is persisted from REQUESTED through DONE so a restart can see
// what was in flight instead of silently starting from IDLE.
func TestTaskIntentLogPersistsPhases(t *testing.T) {
	h := newHarness(t)
	intents := &fakeIntents{}
	h.service.deps.Intents = intents
	accepted, err := h.service.BeginInstall(context.Background(), "")
	if err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	if accepted.Status == "" {
		t.Fatalf("install must be accepted: %+v", accepted)
	}
	h.installer.wait(t)
	writes := intents.snapshot()
	if len(writes) < 2 || writes[0].Phase != ports.PhaseRequested || writes[1].Phase != ports.PhaseRunning {
		t.Fatalf("expected REQUESTED then RUNNING, got %+v", writes)
	}
	for _, write := range writes {
		if write.UpdatedAt == "" {
			t.Fatalf("intent must record a timestamp: %+v", write)
		}
	}
	phases := map[string]bool{}
	for _, write := range writes {
		phases[write.Phase] = true
	}
	if !phases[ports.PhaseRunning] {
		t.Fatalf("RUNNING phase missing: %+v", writes)
	}
}

// A crash-interrupted install whose content did commit must fold into DONE
// without invoking the installer again (no re-download).
func TestTaskReconcileAfterCrashAvoidsRedownload(t *testing.T) {
	h := newHarness(t)
	intents := &fakeIntents{present: true, intent: ports.InstallIntent{
		Phase:       ports.PhaseRunning,
		Branch:      "public",
		ManifestSHA: "before",
		TotalBytes:  512,
		StartedAt:   "2026-09-29T00:00:00Z",
	}}
	h.service.deps.Intents = intents
	h.files.installed = true
	h.files.fingerprint = ports.ArtifactFingerprint{BuildID: "25485538", ManifestSHA: "after", ManifestName: "appmanifest_380870.acf", TotalBytes: 2048}

	if err := h.service.reconcileInstallIntent(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	status, err := h.service.InstallState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "COMPLETED" || status.Progress != 100 {
		t.Fatalf("interrupted-but-committed install must reconcile to COMPLETED: %+v", status)
	}
	if h.installer.calls != 0 {
		t.Fatalf("reconciliation must not re-run the installer, calls=%d", h.installer.calls)
	}
	if intents.cleared == 0 {
		t.Fatal("terminal intent record must be cleared after folding into COMPLETED")
	}
}

// An interrupted install without committed content reconciles to FAILED so the
// operator retries explicitly instead of a silent re-download.
func TestTaskReconcileWithoutContentFailsClosed(t *testing.T) {
	h := newHarness(t)
	intents := &fakeIntents{present: true, intent: ports.InstallIntent{
		Phase: ports.PhaseRequested, Branch: "public",
	}}
	h.service.deps.Intents = intents
	h.files.fingerprint = ports.ArtifactFingerprint{ManifestSHA: "before", TotalBytes: 512}
	if err := h.service.reconcileInstallIntent(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	status, _ := h.service.InstallState(context.Background())
	if status.Status != "FAILED" || status.Error == nil {
		t.Fatalf("incomplete attempt must surface FAILED with detail: %+v", status)
	}
	if h.installer.calls != 0 {
		t.Fatalf("reconciliation must not start an install, calls=%d", h.installer.calls)
	}
}

// Reconciliation must never run before the lock: a stale owner surfaces as
// RECOVERY_REQUIRED and the recorded intent stays untouched.
func TestReconcileDefersToRecoveryRequired(t *testing.T) {
	h := newHarness(t)
	intents := &fakeIntents{present: true, intent: ports.InstallIntent{Phase: ports.PhaseRunning, Branch: "public", ManifestSHA: "before"}}
	h.service.deps.Intents = intents
	h.service.deps.Locks = &fakeLocks{err: errors.New("recovery required: previous ownership record present")}

	if _, err := h.service.Start(context.Background()); err == nil {
		t.Fatal("start must fail while recovery is required")
	} else if code, ok := ErrorCodeOf(err); !ok || code != CodeRecoveryRequired {
		t.Fatalf("expected RECOVERY_REQUIRED, got %v", err)
	}
	if intents.cleared != 0 || len(intents.writes) != 0 {
		t.Fatalf("recovery-required must not touch intent state: writes=%d cleared=%d", len(intents.writes), intents.cleared)
	}
}

// Failures keep the legacy projection (IDLE/INSTALLING/COMPLETED/FAILED) intact.
func TestTaskIntentFailureAndLegacyStatusProjection(t *testing.T) {
	h := newHarness(t)
	intents := &fakeIntents{}
	h.service.deps.Intents = intents
	h.installer.err = errors.New("network unreachable")
	if _, err := h.service.BeginInstall(context.Background(), ""); err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if status, _ := h.service.InstallState(context.Background()); status.Status == "FAILED" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, _ := h.service.InstallState(context.Background())
	if status.Status != "FAILED" || status.Error == nil || *status.Error == "" {
		t.Fatalf("legacy failure projection changed: %+v", status)
	}
	writes := intents.snapshot()
	var last ports.InstallIntent
	for _, write := range writes {
		last = write
	}
	if last.Phase != ports.PhaseFailed {
		t.Fatalf("expected FAILED intent, got %+v", writes)
	}
}

// fakeLocks returns a fixed error so lock ordering can be asserted.
type fakeLocks struct{ err error }

func (f *fakeLocks) Acquire(context.Context, domain.InstanceID) (ports.Lease, error) {
	if f.err != nil {
		return nil, f.err
	}
	return fakeLease{}, nil
}

type fakeLease struct{}

func (fakeLease) Release() error { return nil }
