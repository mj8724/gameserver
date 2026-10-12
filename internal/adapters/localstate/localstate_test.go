package localstate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mj8724/gameserver/internal/domain"
)

func testDefaults() domain.InstanceState {
	return domain.InstanceState{
		ID:         "pz_01",
		Name:       "Project Zomboid Dedicated Server",
		TemplateID: "project_zomboid",
		Variables: map[string]any{
			"SERVER_NAME":    "default-name",
			"ADMIN_PASSWORD": "synthetic-admin-secret",
			"EXTRA_DEFAULT":  "keep-me",
		},
		Ports: map[string]int{"SERVER_PORT": 16261, "DIRECT_PORT": 16262},
	}
}

func TestLoadImportsLegacyAndMergesDefaultsWithoutWriting(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "servers", "pz_01", "instance.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := []byte(`{"instance_id":"pz_01","variables":{"SERVER_NAME":"valid_name","MAX_PLAYERS":12},"ports":{"SERVER_PORT":17000},"mods":{"workshop_ids":["42"]}}`)
	if err := os.WriteFile(legacy, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, testDefaults())
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Load(context.Background(), domain.InstanceID("pz_01"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Variables["SERVER_NAME"] != "valid_name" || state.Variables["MAX_PLAYERS"] != float64(12) {
		t.Fatalf("loaded variables = %#v", state.Variables)
	}
	if state.Variables["ADMIN_PASSWORD"] != "synthetic-admin-secret" || state.Variables["EXTRA_DEFAULT"] != "keep-me" {
		t.Fatalf("missing nested defaults: %#v", state.Variables)
	}
	if state.Ports["SERVER_PORT"] != 17000 || state.Ports["DIRECT_PORT"] != 16262 {
		t.Fatalf("loaded ports = %#v", state.Ports)
	}
	current, _, err := store.Paths(domain.InstanceID("pz_01"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(current); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy load must not promote/write current state, stat err=%v", err)
	}
	gotFixture, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(gotFixture, fixture) {
		t.Fatalf("legacy source changed: err=%v content=%q", err, gotFixture)
	}
}

func TestLoadInvalidServerNameFallsBackAndCorruptStateFailsClosed(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "servers", "pz_01", "instance.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{"variables":{"SERVER_NAME":"../unsafe"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, testDefaults())
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Load(context.Background(), "pz_01")
	if err != nil {
		t.Fatal(err)
	}
	if state.Variables["SERVER_NAME"] != "servertest" {
		t.Fatalf("unsafe name was not replaced: %#v", state.Variables["SERVER_NAME"])
	}
	if err := os.WriteFile(legacy, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background(), "pz_01"); err == nil || !strings.Contains(err.Error(), "recovery required") {
		t.Fatalf("corrupt file should fail closed, got %v", err)
	}
}

func TestLoadRejectsPathTraversalAndInstanceMismatch(t *testing.T) {
	store, err := NewStore(t.TempDir(), testDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background(), "../pz_01"); err == nil {
		t.Fatal("expected invalid instance id")
	}
	legacy := filepath.Join(store.dataRoot, "servers", "pz_01", "instance.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{"instance_id":"another"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background(), "pz_01"); err == nil {
		t.Fatal("expected instance identity mismatch")
	}
}

func TestSaveAtomicBackupModesAndUnknownFieldPreservation(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "servers", "pz_01", "instance.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	legacyBytes := []byte(`{"instance_id":"pz_01","custom":{"owner":"legacy"},"quota_gb":42,"variables":{"SERVER_NAME":"legacy_name"}}`)
	if err := os.WriteFile(legacy, legacyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, testDefaults())
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Load(context.Background(), "pz_01")
	if err != nil {
		t.Fatal(err)
	}
	state.Name = "A name with safe UTF-8"
	state.Variables["MAX_PLAYERS"] = 24
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	current, _, _ := store.Paths("pz_01")
	firstBytes, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(firstBytes, &got); err != nil {
		t.Fatal(err)
	}
	if got["quota_gb"] != float64(42) || got["custom"].(map[string]any)["owner"] != "legacy" {
		t.Fatalf("unknown legacy fields lost: %#v", got)
	}
	state.Name = "Second version"
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(current + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backup, firstBytes) {
		t.Fatalf("backup doesn't contain durable previous version: got %q want %q", backup, firstBytes)
	}
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{
			filepath.Join(root, "servers"):                   0o700,
			filepath.Join(root, "servers", "pz_01"):          0o700,
			filepath.Join(root, "servers", "pz_01", "state"): 0o700,
			current:          0o600,
			current + ".bak": 0o600,
		} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != want {
				t.Errorf("%s mode=%04o want %04o", path, info.Mode().Perm(), want)
			}
		}
	}
}

func TestDefaultStateGeneratesSecretWithoutLogging(t *testing.T) {
	state, err := DefaultState("pz_01")
	if err != nil {
		t.Fatal(err)
	}
	secret, ok := state.Variables["ADMIN_PASSWORD"].(string)
	if !ok || len(secret) < 24 {
		t.Fatalf("unexpected default admin password %T length=%d", state.Variables["ADMIN_PASSWORD"], len(secret))
	}
	if state.Ports["SERVER_PORT"] != 16261 || state.Ports["DIRECT_PORT"] != 16262 {
		t.Fatalf("unexpected legacy ports: %#v", state.Ports)
	}
}

func TestAtomicWriteFaultsNeverReturnSuccessAndLeaveWholeOldOrNewFile(t *testing.T) {
	failureStages := []string{
		"mkdir", "create-temp", "temp.chmod", "temp.write", "temp.sync", "temp.close",
		"lstat", "read-target-old", "backup.create-temp", "backup.chmod", "backup.write",
		"backup.sync", "backup.close", "backup.rename", "backup.dirsync", "target.rename",
		"target.dirsync", "read-target-readback",
	}
	for _, stage := range failureStages {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "instance.json")
			old := []byte(`{"version":"old","nested":{"x":1}}`)
			newData := []byte(`{"version":"new","nested":{"x":2}}`)
			if err := os.WriteFile(target, old, 0o600); err != nil {
				t.Fatal(err)
			}
			ops := &failingFileOps{base: osFileOps{}, failAt: stage}
			err := atomicWriteFile(ops, target, newData, 0o600)
			if err == nil {
				t.Fatalf("injected %s failure returned success", stage)
			}
			got, readErr := os.ReadFile(target)
			if readErr != nil {
				t.Fatalf("target became unavailable after %s: %v", stage, readErr)
			}
			if !bytes.Equal(got, old) && !bytes.Equal(got, newData) {
				t.Fatalf("target is neither old nor new after %s: %q", stage, got)
			}
		})
	}
}

