// Package oslock implements cross-process instance ownership: an OS-level
// exclusive lock, an ownership record outside the switched state subtree, and
// fail-closed startup reconciliation.
package oslock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

const (
	ownerSchema       = "owner/1"
	locksDirName      = ".locks"
	ownersDirName     = ".owners"
	migrationLockName = ".migration.lock"
	dirMode           = 0o700
	fileMode          = 0o600
)

var (
	// ErrLockedByOther reports that another live process owns the instance.
	ErrLockedByOther = errors.New("instance owned by another process")
	// ErrRecoveryRequired reports that ownership cannot be proven safe.
	ErrRecoveryRequired = errors.New("recovery required")
	// ErrUnsupportedPlatform reports that no safe lock primitive exists here.
	ErrUnsupportedPlatform = errors.New("instance locking is unsupported on this platform")

	instancePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// OwnerRecord is the persisted ownership claim for one instance.
type OwnerRecord struct {
	Schema             string    `json:"schema"`
	ServiceID          string    `json:"service_id"`
	PID                int       `json:"pid"`
	SessionToken       string    `json:"session_token"`
	StartedAt          time.Time `json:"started_at"`
	Root               string    `json:"root"`
	CommandFingerprint string    `json:"command_fingerprint,omitempty"`
	OperationID        string    `json:"operation_id,omitempty"`
}

// OwnerMeta carries the fields a start operation knows about itself.
type OwnerMeta struct {
	OperationID        string
	CommandFingerprint string
}

// ProcessProbe reports whether a recorded process is still alive and whether
// any process still looks like it belongs to the instance root.
type ProcessProbe interface {
	Alive(pid int) bool
	MatchingProcesses(root string) ([]int, error)
}

// Manager owns lock and ownership paths for a servers root.
type Manager struct {
	serversRoot string
	serviceID   string
	clock       func() time.Time
	probe       ProcessProbe
}

// Option customizes a Manager (clock and process probe are injectable).
type Option func(*Manager)

// WithClock injects a deterministic clock.
func WithClock(clock func() time.Time) Option {
	return func(manager *Manager) {
		if clock != nil {
			manager.clock = clock
		}
	}
}

// WithProbe injects a process probe.
func WithProbe(probe ProcessProbe) Option {
	return func(manager *Manager) {
		if probe != nil {
			manager.probe = probe
		}
	}
}

// NewManager builds a manager for one servers root (for example
// <data_root>/servers). Locks and owner records always live outside the
// instance directory, so a state-subtree promotion cannot move them.
func NewManager(serversRoot, serviceID string, options ...Option) (*Manager, error) {
	if strings.TrimSpace(serversRoot) == "" {
		return nil, errors.New("servers root is required")
	}
	if strings.TrimSpace(serviceID) == "" {
		return nil, errors.New("service id is required")
	}
	absolute, err := filepath.Abs(serversRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve servers root: %w", err)
	}
	manager := &Manager{
		serversRoot: absolute,
		serviceID:   serviceID,
		clock:       time.Now,
		probe:       OSProbe{},
	}
	for _, option := range options {
		option(manager)
	}
	return manager, nil
}

// LocksDir returns the shared lock directory.
func (m *Manager) LocksDir() string { return filepath.Join(m.serversRoot, locksDirName) }

// OwnersDir returns the shared ownership record directory.
func (m *Manager) OwnersDir() string { return filepath.Join(m.serversRoot, ownersDirName) }

// InstanceLockPath returns the per-instance lock file path.
func (m *Manager) InstanceLockPath(id domain.InstanceID) (string, error) {
	if !instancePattern.MatchString(string(id)) {
		return "", fmt.Errorf("invalid instance id %q", id)
	}
	return filepath.Join(m.LocksDir(), string(id)+".lock"), nil
}

// OwnerRecordPath returns the per-instance ownership record path.
func (m *Manager) OwnerRecordPath(id domain.InstanceID) (string, error) {
	if !instancePattern.MatchString(string(id)) {
		return "", fmt.Errorf("invalid instance id %q", id)
	}
	return filepath.Join(m.OwnersDir(), string(id)+".json"), nil
}

// MigrationLockPath returns the data-root level serialization lock used for
// shared resources (migration journal and the SteamCMD bootstrap tree).
func (m *Manager) MigrationLockPath() string {
	return filepath.Join(m.LocksDir(), migrationLockName)
}

// Handle is a held instance lock plus its ownership record. Release is
// idempotent and removes the record only when it still belongs to this handle.
type Handle struct {
	lock   *fileLock
	path   string
	record OwnerRecord
	probe  ProcessProbe
}

// Record returns the ownership record written by this handle.
func (h *Handle) Record() OwnerRecord { return h.record }

// LockPath returns the lock file path held by this handle.
func (h *Handle) LockPath() string { return h.lock.path }

// Release removes the ownership record and releases the OS lock.
func (h *Handle) Release() error {
	if h == nil || h.lock == nil {
		return nil
	}
	var releaseErr error
	if h.path != "" {
		if record, err := readOwnerRecord(h.path); err == nil && record.SessionToken == h.record.SessionToken {
			if err := os.Remove(h.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				releaseErr = err
			}
		}
	}
	if err := h.lock.release(); err != nil && releaseErr == nil {
		releaseErr = err
	}
	return releaseErr
}

// Unlock releases only the OS lock, keeping the ownership record for
// diagnostics.
func (h *Handle) Unlock() error {
	if h == nil || h.lock == nil {
		return nil
	}
	return h.lock.release()
}

// FileLock is a held advisory lock without an ownership record.
type FileLock struct {
	lock *fileLock
}

// Unlock releases the file lock; it is idempotent.
func (l *FileLock) Unlock() error {
	if l == nil || l.lock == nil {
		return nil
	}
	return l.lock.release()
}

// LockPath returns the held lock path.
func (l *FileLock) LockPath() string {
	if l == nil || l.lock == nil {
		return ""
	}
	return l.lock.path
}

// AcquireInstance takes the instance lock and writes the ownership record.
// It fails closed with ErrRecoveryRequired whenever a previous record exists or
// a residual process is detected, and with ErrLockedByOther when the lock is
// held elsewhere.
func (m *Manager) AcquireInstance(id domain.InstanceID, meta OwnerMeta) (*Handle, error) {
	lockPath, err := m.InstanceLockPath(id)
	if err != nil {
		return nil, err
	}
	recordPath, err := m.OwnerRecordPath(id)
	if err != nil {
		return nil, err
	}
	if err := m.prepareDirs(); err != nil {
		return nil, err
	}
	lock, acquired, err := tryLockFile(lockPath)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrLockedByOther
	}
	releaseOnError := true
	defer func() {
		if releaseOnError {
			_ = lock.release()
		}
	}()

	if _, err := readOwnerRecord(recordPath); err == nil {
		return nil, fmt.Errorf("%w: previous ownership record exists at %s", ErrRecoveryRequired, filepath.Base(recordPath))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: ownership record unreadable: %v", ErrRecoveryRequired, err)
	}

	candidates, err := m.probe.MatchingProcesses(m.serversRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: process probe failed: %v", ErrRecoveryRequired, err)
	}
	if len(candidates) > 0 {
		return nil, fmt.Errorf("%w: %d residual process(es) reference the instance root", ErrRecoveryRequired, len(candidates))
	}

	token, err := randomToken()
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	record := OwnerRecord{
		Schema:             ownerSchema,
		ServiceID:          m.serviceID,
		PID:                os.Getpid(),
		SessionToken:       token,
		StartedAt:          m.clock().UTC(),
		Root:               m.serversRoot,
		OperationID:        meta.OperationID,
		CommandFingerprint: meta.CommandFingerprint,
	}
	if err := writeOwnerRecord(recordPath, record); err != nil {
		return nil, err
	}
	releaseOnError = false
	return &Handle{lock: lock, path: recordPath, record: record, probe: m.probe}, nil
}

