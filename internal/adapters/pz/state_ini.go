package pz

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/mj8724/gameserver/internal/domain"
)

// ManagedINIUpdates maps persisted instance state to the INI keys this adapter
// owns. Keys that are not managed here are preserved untouched by
// Apply/ApplyNamed.
func ManagedINIUpdates(variables map[string]any, ports map[string]int, mods map[string]any) map[string]string {
	return map[string]string{
		"Public":        boolText(variables["PUBLIC_SERVER"], true),
		"Password":      stringValue(variables["SERVER_PASSWORD"]),
		"MaxPlayers":    numberText(variables["MAX_PLAYERS"], 16),
		"PVP":           boolText(variables["PVP_ENABLED"], true),
		"Open":          boolText(variables["OPEN_REGISTRATION"], true),
		"PauseEmpty":    boolText(variables["PAUSE_EMPTY"], true),
		"DefaultPort":   portText(ports, "SERVER_PORT", 16261),
		"UDPPort":       portText(ports, "DIRECT_PORT", 16262),
		"WorkshopItems": strings.Join(stringList(mods["workshop_ids"]), ";"),
		"Mods":          strings.Join(stringList(mods["mod_names"]), ";"),
	}
}

// VariableOwnedINIKeys are the INI keys written through the legacy variables
// path (template variables). Everything else in the catalogue belongs to the
// options path: one physical key, exactly one writer (plan D-C).
func VariableOwnedINIKeys() map[string]bool {
	return map[string]bool{
		"Public":        true,
		"Password":      true,
		"MaxPlayers":    true,
		"PVP":           true,
		"Open":          true,
		"PauseEmpty":    true,
		"DefaultPort":   true,
		"UDPPort":       true,
		"WorkshopItems": true,
		"Mods":          true,
	}
}

// VariableOwnedUpdates keeps only the keys the variables path owns, so a
// variable save can never overwrite an option-owned value (and vice versa).
func VariableOwnedUpdates(updates map[string]string) map[string]string {
	owned := VariableOwnedINIKeys()
	filtered := make(map[string]string, len(owned))
	for key, value := range updates {
		if owned[key] {
			filtered[key] = value
		}
	}
	return filtered
}

// OptionOwnedUpdates is the complement: the keys the options path owns.
func OptionOwnedUpdates(updates map[string]string) map[string]string {
	owned := VariableOwnedINIKeys()
	filtered := make(map[string]string, len(updates))
	for key, value := range updates {
		if !owned[key] {
			filtered[key] = value
		}
	}
	return filtered
}

// ReadinessPortKey reports the template's primary port key used for probes.
func ReadinessPortKey(template domain.Template) string {
	for _, port := range template.Ports {
		if port.IsPrimary {
			return port.Key
		}
	}
	return "SERVER_PORT"
}

func boolText(value any, fallback bool) string {
	switch typed := value.(type) {
	case bool:
		return strconv.FormatBool(typed)
	case string:
		if typed == "true" || typed == "false" {
			return typed
		}
	}
	return strconv.FormatBool(fallback)
}

// portText returns the configured port text, falling back when the key is
// absent or outside the valid range.
func portText(ports map[string]int, key string, fallback int) string {
	value, ok := ports[key]
	if !ok || value < 1 || value > 65535 {
		return strconv.Itoa(fallback)
	}
	return strconv.Itoa(value)
}

// numberText formats a persisted/config variable. Values reaching this adapter
// from the HTTP layer are json.Number (strict decoding), so that case must be
// handled explicitly: an unhandled type silently falls back to the default and
// the INI is never actually updated.
func numberText(value any, fallback int) string {
	switch typed := value.(type) {
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.Itoa(int(typed))
	case json.Number:
		if _, err := typed.Int64(); err == nil {
			return typed.String()
		}
	case string:
		if _, err := strconv.Atoi(typed); err == nil {
			return typed
		}
	}
	return strconv.Itoa(fallback)
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return []string{}
	}
}
