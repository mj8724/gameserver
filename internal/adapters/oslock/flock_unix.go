//go:build unix

package oslock

import (
	"fmt"
	"os"
	"syscall"
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
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("lock %s: %w", path, err)
	}
	return &fileLock{path: path, file: file}, true, nil
}

func (l *fileLock) release() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	closeErr := file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && !os.IsPermission(err) {
		return err
	}
	return nil
}
