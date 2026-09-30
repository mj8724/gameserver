package instanceregistry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

func seedInstance(t *testing.T, dataRoot, id, templateID string, ports map[string]any) {
	t.Helper()
	dir := filepath.Join(dataRoot, "servers", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	document := `{"instance_id":"` + id + `","name":"` + id + `","template_id":"` + templateID + `","ports":` + toJSON(t, ports) + `}`
	if err := os.WriteFile(filepath.Join(dir, "instance.json"), []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
}

func toJSON(t *testing.T, value any) string {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		out := "{"
		first := true
		for key, raw := range typed {
			if !first {
				out += ","
			}
			first = false
			out += `"` + key + `":` + toJSON(t, raw)
		}
		return out + "}"
	case int:
		return itoa(typed)
	default:
		t.Fatalf("unsupported test value %T", value)
		return ""
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

// A legacy single-instance deployment is discovered on read, so the registry
// never hides an instance that exists on disk.
func TestRegistryDiscoversLegacyInstance(t *testing.T) {
	root := t.TempDir()
	seedInstance(t, root, "pz_01", "project_zomboid", map[string]any{"SERVER_PORT": 16261, "DIRECT_PORT": 16262})
	registry, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := registry.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != "pz_01" || records[0].TemplateID != "project_zomboid" {
		t.Fatalf("legacy instance not discovered: %+v", records)
	}
}

// Adding is idempotent-by-refusal: a duplicate id never silently overwrites.
func TestRegistryAddRefusesDuplicate(t *testing.T) {
	root := t.TempDir()
	registry, _ := New(root)
	record := ports.InstanceRecord{ID: "valheim_01", TemplateID: "valheim", Name: "Valheim"}
	if err := registry.Add(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(context.Background(), record); !errors.Is(err, ports.ErrInstanceExists) {
		t.Fatalf("duplicate add = %v, want ErrInstanceExists", err)
	}
	got, err := registry.Get(context.Background(), domain.InstanceID("valheim_01"))
	if err != nil || got.TemplateID != "valheim" {
		t.Fatalf("registered record unreadable: %+v %v", got, err)
	}
}

// Removing an unknown instance is an error, and removal never deletes data.
func TestRegistryRemoveKeepsData(t *testing.T) {
	root := t.TempDir()
	seedInstance(t, root, "pz_01", "project_zomboid", map[string]any{"SERVER_PORT": 16261})
	registry, _ := New(root)
	if err := registry.Add(context.Background(), ports.InstanceRecord{ID: "pz_01", TemplateID: "project_zomboid"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Remove(context.Background(), "pz_01"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "servers", "pz_01", "instance.json")); err != nil {
		t.Fatalf("removal must not delete instance data: %v", err)
	}
	if err := registry.Remove(context.Background(), "nope"); !errors.Is(err, ports.ErrInstanceUnknown) {
		t.Fatalf("unknown removal = %v, want ErrInstanceUnknown", err)
	}
}

// Port allocation avoids ports other instances already use and hands out a
// non-colliding pair.
func TestPortAllocatorAvoidsUsedPorts(t *testing.T) {
	root := t.TempDir()
	seedInstance(t, root, "pz_01", "project_zomboid", map[string]any{"SERVER_PORT": 27015, "DIRECT_PORT": 27016})
	registry, _ := New(root)
	allocator := &PortAllocator{Registry: registry, Start: 27015, Span: 20}

	got, err := allocator.Allocate(context.Background(), "SERVER_PORT", "DIRECT_PORT")
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	primary, direct := got["SERVER_PORT"], got["DIRECT_PORT"]
	if primary == 27015 || primary == 27016 {
		t.Fatalf("allocator reused an in-use port: %d", primary)
	}
	if direct != primary+1 {
		t.Fatalf("pair must be consecutive and non-colliding: %d/%d", primary, direct)
	}
}

// The registry file is replaced atomically and verified by readback.
func TestRegistryWriteIsVerified(t *testing.T) {
	root := t.TempDir()
	registry, _ := New(root)
	if err := registry.Add(context.Background(), ports.InstanceRecord{ID: "a", TemplateID: "project_zomboid"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(context.Background(), ports.InstanceRecord{ID: "b", TemplateID: "valheim"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, registryFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("registry file is empty")
	}
	records, err := registry.read()
	if err != nil || len(records) != 2 {
		t.Fatalf("registry readback mismatch: %+v %v", records, err)
	}
}
