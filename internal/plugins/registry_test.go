package plugins

import (
	"testing"

	"github.com/mj8724/gameserver/internal/plugins/pzplugin"
	"github.com/mj8724/gameserver/internal/plugins/valheimplugin"
	"github.com/mj8724/gameserver/internal/ports"
)

// The registry resolves registered plugins by template id and fails closed for
// anything else, so shared orchestration never falls back to a default game.
func TestRegistryLookupFailsClosed(t *testing.T) {
	registry := New()
	registry.Register(pzplugin.New())
	registry.Register(valheimplugin.New())

	if _, ok := registry.Lookup("project_zomboid"); !ok {
		t.Fatal("project_zomboid must resolve")
	}
	if _, ok := registry.Lookup("valheim"); !ok {
		t.Fatal("valheim must resolve")
	}
	if plugin, ok := registry.Lookup("unknown_game"); ok || plugin != nil {
		t.Fatalf("unknown template must fail closed, got %v", plugin)
	}
}

// Both plugins expose the declarative surface shared orchestration consumes:
// Steam identity, ports and a readiness spec.
func TestPluginDescriptorsAreComplete(t *testing.T) {
	for _, plugin := range []ports.Plugin{pzplugin.New(), valheimplugin.New()} {
		descriptor := plugin.Descriptor()
		if descriptor.ID == "" || descriptor.SteamAppID == "" {
			t.Fatalf("descriptor identity incomplete: %+v", descriptor)
		}
		if len(descriptor.Ports) == 0 {
			t.Fatalf("%s must declare ports", descriptor.ID)
		}
		if descriptor.Readiness.Mode == "" || descriptor.Readiness.PortKey == "" {
			t.Fatalf("%s must declare a readiness spec", descriptor.ID)
		}
		appID, versions := plugin.InstallerSource()
		if appID != descriptor.SteamAppID || len(versions) == 0 {
			t.Fatalf("%s installer source mismatch: %s %d versions", descriptor.ID, appID, len(versions))
		}
	}
}

// Registration of the same template twice is a programming error, not a
// silent override.
func TestDuplicateRegistrationPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate registration must panic")
		}
	}()
	registry := New()
	registry.Register(pzplugin.New())
	registry.Register(pzplugin.New())
}

// The plugin list is stable so status/UI output does not flap.
func TestRegistryListIsStable(t *testing.T) {
	registry := New()
	registry.Register(valheimplugin.New())
	registry.Register(pzplugin.New())
	first := registry.List()
	second := registry.List()
	if len(first) != 2 {
		t.Fatalf("expected 2 descriptors, got %d", len(first))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("list order changed: %v vs %v", first, second)
		}
	}
	if first[0].ID != "project_zomboid" || first[1].ID != "valheim" {
		t.Fatalf("expected sorted descriptors, got %v, %v", first[0].ID, first[1].ID)
	}
}
