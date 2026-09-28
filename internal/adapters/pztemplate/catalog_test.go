package pztemplate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mj8724/gameserver/internal/domain"
)

const validTemplate = `
schema_version: "1.0"
metadata:
  id: "project_zomboid"
  name: "Project Zomboid"
  category: "Survival"
  author: "Official Template"
  version: "1.0.0"
  description: "test"
  supported_os: ["windows", "linux", "darwin"]
steam:
  app_id: "380870"
environments:
  linux:
    executable: "start-server.sh"
    start_arguments: "-adminpassword \"{{ .Variables.ADMIN_PASSWORD }}\""
ports:
  - key: "SERVER_PORT"
    label: "main"
    type: "UDP"
    default: 16261
    is_primary: true
variables:
  - key: "SERVER_NAME"
    label: "name"
    type: "string"
    default: "servertest"
    required: true
    user_editable: true
  - key: "MAX_PLAYERS"
    label: "players"
    type: "number"
    default: 16
    user_editable: true
    validation:
      min: 1
      max: 64
`

func writeTemplate(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogLoadsValidTemplates(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "project_zomboid.yaml", validTemplate)

	catalog, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if warnings := catalog.Warnings(); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	summaries := catalog.List()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %d, want 1", len(summaries))
	}
	summary := summaries[0]
	if summary.ID != "project_zomboid" || summary.AppID != "380870" || len(summary.SupportedOS) != 3 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	template, ok := catalog.Get(domain.TemplateID("project_zomboid"))
	if !ok {
		t.Fatal("template not found")
	}
	variable, ok := template.Variable("MAX_PLAYERS")
	if !ok || variable.Type != "number" || variable.Min == nil || *variable.Min != 1 || variable.Max == nil || *variable.Max != 64 {
		t.Fatalf("unexpected variable: %+v", variable)
	}
	serverName, ok := template.Variable("SERVER_NAME")
	if !ok || !serverName.Required || !serverName.UserEditable || serverName.Default != "servertest" {
		t.Fatalf("unexpected SERVER_NAME: %+v", serverName)
	}
	port, ok := template.Port("SERVER_PORT")
	if !ok || port.Default != 16261 || !port.IsPrimary || port.Protocol != "UDP" {
		t.Fatalf("unexpected port: %+v", port)
	}
}

func TestCatalogSkipsInvalidTemplatesWithoutFailing(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "good.yaml", validTemplate)
	writeTemplate(t, dir, "bad_schema.yaml", "schema_version: \"2.0\"\nmetadata:\n  id: \"x\"\n  name: \"x\"\n")
	writeTemplate(t, dir, "bad_port.yaml", "schema_version: \"1.0\"\nmetadata:\n  id: \"y\"\n  name: \"y\"\nports:\n  - key: \"P\"\n    default: 70000\n")
	writeTemplate(t, dir, "not_yaml.txt", "ignored")

	catalog, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.List()) != 1 {
		t.Fatalf("expected only the valid template, got %d", len(catalog.List()))
	}
	if len(catalog.Warnings()) != 2 {
		t.Fatalf("expected 2 warnings, got %v", catalog.Warnings())
	}
}

func TestCatalogRejectsMissingDirectory(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("empty directory accepted")
	}
	if _, err := New(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory accepted")
	}
}