// AcquireMigrationLock serializes writes to shared resources (migration
// journal, SteamCMD bootstrap tree) across instances.
func (m *Manager) AcquireMigrationLock() (*FileLock, error) {
	if err := m.prepareDirs(); err != nil {
		return nil, err
	}
	lock, acquired, err := tryLockFile(m.MigrationLockPath())
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrLockedByOther
	}
	return &FileLock{lock: lock}, nil
}

// Inspection is a read-only view of one instance's ownership state.
type Inspection struct {
	Record      *OwnerRecord
	RecordError error
	PIDAlive    bool
	Candidates  []int
}

// Inspect reads ownership state without taking the lock.
func (m *Manager) Inspect(id domain.InstanceID) (Inspection, error) {
	recordPath, err := m.OwnerRecordPath(id)
	if err != nil {
		return Inspection{}, err
	}
	inspection := Inspection{}
	record, err := readOwnerRecord(recordPath)
	switch {
	case err == nil:
		inspection.Record = &record
		inspection.PIDAlive = m.probe.Alive(record.PID)
	case errors.Is(err, os.ErrNotExist):
	default:
		inspection.RecordError = err
	}
	candidates, probeErr := m.probe.MatchingProcesses(m.serversRoot)
	if probeErr == nil {
		inspection.Candidates = candidates
	}
	return inspection, nil
}

