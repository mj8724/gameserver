package steamcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalWorkshopCache provisions workshop items from a Steam client's already
// downloaded workshop content (M3.3 alternative path). SteamCMD needs an
// authenticated account to download workshop items, but the operator's Steam
// client may already hold them; copying from that cache keeps the product's
// anonymous/no-credential boundary intact.
type LocalWorkshopCache struct {
	// Root is the Steam library's workshop content directory, i.e. the parent
	// of <appid>/<itemid>.
	Root string
}

// Enabled reports whether a cache root is configured and present.
func (c LocalWorkshopCache) Enabled() bool {
	if strings.TrimSpace(c.Root) == "" {
		return false
	}
	info, err := os.Stat(c.Root)
	return err == nil && info.IsDir()
}

// Stage copies one item from the cache into destinationRoot and verifies that
// the copied tree contains at least one mod.info (PZ's mod marker). It never
// interprets or executes mod content.
func (c LocalWorkshopCache) Stage(appID, itemID, destinationRoot string) (string, error) {
	if !c.Enabled() {
		return "", errors.New("local workshop cache is not configured")
	}
	if !allDigits(appID) || !allDigits(itemID) {
		return "", errors.New("app id and workshop id must be numeric")
	}
	// The Steam client keys workshop content by the GAME app id (e.g. 108600
	// for Project Zomboid), while a dedicated-server install uses its own app
	// id (380870). Look the item up under the requested app id first, then
	// across every app id present in the cache — an item id is unique.
	source := filepath.Join(c.Root, appID, itemID)
	if info, statErr := os.Stat(source); statErr != nil || !info.IsDir() {
		source = ""
		entries, err := os.ReadDir(c.Root)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(c.Root, entry.Name(), itemID)
			if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
				source = candidate
				break
			}
		}
		if source == "" {
			return "", fmt.Errorf("workshop item %s is not present in the local cache (%s)", itemID, c.Root)
		}
	}
	target := filepath.Join(destinationRoot, appID, itemID)
	if err := os.RemoveAll(target); err != nil {
		return "", err
	}
	if err := copyWorkshopTree(source, target); err != nil {
		return "", err
	}
	if !containsModInfo(target) {
		_ = os.RemoveAll(target)
		return "", fmt.Errorf("staged workshop item %s contains no mod.info", itemID)
	}
	return filepath.ToSlash(target), nil
}

// copyWorkshopTree copies a directory tree with regular-file checks only.
func copyWorkshopTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("workshop content contains a symlink: %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.Create(destination)
		if err != nil {
			return err
		}
		defer output.Close()
		if _, err := io.Copy(output, input); err != nil {
			return err
		}
		return output.Sync()
	})
}

// containsModInfo reports whether any directory under root holds a mod.info.
func containsModInfo(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() && strings.EqualFold(entry.Name(), "mod.info") {
			found = true
			return io.EOF
		}
		return nil
	})
	return found
}
