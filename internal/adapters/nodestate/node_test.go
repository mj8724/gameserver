package nodestate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mj8724/gameserver/internal/ports"
)

// The identity is generated on first use and its secret half stays 0600 and
// outside every read path.
func TestIdentityKeyFileIsPrivate(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := store.Identity(context.Background())
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if identity.NodeID == "" || identity.Fingerprint == "" || identity.PublicKey == "" {
		t.Fatalf("identity incomplete: %+v", identity)
	}
	if identity.PrivateKeyPath == "" {
		t.Fatal("identity must report where the secret lives")
	}
	info, err := os.Stat(identity.PrivateKeyPath)
	if err != nil {
		t.Fatalf("secret file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secret file mode = %v, want 0600", info.Mode().Perm())
	}
	// Reading twice returns the same identity (no silent regeneration).
	again, err := store.Identity(context.Background())
	if err != nil || again.Fingerprint != identity.Fingerprint {
		t.Fatalf("identity must be stable: %v %+v", err, again)
	}
}

// Rotation keeps the node id but changes the key material.
func TestRotateChangesKeyKeepsNodeID(t *testing.T) {
	store, _ := New(t.TempDir())
	before, _ := store.Identity(context.Background())
	after, err := store.Rotate(context.Background())
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if after.NodeID != before.NodeID {
		t.Fatalf("rotation must keep the node id: %s -> %s", before.NodeID, after.NodeID)
	}
	if after.Fingerprint == before.Fingerprint || after.PublicKey == before.PublicKey {
		t.Fatal("rotation must change the key material")
	}
	if after.RotatedAt.IsZero() {
		t.Fatal("rotation must be recorded")
	}
}

