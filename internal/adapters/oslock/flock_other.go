//go:build !unix && !windows

package oslock

// tryLockFile fails closed on platforms without a proven lock primitive:
// callers must treat every ownership statement as unprovable.
func tryLockFile(string) (*fileLock, bool, error) {
	return nil, false, ErrUnsupportedPlatform
}

type fileLock struct{ path string }

func (l *fileLock) release() error { return nil }

func syncDir(string) error { return nil }
