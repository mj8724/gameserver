package migrate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
)

func fixedClock() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) }

func newTool(t *testing.T, options ...Option) *Tool {
	t.Helper()
	serversRoot := filepath.Join(t.TempDir(), "servers")
	base := []Option{WithClock(fixedClock)}
	tool, err := New(Layout{ServersRoot: serversRoot, Instance: domain.InstanceID("pz_01")}, append(base, options...)...)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, contents := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func manifestOf(t *testing.T, dir string) Manifest {
	t.Helper()
	manifest, err := buildManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestDryRunWritesNothing(t *testing.T) {
	tool := newTool(t)
	source := filepath.Join(t.TempDir(), "converted-state")
	writeTree(t, source, map[string]string{"instance.json": `{"variables":{"SERVER_NAME":"a"}}`, "extra/notes.txt": "keep"})
	before := manifestOf(t, source)

	manifest, err := tool.DryRun(source)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Checksum != before.Checksum {
		t.Fatalf("dry-run checksum changed: %s vs %s", manifest.Checksum, before.Checksum)
	}
	if _, err := os.Stat(tool.Layout().MigrationDir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run must not create the migration dir, stat err = %v", err)
	}
	after := manifestOf(t, source)
	if after.Checksum != before.Checksum {
		t.Fatal("dry-run modified the source tree")
	}
}

func TestImportThenPromoteKeepsPrevAndLeavesOtherTreesAlone(t *testing.T) {
	tool := newTool(t)
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": `{"generation":"old"}`})
	writeTree(t, filepath.Join(layout.InstanceDir(), "Zomboid", "Server"), map[string]string{"pz.ini": "Password=\n"})
	writeTree(t, filepath.Join(layout.InstanceDir(), "server_files"), map[string]string{"big.bin": "server files"})
	source := filepath.Join(t.TempDir(), "converted-state")
	writeTree(t, source, map[string]string{"instance.json": `{"generation":"new"}`})
	sourceBefore := manifestOf(t, source)

	if _, err := tool.Import(source); err != nil {
		t.Fatalf("import: %v", err)
	}
	snapshot, err := tool.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Journal.State != StateStaged || !snapshot.StagingHere || snapshot.Blocked() != true {
		t.Fatalf("unexpected post-import snapshot: %+v blocked=%v", snapshot, snapshot.Blocked())
	}
	result, err := tool.Promote()
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if result.Action != "committed" || result.Journal.State != StateCommitted {
		t.Fatalf("unexpected promote result: %+v", result)
	}
	contents, err := os.ReadFile(filepath.Join(layout.StateDir(), "instance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != `{"generation":"new"}` {
		t.Fatalf("state content = %s", contents)
	}
	prevContents, err := os.ReadFile(filepath.Join(layout.PrevDir(1), "instance.json"))
	if err != nil {
		t.Fatalf("prev generation missing: %v", err)
	}
	if string(prevContents) != `{"generation":"old"}` {
		t.Fatalf("prev content = %s", prevContents)
	}
	if _, err := os.Stat(filepath.Join(layout.InstanceDir(), "server_files", "big.bin")); err != nil {
		t.Fatalf("server_files must not move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.InstanceDir(), "Zomboid", "Server", "pz.ini")); err != nil {
		t.Fatalf("Zomboid tree must not move: %v", err)
	}
	if after := manifestOf(t, source); after.Checksum != sourceBefore.Checksum {
		t.Fatal("promotion modified the source tree")
	}
	// Re-promotion is idempotent.
	if rerun, err := tool.Promote(); err != nil || rerun.Action != "noop" {
		t.Fatalf("re-promote = %+v, %v", rerun, err)
	}
}

func TestFirstPromotionHasNoPrev(t *testing.T) {
	tool := newTool(t)
	source := filepath.Join(t.TempDir(), "converted-state")
	writeTree(t, source, map[string]string{"instance.json": `{"generation":"first"}`})
	if _, err := tool.Import(source); err != nil {
		t.Fatal(err)
	}
	result, err := tool.Promote()
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if result.Action != "committed" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(tool.Layout().PrevDir(1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first promotion must not create prev, stat err = %v", err)
	}
}

type failingOps struct {
	Ops
	failRename func(oldPath, newPath string) bool
}

func (f failingOps) Rename(oldPath, newPath string) error {
	if f.failRename != nil && f.failRename(oldPath, newPath) {
		return errors.New("injected rename failure")
	}
	return f.Ops.Rename(oldPath, newPath)
}

func TestStepOneFailureIsRecoveryRequiredAndDeletesNothing(t *testing.T) {
	tool := newTool(t, WithOps(nil))
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": `{"generation":"old"}`})
	source := filepath.Join(t.TempDir(), "converted-state")
	writeTree(t, source, map[string]string{"instance.json": `{"generation":"new"}`})
	if _, err := tool.Import(source); err != nil {
		t.Fatal(err)
	}
	injected := newTool(t)
	injected.layout = layout
	injected.ops = failingOps{Ops: realOps{}, failRename: func(oldPath, newPath string) bool {
		return oldPath == layout.StateDir()
	}}
	result, err := injected.Promote()
	if !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("promote error = %v, want ErrRecoveryRequired", err)
	}
	if result.Action != "recovery_required" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(layout.StateDir(), "instance.json")); err != nil {
		t.Fatalf("state must survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.StagingDir(), "instance.json")); err != nil {
		t.Fatalf("staging must survive: %v", err)
	}
}

func TestStepTwoFailureRollsBack(t *testing.T) {
	tool := newTool(t)
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": `{"generation":"old"}`})
	source := filepath.Join(t.TempDir(), "converted-state")
	writeTree(t, source, map[string]string{"instance.json": `{"generation":"new"}`})
	if _, err := tool.Import(source); err != nil {
		t.Fatal(err)
	}
	injected := newTool(t)
	injected.layout = layout
	injected.ops = failingOps{Ops: realOps{}, failRename: func(oldPath, newPath string) bool {
		return oldPath == layout.StagingDir() && newPath == layout.StateDir()
	}}
	result, err := injected.Promote()
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if result.Action != "rolled_back" || result.Journal.State != StateRolledBack {
		t.Fatalf("unexpected result: %+v", result)
	}
	contents, err := os.ReadFile(filepath.Join(layout.StateDir(), "instance.json"))
	if err != nil {
		t.Fatalf("rolled back state missing: %v", err)
	}
	if string(contents) != `{"generation":"old"}` {
		t.Fatalf("state after rollback = %s", contents)
	}
	if result.Journal.LastRolledBackFrom == "" {
		t.Fatal("rollback source must be recorded")
	}
}

func TestStepTwoAndRollbackFailureIsRecoveryRequired(t *testing.T) {
	tool := newTool(t)
	layout := tool.Layout()
	writeTree(t, layout.StateDir(), map[string]string{"instance.json": `{"generation":"old"}`})
	source := filepath.Join(t.TempDir(), "converted-state")
	writeTree(t, source, map[string]string{"instance.json": `{"generation":"new"}`})
	if _, err := tool.Import(source); err != nil {
		t.Fatal(err)
	}
	injected := newTool(t)
	injected.layout = layout
	injected.ops = failingOps{Ops: realOps{}, failRename: func(oldPath, newPath string) bool {
		return newPath == layout.StateDir()
	}}
	result, err := injected.Promote()
	if !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("promote error = %v, want ErrRecoveryRequired", err)
	}
	if result.Action != "recovery_required" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(layout.PrevDir(1), "instance.json")); err != nil {
		t.Fatalf("prev must be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.StagingDir(), "instance.json")); err != nil {
		t.Fatalf("staging must be preserved: %v", err)
	}
}

// layoutWith builds a synthetic on-disk site for the recovery table.
func layoutWith(t *testing.T, journal Journal, trees map[string]map[string]string) *Tool {
	t.Helper()
	tool := newTool(t)
	layout := tool.Layout()
	for name, tree := range trees {
		switch name {
		case "state":
			writeTree(t, layout.StateDir(), tree)
		case "prev":
			writeTree(t, layout.PrevDir(journal.Sequence), tree)
		case "staging":
			writeTree(t, layout.StagingDir(), tree)
		}
	}
	if _, err := os.Stat(layout.StagingDir()); err == nil {
		manifest, err := buildManifest(layout.StagingDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := tool.writeManifest(manifest); err != nil {
			t.Fatal(err)
		}
	}
	if err := tool.writeJournal(journal); err != nil {
		t.Fatal(err)
	}
	return tool
}

func journalFor(state JournalState) Journal {
	return Journal{Schema: journalSchema, State: state, Sequence: 1, UpdatedAt: fixedClock()}
}

func TestRecoveryTableRows(t *testing.T) {
	matching := map[string]string{"instance.json": `{"generation":"new"}`}
	stale := map[string]string{"instance.json": `{"generation":"old"}`}

	cases := []struct {
		name       string
		journal    Journal
		trees      map[string]map[string]string
		wantAction string
		wantState  JournalState
		wantErr    bool
	}{
		{
			name:       "committing state missing prev present staging missing rolls back",
			journal:    journalFor(StateCommitting),
			trees:      map[string]map[string]string{"prev": stale},
			wantAction: "rolled_back", wantState: StateRolledBack,
		},
		{
			name:       "committing state missing prev present staging present rolls forward",
			journal:    journalFor(StateCommitting),
			trees:      map[string]map[string]string{"prev": stale, "staging": matching},
			wantAction: "forward_rolled", wantState: StateCommitted,
		},
		{
			name:       "committing first promotion without prev rolls forward",
			journal:    journalFor(StateCommitting),
			trees:      map[string]map[string]string{"staging": matching},
			wantAction: "forward_rolled", wantState: StateCommitted,
		},
		{
			name:       "committing state present prev missing staging missing is recovery required",
			journal:    journalFor(StateCommitting),
			trees:      map[string]map[string]string{"state": matching},
			wantAction: "recovery_required", wantState: StateCommitting, wantErr: true,
		},
		{
			name:       "verified state missing prev present is recovery required",
			journal:    journalFor(StateVerified),
			trees:      map[string]map[string]string{"prev": stale, "staging": matching},
			wantAction: "recovery_required", wantState: StateVerified, wantErr: true,
		},
		{
			name:       "verified state present discards staging",
			journal:    journalFor(StateVerified),
			trees:      map[string]map[string]string{"state": stale, "staging": matching},
			wantAction: "discarded_staging", wantState: StateVerified,
		},
		{
			name:       "rolled back is a noop",
			journal:    journalFor(StateRolledBack),
			trees:      map[string]map[string]string{"state": stale},
			wantAction: "noop", wantState: StateRolledBack,
		},
		{
			name:       "legacy active is a noop",
			journal:    journalFor(StateLegacyActive),
			trees:      map[string]map[string]string{"prev": stale},
			wantAction: "noop", wantState: StateLegacyActive,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := layoutWith(t, tc.journal, tc.trees)
			result, err := tool.Recover()
			if tc.wantErr && !errors.Is(err, ErrRecoveryRequired) {
				t.Fatalf("recover error = %v, want ErrRecoveryRequired", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("recover: %v", err)
			}
			if result.Action != tc.wantAction {
				t.Fatalf("action = %q, want %q", result.Action, tc.wantAction)
			}
			if result.Journal.State != tc.wantState {
				t.Fatalf("state = %q, want %q", result.Journal.State, tc.wantState)
			}
		})
	}
}

func TestCommittedForwardRollRequiresMatchingChecksum(t *testing.T) {
	stale := map[string]string{"instance.json": `{"generation":"old"}`}
	matching := map[string]string{"instance.json": `{"generation":"new"}`}

	tool := layoutWith(t, journalFor(StateCommitting), map[string]map[string]string{"state": matching, "prev": stale})
	manifest, err := buildManifest(tool.Layout().StateDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := tool.writeManifest(manifest); err != nil {
		t.Fatal(err)
	}
	result, err := tool.Recover()
	if err != nil {
		t.Fatalf("forward roll: %v", err)
	}
	if result.Action != "forward_rolled" || result.Journal.State != StateCommitted {
		t.Fatalf("unexpected forward roll: %+v", result)
	}

	corrupt := layoutWith(t, journalFor(StateCommitting), map[string]map[string]string{"state": matching, "prev": stale})
	other, err := buildManifest(corrupt.Layout().PrevDir(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := corrupt.writeManifest(other); err != nil {
		t.Fatal(err)
	}
	if _, err := corrupt.Recover(); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("checksum mismatch error = %v, want ErrRecoveryRequired", err)
	}
}

func TestJournalIsValidJSONAndBlockedStatesAreNonTerminal(t *testing.T) {
	tool := newTool(t)
	writeTree(t, tool.Layout().StateDir(), map[string]string{"instance.json": "{}"})
	writeTree(t, filepath.Join(tool.Layout().PrevDir(3), "nested"), map[string]string{"a": "b"})
	journal := journalFor(StateStaged)
	journal.Sequence = 3
	if err := tool.writeJournal(journal); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(tool.Layout().JournalPath())
	if err != nil {
		t.Fatal(err)
	}
	var decoded Journal
	if err := json.Unmarshal(contents, &decoded); err != nil {
		t.Fatalf("journal must be valid JSON: %v", err)
	}
	snapshot, err := tool.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.PrevExists || !snapshot.StateExists {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	for _, state := range []JournalState{StateStaged, StateVerified, StateCommitting, StateRollingBack} {
		if !(Snapshot{Journal: Journal{State: state}}).Blocked() {
			t.Fatalf("state %q must block normal startup", state)
		}
	}
	for _, state := range []JournalState{StateIdle, StateExported, StateCommitted, StateRolledBack, StateLegacyActive} {
		if (Snapshot{Journal: Journal{State: state}}).Blocked() {
			t.Fatalf("state %q must not block startup", state)
		}
	}
}
