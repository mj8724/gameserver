package steamcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMod(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// An item present in the operator's Steam client cache is staged into the
// instance without any Steam credentials, and the copy is verified by mod.info.
func TestLocalWorkshopCacheStagesItem(t *testing.T) {
	cache := t.TempDir()
	source := filepath.Join(cache, "380870", "2169435993")
	writeMod(t, source, map[string]string{"mods/MyMod/mod.info": "name=MyMod\nid=MyMod\n", "mods/MyMod/media/x.txt": "x"})
	destination := filepath.Join(t.TempDir(), "content", "380870", "2169435993")

	staged, err := LocalWorkshopCache{Root: cache}.Stage("380870", "2169435993", destination)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := os.Stat(filepath.Join(staged, "mods", "MyMod", "mod.info")); err != nil {
		t.Fatalf("staged tree missing mod.info: %v", err)
	}
}

// A missing item is reported, and a tree without mod.info is refused (and
// cleaned up) so a broken item never looks installed.
func TestLocalWorkshopCacheRejectsMissingAndInvalid(t *testing.T) {
	cache := t.TempDir()
	writeMod(t, filepath.Join(cache, "380870", "999"), map[string]string{"readme.txt": "no mod here"})
	destination := filepath.Join(t.TempDir(), "out")

	if _, err := (LocalWorkshopCache{Root: cache}).Stage("380870", "123", destination); err == nil {
		t.Fatal("an item absent from the cache must fail")
	}
	if _, err := (LocalWorkshopCache{Root: cache}).Stage("380870", "999", destination); err == nil {
		t.Fatal("an item without mod.info must be refused")
	}
	if entries, err := os.ReadDir(filepath.Join(destination, "380870")); err == nil && len(entries) > 0 {
		t.Fatal("a refused item must not leave staged content behind")
	}
	// Non-numeric ids are rejected before touching the filesystem.
	if _, err := (LocalWorkshopCache{Root: cache}).Stage("380870", "../etc", destination); err == nil {
		t.Fatal("path traversal id must be rejected")
	}
}

// The Steam client stores workshop content under the GAME app id, while a
// dedicated-server install uses its own: the cache lookup must find the item
// regardless of which app id directory holds it.
func TestLocalWorkshopCacheFindsItemUnderAnotherAppID(t *testing.T) {
	cache := t.TempDir()
	// Cached under the game id (108600), requested under the server id (380870).
	writeMod(t, filepath.Join(cache, "108600", "2169435993"), map[string]string{"mods/M/mod.info": "name=M\n"})
	destination := filepath.Join(t.TempDir(), "out")

	staged, err := LocalWorkshopCache{Root: cache}.Stage("380870", "2169435993", destination)
	if err != nil {
		t.Fatalf("Stage across app ids: %v", err)
	}
	if !strings.Contains(staged, "2169435993") {
		t.Fatalf("unexpected staged path: %s", staged)
	}
	if _, err := os.Stat(filepath.Join(staged, "mods", "M", "mod.info")); err != nil {
		t.Fatalf("staged content missing: %v", err)
	}
}
