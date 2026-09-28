// Package migrate implements the versioned import and journaled promotion of
// instance state (ADR-001 §4). The switched unit is only <instance>/state/;
// saves, server files, and the SteamCMD tree are never moved.
package migrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
)

// Schema identifiers recorded in on-disk artifacts.
const (
	journalSchema  = "migration-journal/1"
	manifestSchema = "migration-manifest/1"
	dirMode        = 0o700
	fileMode       = 0o600
)

// JournalState is the promotion state machine.
type JournalState string

// State machine values from ADR-001 §4.2/§4.3.
const (
	StateIdle         JournalState = "IDLE"
	StateExported     JournalState = "EXPORTED"
	StateStaged       JournalState = "STAGED"
	StateVerified     JournalState = "VERIFIED"
	StateCommitting   JournalState = "COMMITTING"
	StateCommitted    JournalState = "COMMITTED"
	StateRollingBack  JournalState = "ROLLING_BACK"
	StateRolledBack   JournalState = "ROLLED_BACK"
	StateLegacyActive JournalState = "LEGACY_ACTIVE"
)

// ErrRecoveryRequired reports that the on-disk layout cannot be resolved
// automatically and needs explicit operator recovery.
var ErrRecoveryRequired = errors.New("recovery required")

// ErrManifestMismatch reports that staged or promoted content does not match
// the recorded manifest.
var ErrManifestMismatch = errors.New("manifest checksum mismatch")

var instancePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Journal is the durable progress marker written before any rename.
type Journal struct {
	Schema             string       `json:"schema"`
	State              JournalState `json:"state"`
	Sequence           int          `json:"sequence"`
	UpdatedAt          time.Time    `json:"updated_at"`
	ManifestChecksum   string       `json:"manifest_checksum,omitempty"`
	LastRolledBackFrom string       `json:"last_rolled_back_from,omitempty"`
	RecoveryReason     string       `json:"recovery_reason,omitempty"`
}

// ManifestFile is one checksummed file inside the switched state subtree.
type ManifestFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
}

// Manifest is the versioned inventory of the state subtree.
type Manifest struct {
	Schema    string         `json:"schema"`
	Version   string         `json:"version"`
	CreatedAt time.Time      `json:"created_at"`
	Checksum  string         `json:"checksum"`
	Files     []ManifestFile `json:"files"`
}

// Layout resolves every path used by the promotion protocol.
type Layout struct {
	ServersRoot string
	Instance    domain.InstanceID
}

// Validate rejects unsafe instance identifiers and empty roots.
func (l Layout) Validate() error {
	if l.ServersRoot == "" {
		return errors.New("servers root is required")
	}
	if !instancePattern.MatchString(string(l.Instance)) {
		return fmt.Errorf("invalid instance id %q", l.Instance)
	}
	return nil
}

// InstanceDir is the instance root (never moved as a whole).
func (l Layout) InstanceDir() string {
	return filepath.Join(l.ServersRoot, string(l.Instance))
}

// StateDir is the only switched subtree.
func (l Layout) StateDir() string { return filepath.Join(l.InstanceDir(), "state") }

// StagingDir holds imported state content before promotion.
func (l Layout) StagingDir() string {
	return filepath.Join(l.MigrationDir(), "staging", string(l.Instance))
}

// PrevDir holds the state subtree replaced by one promotion.
func (l Layout) PrevDir(sequence int) string {
	return filepath.Join(l.MigrationDir(), "prev", fmt.Sprintf("%s.%d", l.Instance, sequence))
}

// MigrationDir is the shared migration workspace.
func (l Layout) MigrationDir() string { return filepath.Join(l.ServersRoot, ".migration") }

// JournalPath is the journal file path.
func (l Layout) JournalPath() string { return filepath.Join(l.MigrationDir(), "journal.json") }

// ManifestPath is the manifest file path.
func (l Layout) ManifestPath() string { return filepath.Join(l.MigrationDir(), "manifest.json") }

// Snapshot is a read-only view of the migration site.
type Snapshot struct {
	Journal     Journal
	StateExists bool
	PrevExists  bool
	StagingHere bool
}

// Blocked reports whether a non-terminal journal forbids normal startup.
func (s Snapshot) Blocked() bool {
	switch s.Journal.State {
	case StateStaged, StateVerified, StateCommitting, StateRollingBack:
		return true
	default:
		return false
	}
}

// Ops abstracts filesystem effects so interruption tests can inject failures.
type Ops interface {
	Stat(path string) (os.FileInfo, error)
	MkdirAll(path string, mode os.FileMode) error
	RemoveAll(path string) error
	Rename(oldPath, newPath string) error
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, mode os.FileMode) error
	WriteFileAtomic(path string, data []byte, mode os.FileMode) error
	ReadDir(path string) ([]os.DirEntry, error)
}

// Tool drives promotion for one instance.
type Tool struct {
	layout Layout
	ops    Ops
	clock  func() time.Time
}