// Authorization fails closed for unknown, revoked and mismatched peers.
func TestAuthorizeFailsClosed(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	if err := store.Authorize(ctx, "node-x", "fp"); !errors.Is(err, ports.ErrNodeUnknown) {
		t.Fatalf("unknown node = %v, want ErrNodeUnknown", err)
	}
	if err := store.Register(ctx, ports.RegisteredNode{NodeID: "node-x", Fingerprint: "fp-1", PublicKey: "pk"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize(ctx, "node-x", "fp-1"); err != nil {
		t.Fatalf("registered node must be authorized: %v", err)
	}
	if err := store.Authorize(ctx, "node-x", "fp-2"); err == nil {
		t.Fatal("fingerprint mismatch must be refused")
	}
	if err := store.Revoke(ctx, "node-x"); err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize(ctx, "node-x", "fp-1"); !errors.Is(err, ports.ErrNodeRevoked) {
		t.Fatalf("revoked node = %v, want ErrNodeRevoked", err)
	}
	nodes, err := store.List(ctx)
	if err != nil || len(nodes) != 1 || nodes[0].RevokedAt.IsZero() {
		t.Fatalf("revoked node must stay listed for audit: %+v %v", nodes, err)
	}
	if err := store.Revoke(ctx, "node-y"); !errors.Is(err, ports.ErrNodeUnknown) {
		t.Fatalf("revoking unknown = %v, want ErrNodeUnknown", err)
	}
}

// A replayed request id returns the stored result instead of running again.
func TestLedgerReplayReturnsStoredResult(t *testing.T) {
	store, _ := New(t.TempDir())
	ledger := store.Ledger()
	ctx := context.Background()
	record := ports.TaskRecord{RequestID: "req-1", NodeID: "node-x", Operation: "start", InputHash: "h1"}

	_, existed, err := ledger.Begin(ctx, record)
	if err != nil || existed {
		t.Fatalf("first begin = %v existed=%v", err, existed)
	}
	if err := ledger.Complete(ctx, "req-1", "started", ""); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, existed, err := ledger.Begin(ctx, record)
	if err != nil || !existed {
		t.Fatalf("replay must report the existing record: %v existed=%v", err, existed)
	}
	if got.State != ports.TaskCompleted || got.Result != "started" {
		t.Fatalf("replay must return the stored terminal result: %+v", got)
	}
}

// The same request id with different input is a conflict (never a silent
// second execution with different parameters).
func TestLedgerRejectsReusedRequestIDWithDifferentInput(t *testing.T) {
	store, _ := New(t.TempDir())
	ledger := store.Ledger()
	ctx := context.Background()
	if _, _, err := ledger.Begin(ctx, ports.TaskRecord{RequestID: "req-2", Operation: "start", InputHash: "h1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ledger.Begin(ctx, ports.TaskRecord{RequestID: "req-2", Operation: "stop", InputHash: "h2"}); !errors.Is(err, ports.ErrTaskConflict) {
		t.Fatalf("conflicting reuse = %v, want ErrTaskConflict", err)
	}
}

// Completing twice never overwrites the recorded outcome, and a failed task is
// recorded as failed with its error text.
func TestLedgerCompleteIsIdempotent(t *testing.T) {
	store, _ := New(t.TempDir())
	ledger := store.Ledger()
	ctx := context.Background()
	if _, _, err := ledger.Begin(ctx, ports.TaskRecord{RequestID: "req-3", Operation: "install", InputHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Complete(ctx, "req-3", "", "network unreachable"); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Complete(ctx, "req-3", "overwritten", ""); err != nil {
		t.Fatal(err)
	}
	got, err := ledger.Get(ctx, "req-3")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != ports.TaskFailed || got.Error != "network unreachable" || got.Result != "" {
		t.Fatalf("terminal state must not be overwritten: %+v", got)
	}
	if _, err := ledger.Get(ctx, "missing"); err == nil {
		t.Fatal("unknown request id must be an error")
	}
}

// The ledger survives a restart (durable idempotency) and tolerates a clock
// that moves backwards between records.
func TestLedgerSurvivesRestartAndClockSkew(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	ctx := context.Background()
	if _, _, err := store.Ledger().Begin(ctx, ports.TaskRecord{RequestID: "req-4", Operation: "start", InputHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Ledger().Complete(ctx, "req-4", "ok", ""); err != nil {
		t.Fatal(err)
	}
	// A fresh store over the same directory sees the record.
	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Ledger().Get(ctx, "req-4")
	if err != nil || got.State != ports.TaskCompleted {
		t.Fatalf("ledger must survive restart: %+v %v", got, err)
	}
	// Skewed (earlier) timestamps are accepted and do not corrupt ordering.
	if _, _, err := reopened.Ledger().Begin(ctx, ports.TaskRecord{RequestID: "req-5", Operation: "stop", InputHash: "h", CreatedAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatalf("clock skew must not break the ledger: %v", err)
	}
	records, err := reopened.Ledger().List(ctx)
	if err != nil || len(records) != 2 {
		t.Fatalf("expected two records: %+v %v", records, err)
	}
	if _, err := os.Stat(filepath.Join(root, ledgerFile)); err != nil {
		t.Fatalf("ledger file missing: %v", err)
	}
}

// A task left pending by a disconnected node is reconciled to interrupted, and
// a replay then returns that terminal outcome instead of executing again.
func TestLedgerReconcileClosesDisconnectedTask(t *testing.T) {
	store, _ := New(t.TempDir())
	ledger := store.Ledger()
	ctx := context.Background()
	if _, _, err := ledger.Begin(ctx, ports.TaskRecord{RequestID: "req-d1", NodeID: "node-x", Operation: "start", InputHash: "h"}); err != nil {
		t.Fatal(err)
	}
	// Inside the grace window nothing is touched.
	affected, err := ledger.Reconcile(ctx, time.Hour)
	if err != nil || len(affected) != 0 {
		t.Fatalf("inside grace nothing reconciles: %+v %v", affected, err)
	}
	// After the grace window the pending task becomes interrupted.
	affected, err = ledger.Reconcile(ctx, time.Nanosecond)
	if err != nil || len(affected) != 1 {
		t.Fatalf("pending task must reconcile: %+v %v", affected, err)
	}
	if affected[0].State != ports.TaskInterrupted || affected[0].Error == "" {
		t.Fatalf("reconciled record incomplete: %+v", affected[0])
	}
	// Replaying the same request id reports the reconciled state, not a new run.
	got, existed, err := ledger.Begin(ctx, ports.TaskRecord{RequestID: "req-d1", NodeID: "node-x", Operation: "start", InputHash: "h"})
	if err != nil || !existed || got.State != ports.TaskInterrupted {
		t.Fatalf("replay after reconciliation = %+v existed=%v err=%v", got, existed, err)
	}
	// Reconciling twice is a no-op (terminal states are never rewritten).
	again, err := ledger.Reconcile(ctx, time.Nanosecond)
	if err != nil || len(again) != 0 {
		t.Fatalf("second reconcile must be empty: %+v %v", again, err)
	}
}

// Reconciliation survives a restart (the interrupted state is durable).
func TestLedgerReconcileIsDurable(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	if _, _, err := store.Ledger().Begin(context.Background(), ports.TaskRecord{RequestID: "req-d2", Operation: "install", InputHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Ledger().Reconcile(context.Background(), time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	reopened, _ := New(root)
	record, err := reopened.Ledger().Get(context.Background(), "req-d2")
	if err != nil || record.State != ports.TaskInterrupted {
		t.Fatalf("interrupted state must be durable: %+v %v", record, err)
	}
}
