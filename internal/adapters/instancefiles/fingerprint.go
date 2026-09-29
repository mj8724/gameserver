package instancefiles

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// Fingerprint computes the fingerprint for the instance install directory.
// A missing directory yields an empty fingerprint (nothing installed yet);
// a missing app manifest means the previous step did not complete, which is
// exactly the information reconciliation needs.
func (f *Files) Fingerprint(id domain.InstanceID) (ports.ArtifactFingerprint, error) {
	dir, err := f.InstallDir(id)
	if err != nil {
		return ports.ArtifactFingerprint{}, err
	}
	var fp ports.ArtifactFingerprint
	manifest, err := findAppManifest(dir)
	if err != nil {
		return ports.ArtifactFingerprint{}, err
	}
	if manifest != "" {
		info, err := os.Stat(manifest)
		if err != nil {
			return ports.ArtifactFingerprint{}, err
		}
		digest, err := hashFile(manifest)
		if err != nil {
			return ports.ArtifactFingerprint{}, err
		}
		fp.ManifestName = filepath.Base(manifest)
		fp.ManifestSHA = digest
		fp.TotalBytes = info.Size()
	}
	buildID, err := readLastServerVersion(dir)
	if err != nil {
		return ports.ArtifactFingerprint{}, err
	}
	fp.BuildID = buildID
	return fp, nil
}

// TotalBytes returns the total size of files under the install directory. It
// is used as the third reconciliation element (cheap metadata-only walk).
func (f *Files) TotalBytes(id domain.InstanceID) (int64, error) {
	dir, err := f.InstallDir(id)
	if err != nil {
		return 0, err
	}
	var total int64
	err = filepath.WalkDir(dir, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func findAppManifest(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "steamapps", "appmanifest_*.acf"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", nil
	}
	// Prefer the manifest that mentions our app when several exist (rare).
	for _, m := range matches {
		name := filepath.Base(m)
		if name == "appmanifest_380870.acf" {
			return m, nil
		}
	}
	return matches[0], nil
}

func readLastServerVersion(dir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "last_server_version.txt"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(raw), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
