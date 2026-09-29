package oslock

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
)

type fakeProbe struct {
	alivePIDs  map[int]bool
	candidates []int
	err        error
}

func (p fakeProbe) Alive(pid int) bool { return p.alivePIDs[pid] }

func (p fakeProbe) MatchingProcesses(string) ([]int, error) {
	return p.candidates, p.err
}

func fixedClock() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) }

func newManager(t *testing.T, probe ProcessProbe) (*Manager, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "servers")
	manager, err := NewManager(root, "service-a", WithClock(fixedClock), WithProbe(probe))
	if err != nil {
		t.Fatal(err)
	}
	return manager, root
}

func TestAcquireWritesOwnershipOutsideInstanceTree(t *testing.T) {
	manager, root := newManager(t, fakeProbe{})
	handle, err := manager.AcquireInstance("pz_01", OwnerMeta{OperationID: "op-1", CommandFingerprint: "sha256:redacted"})
	if err != nil {
		t.Fatalf("AcquireInstance: %v", err)
	}
	defer handle.Release()

	recordPath := filepath.Join(root, ".owners", "pz_01.json")
	info, err := os.Stat(recordPath)
	if err != nil {
		t.Fatalf("owner record missing: %v", err)
	}
	// Windows synthesises POSIX modes (and the adapters exempt it), so the mode
	// assertion only applies on POSIX platforms.
	if runtime.GOOS != "windows" && info.Mode().Perm() != fileMode {
		t.Fatalf("owner record mode = %v, want %v", info.Mode().Perm(), os.FileMode(fileMode))
	}
	lockPath := filepath.Join(root, ".locks", "pz_01.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
	for _, path := range []string{recordPath, lockPath} {
		if strings.Contains(path, filepath.Join(root, "pz_01")) {
			t.Fatalf("ownership path %s must stay outside the instance subtree", path)
		}
	}
	record := handle.Record()
	if record.ServiceID != "service-a" || record.OperationID != "op-1" || record.PID != os.Getpid() {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.Schema != ownerSchema || record.SessionToken == "" || !record.StartedAt.Equal(fixedClock()) {
		t.Fatalf("unexpected record fields: %+v", record)
	}
}

func TestSecondProcessIsRefusedAndReleaseFreesTheInstance(t *testing.T) {
	manager, _ := newManager(t, fakeProbe{})
	first, err := manager.AcquireInstance("pz_01", OwnerMeta{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewManager(manager.serversRoot, "service-b", WithProbe(fakeProbe{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.AcquireInstance("pz_01", OwnerMeta{}); !errors.Is(err, ErrLockedByOther) {
		t.Fatalf("second acquire error = %v, want ErrLockedByOther", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(manager.OwnersDir(), "pz_01.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owner record should be removed on release, stat err = %v", err)
	}
	if handle, err := second.AcquireInstance("pz_01", OwnerMeta{}); err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	} else {
		_ = handle.Release()
	}
}

func TestExistingRecordOrResidualProcessRequiresRecovery(t *testing.T) {
	manager, root := newManager(t, fakeProbe{})
	handle, err := manager.AcquireInstance("pz_01", OwnerMeta{})
	if err != nil {
		t.Fatal(err)
	}
	// Unlock keeps the ownership record: an unclean stop must fail closed.
	if err := handle.Unlock(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AcquireInstance("pz_01", OwnerMeta{}); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("stale record error = %v, want ErrRecoveryRequired", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".owners", "pz_01.json")); err != nil {
		t.Fatalf("stale record must be preserved: %v", err)
	}
	if err := os.Remove(filepath.Join(root, ".owners", "pz_01.json")); err != nil {
		t.Fatal(err)
	}

	residual, err := NewManager(manager.serversRoot, "service-b", WithProbe(fakeProbe{candidates: []int{4242}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := residual.AcquireInstance("pz_01", OwnerMeta{}); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("residual process error = %v, want ErrRecoveryRequired", err)
	}
	probeFailure, err := NewManager(manager.serversRoot, "service-c", WithProbe(fakeProbe{err: errors.New("probe down")}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := probeFailure.AcquireInstance("pz_01", OwnerMeta{}); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("probe failure error = %v, want ErrRecoveryRequired", err)
	}
}

func TestRecoverHonoursLockAndLiveness(t *testing.T) {
	manager, root := newManager(t, fakeProbe{alivePIDs: map[int]bool{os.Getpid(): true}})
	handle, err := manager.AcquireInstance("pz_01", OwnerMeta{})
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(root, ".owners", "pz_01.json")

	other, err := NewManager(manager.serversRoot, "service-b", WithProbe(fakeProbe{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Recover("pz_01", true); !errors.Is(err, ErrLockedByOther) {
		t.Fatalf("recover without lock = %v, want ErrLockedByOther", err)
	}
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("record must be untouched: %v", err)
	}

	live := fakeProbe{alivePIDs: map[int]bool{os.Getpid(): true}}
	recoverer, err := NewManager(manager.serversRoot, "service-c", WithProbe(live))
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Unlock(); err != nil {
		t.Fatal(err)
	}
	if _, err := recoverer.Recover("pz_01", false); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("live pid without force = %v, want ErrRecoveryRequired", err)
	}
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("live record must survive refused recovery: %v", err)
	}
	result, err := recoverer.Recover("pz_01", true)
	if err != nil {
		t.Fatalf("forced recovery: %v", err)
	}
	if result.Action != "forced_clear" || result.PreviousRecord == nil || !result.PIDAlive {
		t.Fatalf("unexpected forced recovery result: %+v", result)
	}
	if _, err := os.Stat(recordPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("forced recovery should clear the record, stat err = %v", err)
	}
	if result, err := recoverer.Recover("pz_01", false); err != nil || result.Action != "no_record" {
		t.Fatalf("recover without record = %+v, %v", result, err)
	}
}

func TestRecoverClearsStaleRecordAndInspectionReportsState(t *testing.T) {
	manager, root := newManager(t, fakeProbe{})
	handle, err := manager.AcquireInstance("pz_01", OwnerMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Unlock(); err != nil {
		t.Fatal(err)
	}
	stale, err := NewManager(manager.serversRoot, "service-b", WithProbe(fakeProbe{}))
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := stale.Inspect("pz_01")
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Record == nil || inspection.PIDAlive {
		t.Fatalf("unexpected inspection: %+v", inspection)
	}
	result, err := stale.Recover("pz_01", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "cleared_stale" || result.PIDAlive {
		t.Fatalf("unexpected stale recovery: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, ".owners", "pz_01.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale record should be cleared, stat err = %v", err)
	}
}

func TestMigrationLockIsExclusive(t *testing.T) {
	manager, _ := newManager(t, fakeProbe{})
	first, err := manager.AcquireMigrationLock()
	if err != nil {
		t.Fatal(err)
	}
	defer first.Unlock()
	second, err := NewManager(manager.serversRoot, "service-b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.AcquireMigrationLock(); !errors.Is(err, ErrLockedByOther) {
		t.Fatalf("second migration lock = %v, want ErrLockedByOther", err)
	}
	if !strings.HasSuffix(first.LockPath(), migrationLockName) {
		t.Fatalf("migration lock path = %s", first.LockPath())
	}
}

func TestInvalidInstanceIDAndMissingRootAreRejected(t *testing.T) {
	manager, _ := newManager(t, fakeProbe{})
	if _, err := manager.AcquireInstance(domain.InstanceID("../escape"), OwnerMeta{}); err == nil {
		t.Fatal("traversal instance id accepted")
	}
	if _, err := NewManager("", "service"); err == nil {
		t.Fatal("empty servers root accepted")
	}
	if _, err := NewManager("/tmp/servers", ""); err == nil {
		t.Fatal("empty service id accepted")
	}
}
