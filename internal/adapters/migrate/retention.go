package migrate

// M3.2 retention policy decisions (recorded in the acceptance matrix):
//   - keep: the default number of backups retained by an automatic prune.
//     It matches the ADR §4.6 default of 3, and every prune keeps at least the
//     newest verified backup (see PruneBackups).
//   - server files: never included in backups (regenerable, tens of GB) until
//     the retention/capacity policy work revisits it — see BackupOptions.
const DefaultBackupKeep = 3