// Option customizes a Tool.
type Option func(*Tool)

// WithOps injects filesystem operations (used by fault-injection tests).
func WithOps(ops Ops) Option {
	return func(tool *Tool) {
		if ops != nil {
			tool.ops = ops
		}
	}
}

// WithClock injects a deterministic clock.
func WithClock(clock func() time.Time) Option {
	return func(tool *Tool) {
		if clock != nil {
			tool.clock = clock
		}
	}
}

// New builds a tool for one instance.
func New(layout Layout, options ...Option) (*Tool, error) {
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(layout.ServersRoot)
	if err != nil {
		return nil, err
	}
	layout.ServersRoot = absolute
	tool := &Tool{layout: layout, ops: realOps{}, clock: time.Now}
	for _, option := range options {
		option(tool)
	}
	return tool, nil
}

// Layout returns the resolved layout.
func (t *Tool) Layout() Layout { return t.layout }

func (t *Tool) exists(path string) bool {
	_, err := t.ops.Stat(path)
	return err == nil
}

func (t *Tool) loadJournal() (Journal, error) {
	contents, err := t.ops.ReadFile(t.layout.JournalPath())
	if errors.Is(err, os.ErrNotExist) {
		return Journal{Schema: journalSchema, State: StateIdle}, nil
	}
	if err != nil {
		return Journal{}, err
	}
	var journal Journal
	if err := json.Unmarshal(contents, &journal); err != nil {
		return Journal{}, fmt.Errorf("%w: journal unreadable: %v", ErrRecoveryRequired, err)
	}
	if journal.Schema != journalSchema {
		return Journal{}, fmt.Errorf("%w: unknown journal schema %q", ErrRecoveryRequired, journal.Schema)
	}
	return journal, nil
}

func (t *Tool) writeJournal(journal Journal) error {
	journal.Schema = journalSchema
	journal.UpdatedAt = t.clock().UTC()
	encoded, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	if err := t.ops.MkdirAll(t.layout.MigrationDir(), dirMode); err != nil {
		return err
	}
	// The COMMITTING barrier must be durable before the first rename.
	return t.ops.WriteFileAtomic(t.layout.JournalPath(), append(encoded, '\n'), fileMode)
}

// Inspect reads journal and layout without modifying anything.
func (t *Tool) Inspect() (Snapshot, error) {
	journal, err := t.loadJournal()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Journal:     journal,
		StateExists: t.exists(t.layout.StateDir()),
		PrevExists:  t.exists(t.layout.PrevDir(journal.Sequence)),
		StagingHere: t.exists(t.layout.StagingDir()),
	}, nil
}

// RecoverResult reports what recovery did.
type RecoverResult struct {
	Journal Journal
	Action  string
}

// Recover resolves an interrupted promotion strictly by the ADR-§4.3 table.
func (t *Tool) Recover() (RecoverResult, error) {
	snapshot, err := t.Inspect()
	if err != nil {
		return RecoverResult{}, err
	}
	journal := snapshot.Journal
	prevPath := t.layout.PrevDir(journal.Sequence)
	stagingPath := t.layout.StagingDir()
	statePath := t.layout.StateDir()

	switch journal.State {
	case StateIdle, StateExported:
		if snapshot.StagingHere {
			if err := t.ops.RemoveAll(stagingPath); err != nil {
				return RecoverResult{}, err
			}
		}
		if err := t.writeJournal(journal); err != nil {
			return RecoverResult{}, err
		}
		return RecoverResult{Journal: journal, Action: "discarded_staging"}, nil

	case StateStaged:
		if !snapshot.StagingHere {
			return t.recoveryRequired(journal, "staged content missing")
		}
		manifest, err := t.verifyStaging()
		if err != nil {
			if err := t.ops.RemoveAll(stagingPath); err != nil {
				return RecoverResult{}, err
			}
			journal.State = StateExported
			if err := t.writeJournal(journal); err != nil {
				return RecoverResult{}, err
			}
			return RecoverResult{Journal: journal, Action: "discarded_invalid_staging"}, nil
		}
		journal.State = StateVerified
		journal.ManifestChecksum = manifest.Checksum
		if err := t.writeJournal(journal); err != nil {
			return RecoverResult{}, err
		}
		return RecoverResult{Journal: journal, Action: "verified"}, nil

	case StateVerified:
		if !snapshot.StateExists {
			return t.recoveryRequired(journal, "state missing while verified")
		}
		if snapshot.StagingHere {
			if err := t.ops.RemoveAll(stagingPath); err != nil {
				return RecoverResult{}, err
			}
		}
		return RecoverResult{Journal: journal, Action: "discarded_staging"}, nil

	case StateCommitting:
		return t.recoverCommitting(journal, snapshot, prevPath, stagingPath, statePath)

	case StateRollingBack:
		switch {
		case !snapshot.StateExists && snapshot.PrevExists:
			if err := t.renameInto(prevPath, statePath); err != nil {
				return RecoverResult{}, err
			}
			journal.State = StateRolledBack
			journal.LastRolledBackFrom = prevPath
			if err := t.writeJournal(journal); err != nil {
				return RecoverResult{}, err
			}
			return RecoverResult{Journal: journal, Action: "rolled_back"}, nil
		case snapshot.StateExists && !snapshot.PrevExists:
			journal.State = StateRolledBack
			if err := t.writeJournal(journal); err != nil {
				return RecoverResult{}, err
			}
			return RecoverResult{Journal: journal, Action: "recorded_rolled_back"}, nil
		default:
			return t.recoveryRequired(journal, "inconsistent rollback site")
		}

	case StateRolledBack, StateLegacyActive:
		return RecoverResult{Journal: journal, Action: "noop"}, nil

	case StateCommitted:
		if !snapshot.StateExists {
			return t.recoveryRequired(journal, "committed state missing")
		}
		if manifest, err := t.readManifest(); err == nil {
			if err := t.verifyTree(statePath, manifest); err != nil {
				return t.recoveryRequired(journal, "committed state failed checksum")
			}
		}
		return RecoverResult{Journal: journal, Action: "noop"}, nil

	default:
		return t.recoveryRequired(journal, fmt.Sprintf("unknown journal state %q", journal.State))
	}
}

