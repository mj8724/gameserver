package ports

// BackupOperator is the automatic-backup seam used by the control service
// after a clean stop (M3.2). It is deliberately small: automatic backup is
// best-effort, never blocks control operations and never deletes data.
type BackupOperator interface {
	// Enabled reports whether automatic backups are configured.
	Enabled() bool
	// BackupNow runs one backup including save games when the instance has any.
	BackupNow() (BackupResult, error)
}

// BackupResult is the outcome of one automatic backup.
type BackupResult struct {
	Path     string
	Files    int
	Checksum string
}
