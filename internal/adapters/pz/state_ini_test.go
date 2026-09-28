package pz

import (
	"encoding/json"
	"testing"

	"github.com/mj8724/gameserver/internal/domain"
)

func TestManagedINIUpdatesMapsOnlyManagedKeys(t *testing.T) {
	updates := ManagedINIUpdates(
		map[string]any{
			"PUBLIC_SERVER":     false,
			"SERVER_PASSWORD":   "join-secret",
			"MAX_PLAYERS":       24,
			"PVP_ENABLED":       true,
			"OPEN_REGISTRATION": false,
			"PAUSE_EMPTY":       true,
			"UNRELATED":         "must not leak",
		},
		map[string]int{"SERVER_PORT": 16261, "DIRECT_PORT": 16262},
		map[string]any{"workshop_ids": []string{"123", "456"}, "mod_names": []string{"modA", "modB"}},
	)
	want := map[string]string{
		"Public":        "false",
		"Password":      "join-secret",
		"MaxPlayers":    "24",
		"PVP":           "true",
		"Open":          "false",
		"PauseEmpty":    "true",
		"DefaultPort":   "16261",
		"UDPPort":       "16262",
		"WorkshopItems": "123;456",
		"Mods":          "modA;modB",
	}
	if len(updates) != len(want) {
		t.Fatalf("managed key count = %d, want %d (%v)", len(updates), len(want), updates)
	}
	for key, value := range want {
		if updates[key] != value {
			t.Fatalf("update[%s] = %q, want %q", key, updates[key], value)
		}
	}
	if _, ok := updates["UNRELATED"]; ok {
		t.Fatal("unmanaged variable leaked into INI updates")
	}
}

func TestManagedINIUpdatesClearsEmptyModListsAndFallsBackSafely(t *testing.T) {
	updates := ManagedINIUpdates(nil, nil, map[string]any{"workshop_ids": []any{}, "mod_names": nil})
	if updates["WorkshopItems"] != "" || updates["Mods"] != "" {
		t.Fatalf("empty mod lists must serialize to empty strings: %+v", updates)
	}
	if updates["MaxPlayers"] != "16" || updates["DefaultPort"] != "16261" || updates["UDPPort"] != "16262" {
		t.Fatalf("unexpected fallbacks: %+v", updates)
	}
	if updates["Public"] != "true" || updates["PVP"] != "true" {
		t.Fatalf("unexpected boolean fallbacks: %+v", updates)
	}
}

// The HTTP layer decodes request bodies with json.Number, so managed variable
// values arrive as json.Number, not float64.
func TestManagedINIUpdatesAcceptsJSONNumberVariables(t *testing.T) {
	updates := ManagedINIUpdates(
		map[string]any{"MAX_PLAYERS": json.Number("24"), "SERVER_PASSWORD": "join"},
		map[string]int{"SERVER_PORT": 16261},
		map[string]any{},
	)
	if updates["MaxPlayers"] != "24" {
		t.Fatalf("json.Number MAX_PLAYERS = %q, want 24", updates["MaxPlayers"])
	}
	if updates["Password"] != "join" {
		t.Fatalf("password = %q", updates["Password"])
	}
}

func TestReadinessPortKeyPrefersPrimaryPort(t *testing.T) {
	template := domain.Template{Ports: []domain.TemplatePort{
		{Key: "SECONDARY", Default: 1},
		{Key: "SERVER_PORT", Default: 16261, IsPrimary: true},
	}}
	if key := ReadinessPortKey(template); key != "SERVER_PORT" {
		t.Fatalf("primary port key = %q", key)
	}
	if key := ReadinessPortKey(domain.Template{}); key != "SERVER_PORT" {
		t.Fatalf("fallback port key = %q", key)
	}
}
