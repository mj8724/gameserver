package migrate

import (
	"errors"
	"os"
	"path/filepath"
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
