// Package staticassets provides read-only access to the retained browser UI.
package staticassets

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Store serves files from a fixed root directory. Names are relative, must not
// escape the root, and must resolve (including symlinks) inside the root.
type Store struct {
	root string
}

// New opens a root directory for read-only asset serving.
func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("static asset root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve static asset root: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("stat static asset root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("static asset root %s is not a directory", absolute)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve static asset root symlinks: %w", err)
	}
	return &Store{root: resolved}, nil
}

// Open implements ports.StaticAssets. Only regular files inside the root are
// returned; directories and symlinks escaping the root fail closed.
func (s *Store) Open(name string) (fs.File, error) {
	if s == nil {
		return nil, fs.ErrNotExist
	}
	clean, err := safeName(name)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(s.root, filepath.FromSlash(clean))
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return nil, err
	}
	if !containedIn(s.root, resolved) {
		return nil, fs.ErrNotExist
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fs.ErrNotExist
	}
	return os.Open(resolved)
}

func containedIn(root, candidate string) bool {
	if candidate == root {
		return false
	}
	return strings.HasPrefix(candidate, root+string(os.PathSeparator))
}

func safeName(name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00") {
		return "", fs.ErrNotExist
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "." || segment == ".." {
			return "", fs.ErrNotExist
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fs.ErrNotExist
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fs.ErrNotExist
		}
	}
	return clean, nil
}