func (t *Tool) recoverCommitting(journal Journal, snapshot Snapshot, prevPath, stagingPath, statePath string) (RecoverResult, error) {
	switch {
	case !snapshot.StateExists && snapshot.PrevExists && !snapshot.StagingHere:
		if err := t.renameInto(prevPath, statePath); err != nil {
			return RecoverResult{}, err
		}
		journal.State = StateRolledBack
		journal.LastRolledBackFrom = prevPath
		if err := t.writeJournal(journal); err != nil {
			return RecoverResult{}, err
		}
		return RecoverResult{Journal: journal, Action: "rolled_back"}, nil

	case snapshot.StateExists && snapshot.PrevExists && !snapshot.StagingHere:
		manifest, err := t.readManifest()
		if err != nil {
			return t.recoveryRequired(journal, "manifest unreadable for forward-roll")
		}
		if err := t.verifyTree(statePath, manifest); err != nil {
			return t.recoveryRequired(journal, "forward-roll checksum mismatch")
		}
		journal.State = StateCommitted
		if err := t.writeJournal(journal); err != nil {
			return RecoverResult{}, err
		}
		return RecoverResult{Journal: journal, Action: "forward_rolled"}, nil

	case !snapshot.StateExists && snapshot.PrevExists && snapshot.StagingHere:
		manifest, err := t.verifyStaging()
		if err != nil {
			return t.recoveryRequired(journal, "staging checksum mismatch between renames")
		}
		if err := t.renameInto(stagingPath, statePath); err != nil {
			return RecoverResult{}, err
		}
		journal.State = StateCommitted
		journal.ManifestChecksum = manifest.Checksum
		if err := t.writeJournal(journal); err != nil {
			return RecoverResult{}, err
		}
		return RecoverResult{Journal: journal, Action: "forward_rolled"}, nil

	case !snapshot.StateExists && !snapshot.PrevExists && snapshot.StagingHere:
		// First promotion interrupted: no prev exists, so rollback does not apply.
		manifest, err := t.verifyStaging()
		if err != nil {
			return t.recoveryRequired(journal, "first promotion staging checksum mismatch")
		}
		if err := t.renameInto(stagingPath, statePath); err != nil {
			return RecoverResult{}, err
		}
		journal.State = StateCommitted
		journal.ManifestChecksum = manifest.Checksum
		if err := t.writeJournal(journal); err != nil {
			return RecoverResult{}, err
		}
		return RecoverResult{Journal: journal, Action: "forward_rolled"}, nil

	case snapshot.StateExists && !snapshot.PrevExists && snapshot.StagingHere:
		// Step 1 has not run yet or failed: retry step 1 then step 2.
		if err := t.stepOne(&journal); err != nil {
			return t.recoveryRequired(journal, "retry step 1 failed")
		}
		if err := t.stepTwo(nil); err != nil {
			return t.recoveryRequired(journal, "retry step 2 failed")
		}
		if manifest, err := t.readManifest(); err == nil {
			journal.ManifestChecksum = manifest.Checksum
		}
		journal.State = StateCommitted
		if err := t.writeJournal(journal); err != nil {
			return RecoverResult{}, err
		}
		return RecoverResult{Journal: journal, Action: "forward_rolled"}, nil

	default:
		return t.recoveryRequired(journal, "undetermined COMMITTING layout")
	}
}

func (t *Tool) recoveryRequired(journal Journal, reason string) (RecoverResult, error) {
	journal.RecoveryReason = reason
	_ = t.writeJournal(journal)
	return RecoverResult{Journal: journal, Action: "recovery_required"}, fmt.Errorf("%w: %s", ErrRecoveryRequired, reason)
}
