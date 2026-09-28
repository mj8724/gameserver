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
