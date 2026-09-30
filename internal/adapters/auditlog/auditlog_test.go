package auditlog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mj8724/gameserver/internal/ports"
)

// Records are appended, never rewritten, and the file stays 0600.
func TestAuditAppendOnly(t *testing.T) {
	root := t.TempDir()
	log, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for index := 0; index < 3; index++ {
		if err := log.Append(ctx, ports.AuditEntry{At: time.Now().UTC(), Operator: "ops", NodeID: "node-x", Operation: "start", Outcome: "executed"}); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(filepath.Join(root, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("audit file mode = %v, want 0600", info.Mode().Perm())
	}
	entries, err := log.Recent(ctx, 0)
	if err != nil || len(entries) != 3 {
		t.Fatalf("expected three records: %+v %v", entries, err)
	}
	if entries[0].Operator != "ops" || entries[0].Operation != "start" {
		t.Fatalf("record content incomplete: %+v", entries[0])
	}
	// Appending again must not rewrite earlier records.
	if err := log.Append(ctx, ports.AuditEntry{At: time.Now().UTC(), Operation: "stop", Outcome: "executed"}); err != nil {
		t.Fatal(err)
	}
	entries, _ = log.Recent(ctx, 0)
	if len(entries) != 4 || entries[0].Operation != "start" {
		t.Fatalf("append must preserve history: %+v", entries)
	}
}

// Recent returns the newest records, and a corrupt trail fails closed.
func TestAuditRecentLimitAndCorruption(t *testing.T) {
	root := t.TempDir()
	log, _ := New(root)
	ctx := context.Background()
	for index := 0; index < 5; index++ {
		_ = log.Append(ctx, ports.AuditEntry{At: time.Now().UTC(), Operation: "start", Outcome: "executed"})
	}
	entries, err := log.Recent(ctx, 2)
	if err != nil || len(entries) != 2 {
		t.Fatalf("limit not applied: %+v %v", entries, err)
	}
	// A damaged record is reported rather than silently skipped.
	file, _ := os.OpenFile(filepath.Join(root, fileName), os.O_APPEND|os.O_WRONLY, 0o600)
	file.WriteString("{not json}\n")
	file.Close()
	if _, err := log.Recent(ctx, 0); err == nil {
		t.Fatal("a corrupt audit trail must fail closed")
	}
	if !strings.Contains(filepath.Base(filepath.Join(root, fileName)), "audit") {
		t.Fatal("unexpected audit file name")
	}
}

// A missing trail reads as empty (a fresh node has nothing to show).
func TestAuditMissingFileIsEmpty(t *testing.T) {
	log, _ := New(t.TempDir())
	entries, err := log.Recent(context.Background(), 10)
	if err != nil || len(entries) != 0 {
		t.Fatalf("missing trail must read empty: %+v %v", entries, err)
	}
}
