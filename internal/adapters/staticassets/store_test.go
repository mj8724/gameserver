package staticassets

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<!doctype html><title>ui</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("console.log(1)"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return store, root
}

func TestOpenServesRootAndNestedFile(t *testing.T) {
	store, _ := newStore(t)
	file, err := store.Open("index.html")
	if err != nil {
		t.Fatalf("open index.html: %v", err)
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) == "" {
		t.Fatal("expected non-empty index.html")
	}
	nested, err := store.Open("assets/app.js")
	if err != nil {
		t.Fatalf("open assets/app.js: %v", err)
	}
	defer nested.Close()
}

func TestOpenRejectsTraversalAndAbsoluteNames(t *testing.T) {
	store, root := newStore(t)
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "/etc/passwd", "../outside.txt", "assets/../../outside.txt", `C:\Windows\win.ini`, "assets/./../index.html", ".."} {
		if _, err := store.Open(name); err == nil {
			t.Fatalf("Open(%q) succeeded, want rejection", name)
		}
	}
}

func TestOpenRejectsDirectoryAndSymlinkEscape(t *testing.T) {
	store, root := newStore(t)
	if _, err := store.Open("assets"); err == nil {
		t.Fatal("Open(directory) succeeded, want rejection")
	}
	if _, err := store.Open("."); err == nil {
		t.Fatal("Open(.) succeeded, want rejection")
	}
	outside := filepath.Join(filepath.Dir(root), "outside-target.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := store.Open("escape.txt"); err == nil {
		t.Fatal("Open(symlink escaping root) succeeded, want rejection")
	}
	if _, err := store.Open("missing.html"); err == nil || !os.IsNotExist(err) {
		t.Fatalf("Open(missing) error = %v, want os.ErrNotExist", err)
	}
}

func TestNewRejectsMissingOrNonDirectoryRoot(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("New(empty) succeeded, want error")
	}
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(file); err == nil {
		t.Fatal("New(file) succeeded, want error")
	}
	if _, err := New(filepath.Join(root, "missing")); err == nil {
		t.Fatal("New(missing) succeeded, want error")
	}
}
