package optioncatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodCatalog = `schema_version: "1.0"
template_id: "project_zomboid"
source:
  build_id: "42.21"
  extracted_at: "2026-09-29T07:13:00Z"
  command: "steamcmd +app_info_print"
  files:
    - path: "servertest.ini"
      sha256: "cf8c8dd5fa609ecce4e7fa5a34539658e3681940c284a59652415f0598b6a934"
options:
  - target: "ini"
    key: "Public"
    label: "Public"
    type: "bool"
    secret: false
    default: "true"
    group: "server"
    requires_restart: true
    writable: "ro"
    clearable: true
  - target: "ini"
    key: "RCONPassword"
    label: "RCONPassword"
    type: "string"
    secret: true
    default: ""
    group: "access"
    requires_restart: true
    writable: "rw"
    clearable: false
  - target: "sandboxvars"
    path: "ZombieLore"
    label: "ZombieLore"
    type: "enum"
    secret: false
    default: "1"
    enum: ["1", "2"]
    group: "sandbox"
    requires_restart: true
    writable: "rw"
    clearable: true
`

func write(t *testing.T, body string) (dir, template string) {
	t.Helper()
	dir = t.TempDir()
	template = "project_zomboid"
	if err := os.WriteFile(filepath.Join(dir, template+".options.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, template
}

func TestLoadValidCatalogue(t *testing.T) {
	dir, template := write(t, goodCatalog)
	catalog, err := New(dir, template)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(catalog.Options()) != 3 {
		t.Fatalf("options = %d", len(catalog.Options()))
	}
	if source := catalog.Source(); source.BuildID != "42.21" || len(source.SHA256) != 1 {
		t.Fatalf("source = %+v", source)
	}
	if spec, ok := catalog.Get("ini", "RCONPassword"); !ok || !spec.Secret || spec.Clearable {
		t.Fatalf("secret entry = %+v ok=%t", spec, ok)
	}
	if _, ok := catalog.Get("ini", "Unknown"); ok {
		t.Fatal("unknown key must not resolve")
	}
	if _, ok := catalog.Get("sandboxvars", "ZombieLore"); !ok {
		t.Fatal("sandbox path must resolve")
	}
}

func TestLoadRejectsBadCatalogues(t *testing.T) {
	cases := map[string]string{
		"unknown field":       strings.Replace(goodCatalog, `schema_version: "1.0"`, "schema_version: \"1.0\"\nbogus: true", 1),
		"bad schema":          strings.Replace(goodCatalog, `schema_version: "1.0"`, `schema_version: "9.9"`, 1),
		"template mismatch":   strings.Replace(goodCatalog, `template_id: "project_zomboid"`, `template_id: "other"`, 1),
		"missing source":      strings.Replace(goodCatalog, `  build_id: "42.21"`, `  build_id: ""`, 1),
		"short sha":           strings.Replace(goodCatalog, "cf8c8dd5fa609ecce4e7fa5a34539658e3681940c284a59652415f0598b6a934", "deadbeef", 1),
		"bad type":            strings.Replace(goodCatalog, `type: "bool"`, `type: "date"`, 1),
		"enum without values": strings.Replace(goodCatalog, `    enum: ["1", "2"]`, `    enum: []`, 1),
		"duplicate key": goodCatalog + `  - target: "ini"
    key: "Public"
    label: "Public"
    type: "bool"
    default: "true"
    writable: "ro"
`,
		"secret clearable": strings.Replace(goodCatalog, `    writable: "rw"
    clearable: false`, `    writable: "rw"
    clearable: true`, 1),
		"unknown writable": strings.Replace(goodCatalog, `writable: "ro"`, `writable: "maybe"`, 1),
		"ini with path":    strings.Replace(goodCatalog, `    key: "Public"`, "    key: \"Public\"\n    path: \"Nope\"", 1),
		"min above max":    strings.Replace(goodCatalog, `    group: "server"`, "    min: 10\n    max: 1\n    group: \"server\"", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir, template := write(t, body)
			if _, err := New(dir, template); err == nil {
				t.Fatalf("expected rejection for %s", name)
			}
		})
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	if _, err := New(t.TempDir(), "project_zomboid"); err == nil {
		t.Fatal("missing catalogue must be rejected")
	}
}

// The generated repository catalogue must satisfy the same validation.
func TestRepositoryCatalogueLoads(t *testing.T) {
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := New(filepath.Join(repo, "catalogs"), "project_zomboid")
	if err != nil {
		t.Fatalf("repository catalogue: %v", err)
	}
	options := catalog.Options()
	if len(options) < 400 {
		t.Fatalf("repository catalogue is unexpectedly small: %d", len(options))
	}
	readonly := 0
	for _, option := range options {
		if option.Writable != "rw" {
			readonly++
		}
	}
	if readonly == 0 {
		t.Fatal("variable-bound keys must be marked read-only in the catalogue")
	}
}
