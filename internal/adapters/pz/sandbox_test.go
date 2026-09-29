package pz

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sandboxFixture = `SandboxVars = {
    VERSION = 5,
    -- 注释必须保留
    Zombies = 4,
    Distribution = "Urban Focused",
    ZombieLore = {
        Speed = 1,
    },
}
`

func writeSandbox(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "servertest_SandboxVars.lua")
	if err := os.WriteFile(path, []byte(sandboxFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSandboxReadWriteRoundTrip(t *testing.T) {
	path := writeSandbox(t)
	values, err := ReadSandbox(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if values["Zombies"] != "4" || values["Distribution"] != `"Urban Focused"` {
		t.Fatalf("read values = %v", values)
	}
	if err := ApplySandbox(path, map[string]string{
		"Zombies":      "7",
		"Distribution": "Rural",
		"Speed":        "2",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	after, err := ReadSandbox(path)
	if err != nil {
		t.Fatal(err)
	}
	if after["Zombies"] != "7" || after["Distribution"] != `"Rural"` || after["Speed"] != "2" {
		t.Fatalf("after = %v", after)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "-- 注释必须保留") {
		t.Fatal("comments must survive a sandbox write")
	}
	if !strings.Contains(text, "VERSION = 5") {
		t.Fatal("untouched assignments must survive")
	}
	if !strings.Contains(text, "Speed = 2") {
		t.Fatal("nested assignments must be rewritten in place")
	}
	if _, err := os.Stat(path + ".bak1"); err != nil {
		t.Fatalf("first write must keep a backup: %v", err)
	}
}

func TestSandboxBackupsRotateAcrossGenerations(t *testing.T) {
	path := writeSandbox(t)
	for _, value := range []string{"1", "2", "3", "4"} {
		if err := ApplySandbox(path, map[string]string{"Zombies": value}); err != nil {
			t.Fatalf("apply %s: %v", value, err)
		}
	}
	for generation := 1; generation <= sandboxBackupGenerations; generation++ {
		backup := filepath.Join(path[:len(path)-len(filepath.Ext(path))]) // placeholder, replaced below
		_ = backup
		name := path + ".bak" + string(rune('0'+generation))
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("generation %d missing: %v", generation, err)
		}
	}
	oldest := path + ".bak3"
	raw, err := os.ReadFile(oldest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Zombies = 1") {
		t.Fatalf("oldest generation should hold the earliest revision:\n%s", raw)
	}
}

func TestSandboxRejectsUnrepresentableValues(t *testing.T) {
	cases := map[string]string{
		"control char": "1\nZombies = 99",
		"quote break":  `"abc"..os.execute("x")`,
		"stray quote":  `he said "hi"`,
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeSandbox(t)
			if err := ApplySandbox(path, map[string]string{"Zombies": value}); err == nil {
				t.Fatalf("expected rejection for %s", name)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "Zombies = 4") {
				t.Fatal("a rejected write must not modify the file")
			}
		})
	}
}

func TestSandboxUnknownKeyFailsClosed(t *testing.T) {
	path := writeSandbox(t)
	if err := ApplySandbox(path, map[string]string{"NotPresent": "1"}); err == nil {
		t.Fatal("writing an absent key must fail closed")
	}
	if err := ApplySandbox(path, map[string]string{"bad-key": "1"}); err == nil {
		t.Fatal("invalid key names must be rejected")
	}
}

func TestSandboxPathValidation(t *testing.T) {
	if _, err := SandboxVarsPath("/tmp/data", "pz_01", "servertest"); err != nil {
		t.Fatalf("valid path: %v", err)
	}
	if _, err := SandboxVarsPath("/tmp/data", "../escape", "servertest"); err == nil {
		t.Fatal("traversal instance id must be rejected")
	}
	if _, err := SandboxVarsPath("/tmp/data", "pz_01", "bad/name"); err == nil {
		t.Fatal("traversal server name must be rejected")
	}
}

// A bare word for a string option is written as an inert quoted literal, so a
// payload cannot become code and the catalogue's string type is honoured.
func TestSandboxBareWordBecomesInertString(t *testing.T) {
	path := writeSandbox(t)
	if err := ApplySandbox(path, map[string]string{"Distribution": "Rural"}); err != nil {
		t.Fatalf("bare word must be accepted as a string: %v", err)
	}
	values, err := ReadSandbox(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["Distribution"] != `"Rural"` {
		t.Fatalf("Distribution = %q, want a quoted literal", values["Distribution"])
	}
	if err := ApplySandbox(path, map[string]string{"Distribution": "os.execute('x')"}); err != nil {
		t.Fatalf("payload-like text must be accepted as inert text: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `Distribution = "os.execute('x')"`) {
		t.Fatalf("payload must be quoted verbatim:\n%s", raw)
	}
}
