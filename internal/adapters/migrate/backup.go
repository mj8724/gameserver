package migrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const backupSchema = "migration-backup/1"

// BackupRecord describes one completed backup.
type BackupRecord struct {
	Schema    string         `json:"schema"`
	CreatedAt time.Time      `json:"created_at"`
	Instance  string         `json:"instance"`
	Files     []ManifestFile `json:"files"`
	Checksum  string         `json:"checksum"`
	Includes  []string       `json:"includes,omitempty"`
	Path      string         `json:"-"`
}

// BackupOptions selects the optional content planes carried by a backup.
// The state subtree and the managed PZ INI/SandboxVars files are always
// included; saves and server files are opt-in (M3.2 decision).
type BackupOptions struct {
	IncludeSaves bool
	// IncludeServerFiles is deliberately unimplemented for M3.2 and recorded
	// as a decision: server_files can be regenerated and adds >20 GB to every
	// backup, which destabilizes capacity limits (M3.5). Revisit with the
	// retention/capacity policy work.
	IncludeServerFiles bool
}

// Backup copies the managed state subtree and the PZ server INI files into a
// timestamped directory and verifies every checksum by re-reading. Saves and
// server_files are deliberately excluded (ADR §4.6).
func (t *Tool) Backup(destinationRoot string) (BackupRecord, error) {
	return t.BackupWithOptions(destinationRoot, BackupOptions{})
}

// BackupWithOptions copies the managed state subtree, the PZ server INI files
// and (optionally) the save games into a timestamped directory and verifies
// every checksum by re-reading. The include mode is recorded in the manifest so
// a restore never claims more than it holds.
func (t *Tool) BackupWithOptions(destinationRoot string, opts BackupOptions) (BackupRecord, error) {
	if strings.TrimSpace(destinationRoot) == "" {
		return BackupRecord{}, errors.New("backup destination root is required")
	}
	layout := t.layout
	stamp := t.clock().UTC().Format("20060102T150405Z")
	destination := filepath.Join(destinationRoot, string(layout.Instance), stamp)
	if err := t.ops.MkdirAll(destination, dirMode); err != nil {
		return BackupRecord{}, err
	}

	sources := []string{layout.StateDir()}
	iniDir := filepath.Join(layout.InstanceDir(), "Zomboid", "Server")
	if entries, err := t.ops.ReadDir(iniDir); err == nil {
		for _, entry := range entries {
			name := strings.ToLower(entry.Name())
			// ADR §4.6: the sandbox file is a managed file since r3 and must be
			// covered by the default backup range alongside the INI files.
			if entry.IsDir() || !(strings.HasSuffix(name, ".ini") || strings.HasSuffix(name, "_sandboxvars.lua")) {
				continue
			}
			sources = append(sources, filepath.Join(iniDir, entry.Name()))
		}
	}
	var includes []string
	if opts.IncludeSaves {
		// PZ keeps the world under the instance cache directory; several
		// subfolders look like "Saves". Back up the whole cache only when the
		// caller opts in, and record it so restore can target the same shape.
		savesRoot := filepath.Join(layout.InstanceDir(), "Zomboid", "Saves")
		if info, err := t.ops.Stat(savesRoot); err == nil && info.IsDir() {
			sources = append(sources, savesRoot)
			includes = append(includes, "saves")
		}
	}

	record := BackupRecord{Schema: backupSchema, CreatedAt: t.clock().UTC(), Instance: string(layout.Instance), Includes: includes, Path: destination}
	for _, source := range sources {
		info, err := t.ops.Stat(source)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return BackupRecord{}, err
		}
		if info.IsDir() {
			if err := copyTree(source, filepath.Join(destination, filepath.Base(source)), dirMode, fileMode); err != nil {
				return BackupRecord{}, err
			}
			continue
		}
		contents, err := t.ops.ReadFile(source)
		if err != nil {
			return BackupRecord{}, err
		}
		target := filepath.Join(destination, filepath.Base(source))
		if err := t.ops.WriteFile(target, contents, fileMode); err != nil {
			return BackupRecord{}, err
		}
	}
	files, err := t.inventory(destination)
	if err != nil {
		return BackupRecord{}, err
	}
	record.Files = files
	record.Checksum = manifestChecksum(files)
	record.Checksum = manifestChecksum(files)
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return BackupRecord{}, err
	}
	if err := t.ops.WriteFile(filepath.Join(destination, "backup.json"), append(encoded, '\n'), fileMode); err != nil {
		return BackupRecord{}, err
	}
	// A backup only counts once every file has been re-read and verified.
	if err := t.VerifyBackup(destination); err != nil {
		_ = t.ops.RemoveAll(destination)
		return BackupRecord{}, err
	}
	return record, nil
}