type failingFileOps struct {
	base    fileOps
	failAt  string
	reads   int
	renames int
	dirsync int
}

func (f *failingFileOps) hit(stage string) error {
	if f.failAt == stage {
		return errors.New("injected " + stage + " failure")
	}
	return nil
}
func (f *failingFileOps) MkdirAll(path string, mode os.FileMode) error {
	if err := f.hit("mkdir"); err != nil {
		return err
	}
	return f.base.MkdirAll(path, mode)
}
func (f *failingFileOps) CreateTemp(dir, pattern string) (atomicFile, error) {
	backup := strings.Contains(pattern, ".bak.")
	stage := "create-temp"
	if backup {
		stage = "backup.create-temp"
	}
	if err := f.hit(stage); err != nil {
		return nil, err
	}
	file, err := f.base.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	return &failingAtomicFile{atomicFile: file, ops: f, prefix: map[bool]string{true: "backup.", false: "temp."}[backup]}, nil
}
func (f *failingFileOps) Lstat(path string) (os.FileInfo, error) {
	if err := f.hit("lstat"); err != nil {
		return nil, err
	}
	return f.base.Lstat(path)
}
func (f *failingFileOps) ReadFile(path string) ([]byte, error) {
	f.reads++
	stage := "read-target-old"
	if f.reads > 1 {
		stage = "read-target-readback"
	}
	if err := f.hit(stage); err != nil {
		return nil, err
	}
	return f.base.ReadFile(path)
}
func (f *failingFileOps) Rename(oldPath, newPath string) error {
	f.renames++
	stage := "backup.rename"
	if f.renames > 1 {
		stage = "target.rename"
	}
	if err := f.hit(stage); err != nil {
		return err
	}
	return f.base.Rename(oldPath, newPath)
}
func (f *failingFileOps) Remove(path string) error { return f.base.Remove(path) }
func (f *failingFileOps) SyncDir(path string) error {
	f.dirsync++
	stage := "backup.dirsync"
	if f.dirsync > 1 {
		stage = "target.dirsync"
	}
	if err := f.hit(stage); err != nil {
		return err
	}
	return f.base.SyncDir(path)
}

type failingAtomicFile struct {
	atomicFile
	ops    *failingFileOps
	prefix string
}

func (f *failingAtomicFile) Chmod(mode os.FileMode) error {
	if err := f.ops.hit(f.prefix + "chmod"); err != nil {
		return err
	}
	return f.atomicFile.Chmod(mode)
}
func (f *failingAtomicFile) Write(data []byte) (int, error) {
	if err := f.ops.hit(f.prefix + "write"); err != nil {
		return 0, err
	}
	return f.atomicFile.Write(data)
}
func (f *failingAtomicFile) Sync() error {
	if err := f.ops.hit(f.prefix + "sync"); err != nil {
		return err
	}
	return f.atomicFile.Sync()
}
func (f *failingAtomicFile) Close() error {
	err := f.atomicFile.Close()
	if err != nil {
		return err
	}
	return f.ops.hit(f.prefix + "close")
}

func TestAtomicWriteDetectsShortWrite(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "short-")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	fileOps := shortWriteOps{base: osFileOps{}}
	if err := atomicWriteFile(fileOps, filepath.Join(dir, "state.json"), []byte("new"), 0o600); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write should fail, got %v", err)
	}
}

type shortWriteOps struct{ base fileOps }

func (s shortWriteOps) MkdirAll(path string, mode os.FileMode) error {
	return s.base.MkdirAll(path, mode)
}
func (s shortWriteOps) CreateTemp(dir, pattern string) (atomicFile, error) {
	file, err := s.base.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	return shortWriter{atomicFile: file}, nil
}
func (s shortWriteOps) Lstat(path string) (os.FileInfo, error) { return s.base.Lstat(path) }
func (s shortWriteOps) ReadFile(path string) ([]byte, error)   { return s.base.ReadFile(path) }
func (s shortWriteOps) Rename(oldPath, newPath string) error   { return s.base.Rename(oldPath, newPath) }
func (s shortWriteOps) Remove(path string) error               { return s.base.Remove(path) }
func (s shortWriteOps) SyncDir(path string) error              { return s.base.SyncDir(path) }

type shortWriter struct{ atomicFile }

func (shortWriter) Write([]byte) (int, error) { return 0, nil }
