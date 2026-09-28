package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// realOps performs real filesystem effects.
type realOps struct{}

func (realOps) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }

func (realOps) MkdirAll(path string, mode os.FileMode) error { return os.MkdirAll(path, mode) }

func (realOps) RemoveAll(path string) error { return os.RemoveAll(path) }

func (realOps) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

func (realOps) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (realOps) WriteFile(path string, data []byte, mode os.FileMode) error {
	return os.WriteFile(path, data, mode)
}

func (realOps) WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".migrate-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
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
	return os.Rename(tempName, path)
}

func (realOps) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }

// hashFile returns the SHA-256 of one file.
func hashFile(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:]), nil
}

// buildManifest inventories every regular file under root.
func buildManifest(root string) (Manifest, error) {
	manifest := Manifest{Schema: manifestSchema, Version: "1"}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("state content must not contain symlinks: %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		checksum, err := hashFile(path)
		if err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, ManifestFile{
			Path:   filepath.ToSlash(relative),
			Size:   info.Size(),
			SHA256: checksum,
			Mode:   uint32(info.Mode().Perm()),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manifest, nil
		}
		return Manifest{}, err
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	manifest.Checksum = manifestChecksum(manifest.Files)
	return manifest, nil
}

func manifestChecksum(files []ManifestFile) string {
	hash := sha256.New()
	for _, file := range files {
		_, _ = hash.Write([]byte(file.Path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(file.SHA256))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (t *Tool) readManifest() (Manifest, error) {
	contents, err := t.ops.ReadFile(t.layout.ManifestPath())
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.Schema != manifestSchema {
		return Manifest{}, fmt.Errorf("unknown manifest schema %q", manifest.Schema)
	}
	return manifest, nil
}

func (t *Tool) writeManifest(manifest Manifest) error {
	if err := t.ops.MkdirAll(t.layout.MigrationDir(), dirMode); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return t.ops.WriteFileAtomic(t.layout.ManifestPath(), append(encoded, '\n'), fileMode)
}

// verifyTree checks that a directory matches the manifest exactly.
func (t *Tool) verifyTree(root string, manifest Manifest) error {
	actual, err := buildManifest(root)
	if err != nil {
		return err
	}
	if actual.Checksum != manifest.Checksum || len(actual.Files) != len(manifest.Files) {
		return ErrManifestMismatch
	}
	return nil
}

func (t *Tool) verifyStaging() (Manifest, error) {
	manifest, err := t.readManifest()
	if err != nil {
		return Manifest{}, err
	}
	if err := t.verifyTree(t.layout.StagingDir(), manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// DryRun inventories a source state directory without writing anything.
func (t *Tool) DryRun(sourceDir string) (Manifest, error) {
	info, err := t.ops.Stat(sourceDir)
	if err != nil {
		return Manifest{}, fmt.Errorf("read source: %w", err)
	}
	if !info.IsDir() {
		return Manifest{}, errors.New("source must be a directory")
	}
	return buildManifest(sourceDir)
}

// Import copies source state content into staging and records STAGED.
// The source directory is only read.
func (t *Tool) Import(sourceDir string) (Manifest, error) {
	expected, err := t.DryRun(sourceDir)
	if err != nil {
		return Manifest{}, err
	}
	if err := t.ops.RemoveAll(t.layout.StagingDir()); err != nil {
		return Manifest{}, err
	}
	if err := t.ops.MkdirAll(t.layout.StagingDir(), dirMode); err != nil {
		return Manifest{}, err
	}
	if err := copyTree(sourceDir, t.layout.StagingDir(), dirMode, fileMode); err != nil {
		return Manifest{}, err
	}
	staged, err := buildManifest(t.layout.StagingDir())
	if err != nil {
		return Manifest{}, err
	}
	if staged.Checksum != expected.Checksum {
		return Manifest{}, fmt.Errorf("%w: source changed during import", ErrManifestMismatch)
	}
	if err := t.writeManifest(staged); err != nil {
		return Manifest{}, err
	}
	journal, err := t.loadJournal()
	if err != nil {
		return Manifest{}, err
	}
	if !t.exists(t.layout.StateDir()) && journal.State == StateIdle {
		journal.State = StateExported
	}
	journal.State = StateStaged
	journal.ManifestChecksum = staged.Checksum
	if err := t.writeJournal(journal); err != nil {
		return Manifest{}, err
	}
	return staged, nil
}

func copyTree(source, target string, dirMode, fileMode os.FileMode) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, dirMode)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source must not contain symlinks: %s", path)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), dirMode); err != nil {
			return err
		}
		return os.WriteFile(destination, contents, fileMode)
	})
}

// Promote runs the journaled switch from STAGED/VERIFIED to COMMITTED, or
// fails closed. It never deletes a live side of the migration site.
func (t *Tool) Promote() (RecoverResult, error) {
	snapshot, err := t.Inspect()
	if err != nil {
		return RecoverResult{}, err
	}
	journal := snapshot.Journal

	switch journal.State {
	case StateCommitted, StateRolledBack, StateLegacyActive:
		return RecoverResult{Journal: journal, Action: "noop"}, nil
	case StateCommitting, StateRollingBack:
		return t.Recover()
	}

	if !snapshot.StagingHere {
		return t.recoveryRequired(journal, "staging content missing")
	}
	manifest, err := t.verifyStaging()
	if err != nil {
		return t.recoveryRequired(journal, "staging failed verification")
	}
	journal.State = StateVerified
	journal.ManifestChecksum = manifest.Checksum
	if err := t.writeJournal(journal); err != nil {
		return RecoverResult{}, err
	}

	// COMMITTING must be durable before the first rename.
	journal.State = StateCommitting
	if err := t.writeJournal(journal); err != nil {
		return RecoverResult{}, err
	}

	if err := t.stepOne(&journal); err != nil {
		return t.recoveryRequired(journal, fmt.Sprintf("step 1 rename failed: %v", err))
	}
	if err := t.stepTwo(&manifest); err != nil {
		prevPath := t.layout.PrevDir(journal.Sequence)
		if rollbackErr := t.renameInto(prevPath, t.layout.StateDir()); rollbackErr != nil {
			return t.recoveryRequired(journal, fmt.Sprintf("step 2 and rollback failed: %v / %v", err, rollbackErr))
		}
		journal.State = StateRolledBack
		journal.LastRolledBackFrom = prevPath
		if writeErr := t.writeJournal(journal); writeErr != nil {
			return RecoverResult{}, writeErr
		}
		return RecoverResult{Journal: journal, Action: "rolled_back"}, nil
	}
	journal.State = StateCommitted
	if err := t.writeJournal(journal); err != nil {
		return RecoverResult{}, err
	}
	return RecoverResult{Journal: journal, Action: "committed"}, nil
}

// stepOne moves the current state subtree to the next free prev slot.
func (t *Tool) stepOne(journal *Journal) error {
	if !t.exists(t.layout.StateDir()) {
		return nil
	}
	sequence := journal.Sequence
	if sequence <= 0 {
		sequence = 1
	}
	for t.exists(t.layout.PrevDir(sequence)) {
		sequence++
	}
	if err := t.ops.MkdirAll(filepath.Dir(t.layout.PrevDir(sequence)), dirMode); err != nil {
		return err
	}
	if err := t.ops.Rename(t.layout.StateDir(), t.layout.PrevDir(sequence)); err != nil {
		return err
	}
	journal.Sequence = sequence
	return t.writeJournal(*journal)
}

// stepTwo moves staging into the active state path.
func (t *Tool) stepTwo(*Manifest) error {
	return t.renameInto(t.layout.StagingDir(), t.layout.StateDir())
}

// renameInto creates the destination parent so a first promotion into a
// missing instance directory still works.
func (t *Tool) renameInto(oldPath, newPath string) error {
	if err := t.ops.MkdirAll(filepath.Dir(newPath), dirMode); err != nil {
		return err
	}
	return t.ops.Rename(oldPath, newPath)
}

// InstanceRootStateFiles lists the migration-relevant relative paths inside an
// instance root, for diagnostics and evidence.
func InstanceRootStateFiles(instanceDir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(instanceDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "server_files" || entry.Name() == "Zomboid" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, relErr := filepath.Rel(instanceDir, path)
		if relErr != nil {
			return relErr
		}
		if strings.HasPrefix(filepath.ToSlash(relative), "state/") {
			paths = append(paths, filepath.ToSlash(relative))
		}
		return nil
	})
	sort.Strings(paths)
	return paths, err
}
