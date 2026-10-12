package localstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// IntentLog persists the phase history of a long-running task so a restarted
// process can reconcile instead of silently forgetting it (ADR §5.4 D10). The
// file lives beside the instance state and is safe to delete: the state
// document keeps its own schema and stays the only authority for locks.
type IntentLog struct {
	dataRoot string
	ops      fileOps
	mu       sync.Mutex
}

// NewIntentLog creates an intent log rooted at the same data root as the state
// store so both share the instance directory and its guarantees.
func NewIntentLog(dataRoot string) (*IntentLog, error) {
	if strings.TrimSpace(dataRoot) == "" || strings.ContainsRune(dataRoot, '\x00') {
		return nil, errors.New("valid data root is required")
	}
	root, err := filepath.Abs(dataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve data root: %w", err)
	}
	return &IntentLog{dataRoot: root, ops: osFileOps{}}, nil
}

func (l *IntentLog) path(id domain.InstanceID) string {
	return filepath.Join(l.dataRoot, "servers", string(id), "state", "install-intent.json")
}

func validID(id domain.InstanceID) bool {
	return validInstanceID(id)
}

// Load returns the recorded intent, or ErrIntentAbsent when none exists.
func (l *IntentLog) Load(_ context.Context, id domain.InstanceID) (ports.InstallIntent, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !validID(id) {
		return ports.InstallIntent{}, errors.New("invalid instance id")
	}
	raw, err := l.ops.ReadFile(l.path(id))
	if err != nil {
		if os.IsNotExist(err) {
			return ports.InstallIntent{}, ports.ErrIntentAbsent
		}
		return ports.InstallIntent{}, err
	}
	var intent ports.InstallIntent
	if err := json.Unmarshal(raw, &intent); err != nil {
		return ports.InstallIntent{}, fmt.Errorf("parse install intent: %w", err)
	}
	return intent, nil
}

// Write atomically persists the intent (same-directory temp + fsync + readback,
// reusing the state store's durability protocol).
func (l *IntentLog) Write(_ context.Context, id domain.InstanceID, intent ports.InstallIntent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !validID(id) {
		return errors.New("invalid instance id")
	}
	if intent.Phase == "" {
		return errors.New("install intent phase is required")
	}
	data, err := json.MarshalIndent(intent, "", "  ")
	if err != nil {
		return err
	}
	target := l.path(id)
	dir := filepath.Dir(target)
	if err := l.ops.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := l.ops.Lstat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("intent directory is not a real directory: %q", dir)
	}
	return atomicWriteFile(l.ops, target, data, 0o600)
}

// Clear removes the intent file. The instance is then indistinguishable from a
// fresh one for reconciliation purposes.
func (l *IntentLog) Clear(_ context.Context, id domain.InstanceID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !validID(id) {
		return errors.New("invalid instance id")
	}
	if err := l.ops.Remove(l.path(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
