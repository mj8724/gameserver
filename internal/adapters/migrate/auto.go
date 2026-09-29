package migrate

import (
	"errors"

	"github.com/mj8724/gameserver/internal/ports"
)

// AutoBackup adapts the migration Tool to ports.BackupOperator. Automatic
// backups are opt-in from the composition root so the default wiring keeps the
// historical behaviour (nothing runs unless planned).
type AutoBackup struct {
	Tool     *Tool
	Keep     int
	DestRoot string
	enabled  bool
}

// NewAutoBackup wraps the tool with the destination root, retention floor and
// the enabled flag.
func NewAutoBackup(tool *Tool, destRoot string, enabled bool) *AutoBackup {
	return &AutoBackup{Tool: tool, Keep: DefaultBackupKeep, DestRoot: destRoot, enabled: enabled}
}

// Enabled reports whether automatic backups are configured.
func (a *AutoBackup) Enabled() bool { return a.enabled }

// BackupNow runs one backup including save games, then prunes to the retention
// floor (keeping at least the newest verified backup). Prune failures do not
// fail the backup: a completed backup is always worth keeping.
func (a *AutoBackup) BackupNow() (ports.BackupResult, error) {
	if a == nil || a.Tool == nil {
		return ports.BackupResult{}, errors.New("automatic backup is not configured")
	}
	if a.DestRoot == "" {
		return ports.BackupResult{}, errors.New("automatic backup destination root is not configured")
	}
	record, err := a.Tool.BackupWithOptions(a.DestRoot, BackupOptions{IncludeSaves: true})
	if err != nil {
		return ports.BackupResult{}, err
	}
	if _, err := a.Tool.PruneBackups(a.DestRoot, a.Keep); err != nil {
		// A completed backup must not be reported as failed because cleaning
		// older ones failed; the operator is told through the prune path.
		return ports.BackupResult{Path: record.Path, Files: len(record.Files), Checksum: record.Checksum}, nil
	}
	_ = record
	return ports.BackupResult{Path: record.Path, Files: len(record.Files), Checksum: record.Checksum}, nil
}
