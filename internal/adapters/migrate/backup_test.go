package migrate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupVerifyTamperAndRestoreThroughPromotion(t *testing.T) {
	tool := newTool(t)
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": `{"generation":"one"}`})
	writeTree(t, filepath.Join(layout.InstanceDir(), "Zomboid", "Server"), map[string]string{"pz.ini": "Password=\n"})
	writeTree(t, filepath.Join(layout.InstanceDir(), "server_files"), map[string]string{"big.bin": "must not be backed up"})

	backups := filepath.Join(t.TempDir(), "backups")
	record, err := tool.Backup(backups)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if record.Checksum == "" || len(record.Files) == 0 {
		t.Fatalf("unexpected backup record: %+v", record)
	}
	if _, err := os.Stat(filepath.Join(record.Path, "state", "instance.json")); err != nil {
		t.Fatalf("state file missing from backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(record.Path, "pz.ini")); err != nil {
		t.Fatalf("INI file missing from backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(record.Path, "big.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("server_files must not be backed up, stat err = %v", err)
	}
	if err := tool.VerifyBackup(record.Path); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// A single tampered byte must be rejected.
	victim := filepath.Join(record.Path, "state", "instance.json")
	if err := os.WriteFile(victim, []byte(`{"generation":"evil"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := tool.VerifyBackup(record.Path); !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("tampered backup verify = %v, want ErrManifestMismatch", err)
	}
	if _, err := tool.RestoreBackup(record.Path); !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("tampered restore = %v, want ErrManifestMismatch", err)
	}

	// Repair the byte and restore through the journaled promotion path.
	if err := os.WriteFile(victim, []byte(`{"generation":"one"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.RestoreBackup(record.Path); err != nil {
		t.Fatalf("restore: %v", err)
	}
	result, err := tool.Promote()
	if err != nil {
		t.Fatalf("promote after restore: %v", err)
	}
	if result.Action != "committed" {
		t.Fatalf("unexpected promote result: %+v", result)
	}
	contents, err := os.ReadFile(filepath.Join(layout.StateDir(), "instance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != `{"generation":"one"}` {
		t.Fatalf("restored state = %s", contents)
	}
}

func TestPruneBackupsKeepsNewestAndRequiresExplicitCall(t *testing.T) {
	tool := newTool(t)
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": "{}"})
	backups := filepath.Join(t.TempDir(), "backups")

	clock := fixedClock()
	created := make([]string, 0, 4)
	for index := 0; index < 4; index++ {
		indexed, err := New(layout, WithClock(func() time.Time { return clock }))
		if err != nil {
			t.Fatal(err)
		}
		record, err := indexed.Backup(backups)
		if err != nil {
			t.Fatalf("backup %d: %v", index, err)
		}
		created = append(created, record.Path)
		clock = clock.Add(time.Second)
	}
	removed, err := tool.PruneBackups(backups, 3)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("prune removed %d backups, want 1", len(removed))
	}
	if _, err := os.Stat(created[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oldest backup should be removed, stat err = %v", err)
	}
	for _, path := range created[1:] {
		if err := tool.VerifyBackup(path); err != nil {
			t.Fatalf("remaining backup %s failed verify: %v", path, err)
		}
	}
}

// ADR §4.6 (r3): the sandbox file is a managed file, so the default backup
// range must carry it next to the INI files.
func TestBackupIncludesSandboxVars(t *testing.T) {
	tool := newTool(t)
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": `{"instance_id":"pz_01"}`})
	writeTree(t, filepath.Join(layout.InstanceDir(), "Zomboid", "Server"), map[string]string{
		"servertest.ini":             "Public=true\n",
		"servertest_SandboxVars.lua": "SandboxVars = {\n    Zombies = 4,\n}\n",
	})

	record, err := tool.Backup(filepath.Join(t.TempDir(), "backups"))
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	found := false
	for _, file := range record.Files {
		if strings.HasSuffix(file.Path, "servertest_SandboxVars.lua") {
			found = true
		}
	}
	if !found {
		t.Fatalf("sandbox file missing from the backup manifest: %+v", record.Files)
	}
}

// The M3.2 retention floor: prune never removes the newest verified backup,
// even when the configured keep is zero or negative.
func TestPruneNeverRemovesNewestVerifiedBackup(t *testing.T) {
	tool := newTool(t)
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": "{}"})
	backups := filepath.Join(t.TempDir(), "backups")

	for index := 0; index < 2; index++ {
		clock := fixedClock().Add(time.Duration(index) * time.Second)
		indexed, err := New(layout, WithClock(func() time.Time { return clock }))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := indexed.Backup(backups); err != nil {
			t.Fatalf("backup %d: %v", index, err)
		}
	}
	removed, err := tool.PruneBackups(backups, 0)
	if err != nil {
		t.Fatalf("prune keep=0: %v", err)
	}
	// keep=0 means keep one: exactly one backup is removed, never all.
	if len(removed) != 1 {
		t.Fatalf("prune removed %d backups, want exactly 1 (the oldest)", len(removed))
	}
	entries, err := os.ReadDir(filepath.Join(backups, string(layout.Instance)))
	if err != nil {
		t.Fatal(err)
	}
	var verified int
	for _, entry := range entries {
		if err := tool.VerifyBackup(filepath.Join(backups, string(layout.Instance), entry.Name())); err == nil {
			verified++
		}
	}
	if verified != 1 {
		t.Fatalf("exactly one verified backup must survive, got %d", verified)
	}
}

// An opt-in backup includes the save games and records the include mode, so a
// restore can later claim exactly what the backup holds (M3.2 decision).
func TestBackupWithSavesRecordsIncludeMode(t *testing.T) {
	tool := newTool(t)
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": "{}"})
	savesRoot := filepath.Join(layout.InstanceDir(), "Zomboid", "Saves", "servertest")
	writeTree(t, savesRoot, map[string]string{"map.bin": "world-data"})

	record, err := tool.BackupWithOptions(t.TempDir(), BackupOptions{IncludeSaves: true})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if len(record.Includes) != 1 || record.Includes[0] != "saves" {
		t.Fatalf("include mode not recorded: %+v", record.Includes)
	}
	found := false
	for _, file := range record.Files {
		if strings.Contains(file.Path, "Saves") {
			found = true
		}
	}
	if !found {
		t.Fatalf("backup files do not contain the saves tree: %+v", record.Files)
	}
}

// The default backup range still excludes saves: opting in never becomes the
// silent default (ADR §4.6, M3.2 decision).
func TestDefaultBackupExcludesSaves(t *testing.T) {
	tool := newTool(t)
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": "{}"})
	writeTree(t, filepath.Join(layout.InstanceDir(), "Zomboid", "Saves", "servertest"), map[string]string{"map.bin": "world-data"})

	record, err := tool.Backup(t.TempDir())
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if len(record.Includes) != 0 {
		t.Fatalf("default backup must not opt into saves: %+v", record.Includes)
	}
	for _, file := range record.Files {
		if strings.Contains(file.Path, "Saves") {
			t.Fatalf("default backup must exclude saves, found %s", file.Path)
		}
	}
}

// The M3.2 automatic-backup adoption: BackupNow backs up including saves and
// reports a valid checksum; the retention floor keeps at least one backup.
func TestAutoBackupBackupNowIncludesSavesAndPrunes(t *testing.T) {
	tool := newTool(t)
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": "{}"})
	writeTree(t, filepath.Join(layout.InstanceDir(), "Zomboid", "Saves", "servertest"), map[string]string{"map.bin": "world"})
	dest := filepath.Join(t.TempDir(), "backups")

	auto := NewAutoBackup(tool, dest, true)
	result, err := auto.BackupNow()
	if err != nil {
		t.Fatalf("BackupNow: %v", err)
	}
	if result.Checksum == "" || result.Files == 0 || result.Path == "" {
		t.Fatalf("BackupNow result incomplete: %+v", result)
	}
	if err := tool.VerifyBackup(result.Path); err != nil {
		t.Fatalf("auto backup must verify: %v", err)
	}
	if !auto.Enabled() {
		t.Fatal("enabled flag must be honoured")
	}
}
