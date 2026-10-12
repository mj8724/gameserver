//go:build windows

package oslock

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
	// errorLockViolation is Win32 ERROR_LOCK_VIOLATION (33).
	errorLockViolation syscall.Errno = 33
)

type fileLock struct {
	path string
	file *os.File
}

func tryLockFile(path string) (*fileLock, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return nil, false, fmt.Errorf("open lock file %s: %w", path, err)
	}
	var overlapped syscall.Overlapped
	result, _, callErr := procLockFileEx.Call(
		file.Fd(),
		lockfileExclusiveLock|lockfileFailImmediately,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if result == 0 {
		_ = file.Close()
		if errno, ok := callErr.(syscall.Errno); ok && errno == errorLockViolation {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("lock %s: %w", path, callErr)
	}
	return &fileLock{path: path, file: file}, true, nil
}

func (l *fileLock) release() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	var overlapped syscall.Overlapped
	_, _, _ = procUnlockFileEx.Call(
		file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	return file.Close()
}

func syncDir(string) error { return nil }