// VerifyBackup re-reads a backup and rejects any mismatch.
func (t *Tool) VerifyBackup(backupDir string) error {
	contents, err := t.ops.ReadFile(filepath.Join(backupDir, "backup.json"))
	if err != nil {
		return fmt.Errorf("%w: backup manifest unreadable: %v", ErrManifestMismatch, err)
	}
	var record BackupRecord
	if err := json.Unmarshal(contents, &record); err != nil {
		return fmt.Errorf("%w: backup manifest invalid: %v", ErrManifestMismatch, err)
	}
	if record.Schema != backupSchema {
		return fmt.Errorf("%w: unknown backup schema %q", ErrManifestMismatch, record.Schema)
	}
	actual, err := t.inventory(backupDir)
	if err != nil {
		return err
	}
	if actual == nil {
		actual = []ManifestFile{}
	}
	if manifestChecksum(actual) != record.Checksum || len(actual) != len(record.Files) {
		return fmt.Errorf("%w: backup content does not match its manifest", ErrManifestMismatch)
	}
	return nil
}

// RestoreBackup loads a verified backup into staging so the normal journaled
// promotion path performs the switch. It never writes in place.
func (t *Tool) RestoreBackup(backupDir string) (Manifest, error) {
	if err := t.VerifyBackup(backupDir); err != nil {
		return Manifest{}, err
	}
	stateSource := filepath.Join(backupDir, "state")
	if _, err := t.ops.Stat(stateSource); err != nil {
		return Manifest{}, fmt.Errorf("backup does not contain state content: %w", err)
	}
	if err := t.ops.RemoveAll(t.layout.StagingDir()); err != nil {
		return Manifest{}, err
	}
	if err := copyTree(stateSource, t.layout.StagingDir(), dirMode, fileMode); err != nil {
		return Manifest{}, err
	}
	manifest, err := buildManifest(t.layout.StagingDir())
	if err != nil {
		return Manifest{}, err
	}
	if err := t.writeManifest(manifest); err != nil {
		return Manifest{}, err
	}
	journal, err := t.loadJournal()
	if err != nil {
		return Manifest{}, err
	}
	journal.State = StateStaged
	journal.ManifestChecksum = manifest.Checksum
	if err := t.writeJournal(journal); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// PruneBackups keeps the newest keep backups and removes older ones. It is
// only called by an explicit operator action; nothing prunes automatically.
func (t *Tool) PruneBackups(destinationRoot string, keep int) ([]string, error) {
	if keep < 0 {
		return nil, errors.New("keep must not be negative")
	}
	instanceRoot := filepath.Join(destinationRoot, string(t.layout.Instance))
	entries, err := t.ops.ReadDir(instanceRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	// A directory that is not a verifiable backup never counts toward keep and
	// is left alone; and at least the newest verified backup survives any prune
	// (keep=0 therefore means "keep one", never "delete everything").
	var removable []string
	for _, name := range names {
		target := filepath.Join(instanceRoot, name)
		if err := t.VerifyBackup(target); err != nil {
			continue
		}
		removable = append(removable, name)
	}
	if len(removable) == 0 {
		return nil, nil
	}
	// removable is descending (newest first).
	keepFromNewest := keep
	if keepFromNewest < 1 {
		keepFromNewest = 1
	}
	var removed []string
	for index, name := range removable {
		if index < keepFromNewest {
			continue
		}
		target := filepath.Join(instanceRoot, name)
		if err := t.ops.RemoveAll(target); err != nil {
			return removed, err
		}
		removed = append(removed, target)
	}
	return removed, nil
}

func (t *Tool) inventory(root string) ([]ManifestFile, error) {
	manifest, err := buildManifest(root)
	if err != nil {
		return nil, err
	}
	files := make([]ManifestFile, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		// The backup manifest cannot checksum itself.
		if filepath.Base(file.Path) == "backup.json" {
			continue
		}
		files = append(files, file)
	}
	return files, nil
}
