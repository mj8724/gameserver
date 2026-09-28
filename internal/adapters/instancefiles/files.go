// Package instancefiles resolves the on-disk layout of one instance without
// touching game content.
package instancefiles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

var instancePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Files implements ports.InstanceFiles for a servers root.
type Files struct {
	ServersRoot string
	Platform    string
}

// New validates the servers root and platform.
func New(serversRoot, platform string) (*Files, error) {
	if strings.TrimSpace(serversRoot) == "" {
		return nil, errors.New("servers root is required")
	}
	absolute, err := filepath.Abs(serversRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve servers root: %w", err)
	}
	switch platform {
	case "windows", "linux", "darwin":
	default:
		return nil, fmt.Errorf("unsupported platform %q", platform)
	}
	return &Files{ServersRoot: absolute, Platform: platform}, nil
}

// InstanceRoot returns <serversRoot>/<instance>.
func (f *Files) InstanceRoot(id domain.InstanceID) (string, error) {
	if !instancePattern.MatchString(string(id)) {
		return "", fmt.Errorf("invalid instance id %q", id)
	}
	return filepath.Join(f.ServersRoot, string(id)), nil
}

// InstallDir returns the SteamCMD install target for the instance.
func (f *Files) InstallDir(id domain.InstanceID) (string, error) {
	root, err := f.InstanceRoot(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "server_files"), nil
}

// CacheDir returns the game cache/home directory for the instance.
func (f *Files) CacheDir(id domain.InstanceID) (string, error) {
	root, err := f.InstanceRoot(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "Zomboid"), nil
}

// IsInstalled mirrors the legacy presence check on the install directory.
func (f *Files) IsInstalled(id domain.InstanceID) bool {
	installDir, err := f.InstallDir(id)
	if err != nil {
		return false
	}
	var candidates []string
	switch f.Platform {
	case "windows":
		candidates = []string{"StartServer64.bat", "ProjectZomboid64.exe"}
	default:
		candidates = []string{"start-server.sh", "ProjectZomboid64"}
	}
	for _, name := range candidates {
		if info, err := os.Stat(filepath.Join(installDir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// DiskUsageMB reports the observed size of the instance directory. It is an
// observation only, never a quota.
func (f *Files) DiskUsageMB(id domain.InstanceID) (float64, error) {
	root, err := f.InstanceRoot(id)
	if err != nil {
		return 0, err
	}
	var total int64
	err = filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return float64(total) / (1024 * 1024), nil
}

var _ ports.InstanceFiles = (*Files)(nil)