// RecoveryResult records what a recovery attempt changed.
type RecoveryResult struct {
	Action         string
	PreviousRecord *OwnerRecord
	PIDAlive       bool
}

// Recover clears stale ownership. It must hold the same instance lock; if the
// lock is unavailable it refuses and changes nothing. A live recorded process
// is never taken over unless force is set.
func (m *Manager) Recover(id domain.InstanceID, force bool) (RecoveryResult, error) {
	lockPath, err := m.InstanceLockPath(id)
	if err != nil {
		return RecoveryResult{}, err
	}
	recordPath, err := m.OwnerRecordPath(id)
	if err != nil {
		return RecoveryResult{}, err
	}
	if err := m.prepareDirs(); err != nil {
		return RecoveryResult{}, err
	}
	lock, acquired, err := tryLockFile(lockPath)
	if err != nil {
		return RecoveryResult{}, err
	}
	if !acquired {
		return RecoveryResult{}, ErrLockedByOther
	}
	defer func() { _ = lock.release() }()

	record, err := readOwnerRecord(recordPath)
	if errors.Is(err, os.ErrNotExist) {
		return RecoveryResult{Action: "no_record"}, nil
	}
	if err != nil {
		return RecoveryResult{}, fmt.Errorf("%w: ownership record unreadable: %v", ErrRecoveryRequired, err)
	}
	alive := m.probe.Alive(record.PID)
	result := RecoveryResult{PreviousRecord: &record, PIDAlive: alive}
	if alive && !force {
		return result, fmt.Errorf("%w: recorded process %d is still alive", ErrRecoveryRequired, record.PID)
	}
	if err := os.Remove(recordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if force && alive {
		result.Action = "forced_clear"
		return result, nil
	}
	result.Action = "cleared_stale"
	return result, nil
}

func (m *Manager) prepareDirs() error {
	for _, dir := range []string{m.serversRoot, m.LocksDir(), m.OwnersDir()} {
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return err
		}
	}
	return nil
}

func writeOwnerRecord(path string, record OwnerRecord) error {
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ownership record: %w", err)
	}
	encoded = append(encoded, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".owner-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if err := temp.Chmod(fileMode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func readOwnerRecord(path string) (OwnerRecord, error) {
	var record OwnerRecord
	contents, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(contents, &record); err != nil {
		return record, err
	}
	return record, nil
}

func randomToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

// Locks is the ports.InstanceLock implementation over a servers root.
type Locks struct {
	ServersRoot string
	ServiceID   string
	Options     []Option
}

// Acquire implements ports.InstanceLock for the configured servers root.
func (l *Locks) Acquire(_ context.Context, id domain.InstanceID) (ports.Lease, error) {
	manager, err := NewManager(l.ServersRoot, l.ServiceID, l.Options...)
	if err != nil {
		return nil, err
	}
	return manager.AcquireInstance(id, OwnerMeta{})
}
