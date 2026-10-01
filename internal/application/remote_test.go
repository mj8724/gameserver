package application

import (
	"context"
	"errors"
	"testing"

	"github.com/mj8724/gameserver/internal/ports"
)

type fakeNodes struct {
	authorizeErr error
	rotated      int
	registry     map[string]string
}

func (f *fakeNodes) Identity(context.Context) (ports.NodeIdentity, error) {
	return ports.NodeIdentity{NodeID: "node-local"}, nil
}
func (f *fakeNodes) Rotate(context.Context) (ports.NodeIdentity, error) {
	f.rotated++
	return ports.NodeIdentity{NodeID: "node-local", Fingerprint: "fp-new"}, nil
}
func (f *fakeNodes) Register(_ context.Context, node ports.RegisteredNode) error {
	if f.registry == nil {
		f.registry = map[string]string{}
	}
	f.registry[node.NodeID] = node.Fingerprint
	return nil
}
func (f *fakeNodes) Revoke(context.Context, string) error { return nil }

// Authorize models the production adapter's pinning rule; an empty registry
// means "no pinning configured" so unrelated remote tests keep their intent,
// while the upgrade/rollback drill registers explicitly to exercise pinning.
func (f *fakeNodes) Authorize(_ context.Context, nodeID, fingerprint string) error {
	if f.authorizeErr != nil {
		return f.authorizeErr
	}
	if pinned, ok := f.registry[nodeID]; ok {
		if pinned != fingerprint {
			return ports.ErrNodeUnknown
		}
	}
	return nil
}
func (f *fakeNodes) List(context.Context) ([]ports.RegisteredNode, error) {
	return []ports.RegisteredNode{}, nil
}

type fakeAudit struct{ entries []ports.AuditEntry }

func (f *fakeAudit) Append(_ context.Context, entry ports.AuditEntry) error {
	f.entries = append(f.entries, entry)
	return nil
}
func (f *fakeAudit) Recent(context.Context, int) ([]ports.AuditEntry, error) {
	return f.entries, nil
}

type fakeLedger struct {
	records map[string]ports.TaskRecord
}

func (f *fakeLedger) Begin(_ context.Context, record ports.TaskRecord) (ports.TaskRecord, bool, error) {
	if f.records == nil {
		f.records = map[string]ports.TaskRecord{}
	}
	if existing, ok := f.records[record.RequestID]; ok {
		if existing.InputHash != record.InputHash || existing.Operation != record.Operation {
			return ports.TaskRecord{}, false, ports.ErrTaskConflict
		}
		return existing, true, nil
	}
	record.State = ports.TaskPending
	f.records[record.RequestID] = record
	return record, false, nil
}
func (f *fakeLedger) Complete(_ context.Context, requestID, result, errText string) error {
	record := f.records[requestID]
	if errText != "" {
		record.State, record.Error = ports.TaskFailed, errText
	} else {
		record.State, record.Result = ports.TaskCompleted, result
	}
	f.records[requestID] = record
	return nil
}
func (f *fakeLedger) Get(_ context.Context, requestID string) (ports.TaskRecord, error) {
	record, ok := f.records[requestID]
	if !ok {
		return ports.TaskRecord{}, errors.New("not found")
	}
	return record, nil
}
func (f *fakeLedger) List(context.Context) ([]ports.TaskRecord, error) {
	out := make([]ports.TaskRecord, 0, len(f.records))
	for _, record := range f.records {
		out = append(out, record)
	}
	return out, nil
}

func remoteHarness(t *testing.T) (*harness, *fakeAudit, *fakeLedger) {
	t.Helper()
	h := newHarness(t)
	audit := &fakeAudit{}
	ledger := &fakeLedger{}
	h.service.deps.Nodes = &fakeNodes{}
	h.service.deps.Audit = audit
	h.service.deps.Tasks = ledger
	return h, audit, ledger
}

// Only allowlisted operations may be requested; anything else is refused and
// audited. There is no generic command channel.
func TestRemoteRejectsArbitraryOperation(t *testing.T) {
	h, audit, _ := remoteHarness(t)
	_, err := h.service.ExecuteRemote(context.Background(), RemoteRequest{
		NodeID: "node-x", Fingerprint: "fp", Operator: "ops", RequestID: "r1", Operation: RemoteOperation("sh -c rm -rf /"),
	})
	if err == nil {
		t.Fatal("arbitrary operation must be refused")
	}
	if len(audit.entries) != 1 || audit.entries[0].Outcome != "rejected" {
		t.Fatalf("refusal must be audited: %+v", audit.entries)
	}
}

