package localstate

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type atomicFile interface {
	Name() string
	Write([]byte) (int, error)
	Sync() error
	Chmod(os.FileMode) error
	Close() error
}

type fileOps interface {
	MkdirAll(string, os.FileMode) error
	CreateTemp(string, string) (atomicFile, error)
	Lstat(string) (os.FileInfo, error)
	ReadFile(string) ([]byte, error)
	Rename(string, string) error
	Remove(string) error
	SyncDir(string) error
}

type osFileOps struct{}

func (osFileOps) MkdirAll(path string, mode os.FileMode) error { return os.MkdirAll(path, mode) }
func (osFileOps) CreateTemp(dir, pattern string) (atomicFile, error) {
	return os.CreateTemp(dir, pattern)
}
func (osFileOps) Lstat(path string) (os.FileInfo, error) { return os.Lstat(path) }
func (osFileOps) ReadFile(path string) ([]byte, error)   { return os.ReadFile(path) }
func (osFileOps) Rename(oldPath, newPath string) error   { return os.Rename(oldPath, newPath) }
func (osFileOps) Remove(path string) error               { return os.Remove(path) }

// SyncDir durably flushes a directory entry. Windows does not support flushing
// a directory handle (FlushFileBuffers returns ERROR_ACCESS_DENIED), and the
// rename plus the file's own flush already provide the durability story there,
// so this is a documented no-op on Windows.
func (osFileOps) SyncDir(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// atomicWriteFile commits data using a same-directory temporary file. If a target
// already exists, its old bytes are durably copied to target+.bak before the
// replacement is renamed into place. An error is returned for every failed
// stage, including a post-rename directory sync or readback failure.
func atomicWriteFile(fs fileOps, target string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(target)
	base := filepath.Base(target)
	if err := fs.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	tmp, err := fs.CreateTemp(dir, "."+base+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	tmpClosed := false
	tmpRenamed := false
	defer func() {
		if !tmpClosed {
			_ = tmp.Close()
		}
		if !tmpRenamed {
			_ = fs.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("set temporary file mode: %w", err)
	}
	if n, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	} else if n != len(data) {
		return fmt.Errorf("write temporary file: %w", io.ErrShortWrite)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		tmpClosed = true
		return fmt.Errorf("close temporary file: %w", err)
	}
	tmpClosed = true

	backupPath := target + ".bak"
	if info, statErr := fs.Lstat(target); statErr == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse to replace non-regular state file %q", target)
		}
		oldBytes, readErr := fs.ReadFile(target)
		if readErr != nil {
			return fmt.Errorf("read prior state for backup: %w", readErr)
		}
		if err := durableCopy(fs, dir, filepath.Base(backupPath), oldBytes, mode); err != nil {
			return fmt.Errorf("persist backup: %w", err)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect prior state: %w", statErr)
	} else {
		// Durable directory creation ensures that creation of an initial target
		// is covered by the same directory-entry persistence barrier.
		if err := fs.SyncDir(dir); err != nil {
			return fmt.Errorf("sync state directory before first write: %w", err)
		}
	}

	if err := fs.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("replace target: %w", err)
	}
	tmpRenamed = true
	if err := fs.SyncDir(dir); err != nil {
		return fmt.Errorf("sync target directory: %w", err)
	}
	readback, err := fs.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read back target: %w", err)
	}
	if !bytes.Equal(readback, data) {
		return fmt.Errorf("read back target: %w", errors.New("content mismatch"))
	}
	return nil
}

func durableCopy(fs fileOps, dir, backupName string, data []byte, mode os.FileMode) error {
	f, err := fs.CreateTemp(dir, "."+backupName+".tmp-")
	if err != nil {
		return fmt.Errorf("create backup temporary file: %w", err)
	}
	tmpPath := f.Name()
	closed := false
	renamed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
		if !renamed {
			_ = fs.Remove(tmpPath)
		}
	}()
	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("set backup mode: %w", err)
	}
	if n, err := f.Write(data); err != nil {
		return fmt.Errorf("write backup: %w", err)
	} else if n != len(data) {
		return fmt.Errorf("write backup: %w", io.ErrShortWrite)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync backup: %w", err)
	}
	if err := f.Close(); err != nil {
		closed = true
		return fmt.Errorf("close backup: %w", err)
	}
	closed = true
	if err := fs.Rename(tmpPath, filepath.Join(dir, backupName)); err != nil {
		return fmt.Errorf("rename backup: %w", err)
	}
	renamed = true
	if err := fs.SyncDir(dir); err != nil {
		return fmt.Errorf("sync backup directory: %w", err)
	}
	return nil
}
