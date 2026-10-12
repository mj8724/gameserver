package instancefiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mj8724/gameserver/internal/domain"
)

func TestLayoutAndInstallDetection(t *testing.T) {
	root := t.TempDir()
	files, err := New(root, "linux")
	if err != nil {
		t.Fatal(err)
	}
	instance := domain.InstanceID("pz_01")
	instanceRoot, err := files.InstanceRoot(instance)
	if err != nil {
		t.Fatal(err)
	}
	if instanceRoot != filepath.Join(root, "pz_01") {
		t.Fatalf("instance root = %s", instanceRoot)
	}
	installDir, err := files.InstallDir(instance)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir, err := files.CacheDir(instance)
	if err != nil {
		t.Fatal(err)
	}
	if installDir != filepath.Join(instanceRoot, "server_files") || cacheDir != filepath.Join(instanceRoot, "Zomboid") {
		t.Fatalf("unexpected dirs: %s / %s", installDir, cacheDir)
	}
	if files.IsInstalled(instance) {
		t.Fatal("empty install dir must not report installed")
	}
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "start-server.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !files.IsInstalled(instance) {
		t.Fatal("start-server.sh should mark the instance installed")
	}

	windows, err := New(root, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if windows.IsInstalled(instance) {
		t.Fatal("windows must look for its own artifacts, not the unix script")
	}
	if err := os.WriteFile(filepath.Join(installDir, "StartServer64.bat"), []byte("@echo off\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !windows.IsInstalled(instance) {
		t.Fatal("StartServer64.bat should mark the instance installed on windows")
	}
}

func TestRejectsInvalidInputs(t *testing.T) {
	if _, err := New("", "linux"); err == nil {
		t.Fatal("empty root accepted")
	}
	if _, err := New(t.TempDir(), "plan9"); err == nil {
		t.Fatal("unsupported platform accepted")
	}
	files, err := New(t.TempDir(), "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.InstanceRoot(domain.InstanceID("../escape")); err == nil {
		t.Fatal("traversal instance id accepted")
	}
	if files.IsInstalled(domain.InstanceID("../escape")) {
		t.Fatal("traversal instance id must not report installed")
	}
}

// M3.5 accounting rule: DiskUsageMB walks the instance root, which contains no
// backup or lock directories (those live outside the instance subtree), so the
// capacity view and the status projection measure the same bytes.
func TestDiskUsageExcludesBackupsAndLockDirs(t *testing.T) {
	root := t.TempDir()
	serversRoot := filepath.Join(root, "servers")
	files, err := New(serversRoot, "linux")
	if err != nil {
		t.Fatal(err)
	}
	instanceDir, err := files.InstanceRoot("pz_01")
	if err != nil {
		t.Fatal(err)
	}
	for path, size := range map[string]int{
		filepath.Join(instanceDir, "server_files", "game.bin"):  4096,
		filepath.Join(instanceDir, "Zomboid", "Saves", "s.bin"): 2048,
		// Outside the instance subtree: must not be counted.
		filepath.Join(serversRoot, "#locks", "pz_01.lock"):         8192,
		filepath.Join(serversRoot, "#owners", "pz_01.json"):        8192,
		filepath.Join(root, "backups", "pz_01", "stamp", "st.bin"): 8192,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	used, err := files.DiskUsageMB("pz_01")
	if err != nil {
		t.Fatal(err)
	}
	want := float64(4096+2048) / (1024 * 1024)
	if used != want {
		t.Fatalf("usage = %v MB, want %v MB (locks/owners/backups must be excluded)", used, want)
	}
}