// Revoked/unknown nodes are refused before any execution and the attempt is
// recorded with operator, node, operation and outcome.
func TestRemoteUnauthorizedNodeIsAudited(t *testing.T) {
	h, audit, _ := remoteHarness(t)
	h.service.deps.Nodes = &fakeNodes{authorizeErr: ports.ErrNodeRevoked}
	_, err := h.service.ExecuteRemote(context.Background(), RemoteRequest{
		NodeID: "node-x", Fingerprint: "fp", Operator: "ops", RequestID: "r2", Operation: RemoteStop,
	})
	if err == nil {
		t.Fatal("revoked node must be refused")
	}
	if len(audit.entries) != 1 || audit.entries[0].Outcome != "unauthorized" || audit.entries[0].NodeID != "node-x" {
		t.Fatalf("unauthorized attempt must be audited with identity: %+v", audit.entries)
	}
}

// A replayed request id reports the stored outcome without executing again.
func TestRemoteReplayDoesNotExecuteAgain(t *testing.T) {
	h, audit, ledger := remoteHarness(t)
	request := RemoteRequest{NodeID: "node-x", Fingerprint: "fp", Operator: "ops", RequestID: "r3", Operation: RemoteStatus}
	first, err := h.service.ExecuteRemote(context.Background(), request)
	if err != nil {
		t.Fatalf("first execution: %v", err)
	}
	if !first.Executed {
		t.Fatal("first execution must report executed=true")
	}
	second, err := h.service.ExecuteRemote(context.Background(), request)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if second.Executed {
		t.Fatal("replay must not execute again")
	}
	if second.Outcome != ports.TaskCompleted {
		t.Fatalf("replay must report the stored outcome: %+v", second)
	}
	outcomes := []string{}
	for _, entry := range audit.entries {
		outcomes = append(outcomes, entry.Outcome)
	}
	if len(outcomes) != 2 || outcomes[1] != "replayed" {
		t.Fatalf("both attempts must be audited: %v", outcomes)
	}
	// A different input under the same request id is a conflict.
	conflicting := request
	conflicting.Operation = RemoteStart
	if _, err := h.service.ExecuteRemote(context.Background(), conflicting); err == nil {
		t.Fatal("request id reuse with different input must be refused")
	}
	if ledger.records["r3"].State != ports.TaskCompleted {
		t.Fatalf("the original record must stay intact: %+v", ledger.records["r3"])
	}
}

// Every audit entry carries operator, node, operation, request id and time.
func TestRemoteAuditEntryIsComplete(t *testing.T) {
	h, audit, _ := remoteHarness(t)
	if _, err := h.service.ExecuteRemote(context.Background(), RemoteRequest{
		NodeID: "node-y", Fingerprint: "fp", Operator: "alice", RequestID: "r4", Operation: RemoteStop,
	}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	entry := audit.entries[len(audit.entries)-1]
	if entry.Operator != "alice" || entry.NodeID != "node-y" || entry.Operation != "stop" || entry.RequestID != "r4" {
		t.Fatalf("audit entry incomplete: %+v", entry)
	}
	if entry.At.IsZero() || entry.InputHash == "" {
		t.Fatalf("audit entry must carry a timestamp and input hash: %+v", entry)
	}
}

// Per-instance authorization: a remote request naming another instance is
// refused before anything runs, and the refusal is audited.
func TestRemoteRejectsForeignInstance(t *testing.T) {
	h, audit, _ := remoteHarness(t)
	_, err := h.service.ExecuteRemote(context.Background(), RemoteRequest{
		NodeID: "node-x", Fingerprint: "fp", Operator: "ops", RequestID: "r-f1",
		Operation: RemoteStatus, InstanceID: "valheim_01",
	})
	if err == nil {
		t.Fatal("a foreign instance must be refused")
	}
	if len(audit.entries) == 0 || audit.entries[len(audit.entries)-1].Outcome != "instance_not_authorized" {
		t.Fatalf("refusal must be audited: %+v", audit.entries)
	}
	// The active instance (explicit or empty) is allowed.
	if _, err := h.service.ExecuteRemote(context.Background(), RemoteRequest{
		NodeID: "node-x", Fingerprint: "fp", Operator: "ops", RequestID: "r-f2",
		Operation: RemoteStatus, InstanceID: "pz_01",
	}); err != nil {
		t.Fatalf("the active instance must be allowed: %v", err)
	}
	if _, err := h.service.ExecuteRemote(context.Background(), RemoteRequest{
		NodeID: "node-x", Fingerprint: "fp", Operator: "ops", RequestID: "r-f3", Operation: RemoteStatus,
	}); err != nil {
		t.Fatalf("an empty instance id means the active instance: %v", err)
	}
}
