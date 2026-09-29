package pztemplate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateVersionsParsedAndValidated(t *testing.T) {
	dir := t.TempDir()
	body := `schema_version: "1.0"
metadata:
  id: "demo"
  name: "Demo"
steam:
  app_id: "1"
  versions:
    - label: "稳定版"
      branch: "public"
      build_id: "1"
      default: true
      evidence_ref: "docs/x.md#1"
    - label: "测试版"
      branch: "unstable"
      evidence_ref: "docs/x.md#1"
`
	if err := os.WriteFile(filepath.Join(dir, "demo.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := New(dir)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	template, ok := catalog.Get("demo")
	if !ok {
		t.Fatal("template missing")
	}
	if len(template.Versions) != 2 || !template.Versions[0].Default || template.Versions[1].Branch != "unstable" {
		t.Fatalf("versions = %+v", template.Versions)
	}

	bad := []string{
		`    - label: "x"` + "\n" + `      branch: "public"` + "\n",                                        // missing evidence
		`    - label: "x"` + "\n" + `      branch: "bad branch"` + "\n" + `      evidence_ref: "a"` + "\n", // bad branch
		`    - label: "x"` + "\n" + `      branch: "public"` + "\n" + `      evidence_ref: "a"` + "\n" +
			`    - label: "y"` + "\n" + `      branch: "public"` + "\n" + `      evidence_ref: "b"` + "\n", // duplicate
	}
	for index, fragment := range bad {
		badDir := t.TempDir()
		content := `schema_version: "1.0"` + "\nmetadata:\n  id: \"demo\"\n  name: \"Demo\"\nsteam:\n  app_id: \"1\"\n  versions:\n" + fragment
		if err := os.WriteFile(filepath.Join(badDir, "demo.yaml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		catalog, err := New(badDir)
		if err != nil {
			continue // rejected at load time is acceptable
		}
		if _, ok := catalog.Get("demo"); ok {
			t.Fatalf("case %d must be rejected", index)
		}
	}
}
