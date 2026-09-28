// Package domain contains transport- and infrastructure-independent game server types.
package domain

// InstanceID identifies a single local game server instance.
type InstanceID string

// InstanceState is the persisted state consumed by application use cases.
type InstanceState struct {
	ID         InstanceID     `json:"instance_id"`
	Name       string         `json:"name"`
	TemplateID string         `json:"template_id"`
	Variables  map[string]any `json:"variables"`
	Ports      map[string]int `json:"ports"`
	Mods       map[string]any `json:"mods,omitempty"`
	Billing    map[string]any `json:"billing,omitempty"`
	QuotaGB    float64        `json:"quota_gb,omitempty"`
}

// Clone returns a deep-enough copy for mutation by use cases.
func (s InstanceState) Clone() InstanceState {
	clone := s
	clone.Variables = cloneAnyMap(s.Variables)
	clone.Mods = cloneAnyMap(s.Mods)
	clone.Billing = cloneAnyMap(s.Billing)
	ports := make(map[string]int, len(s.Ports))
	for key, value := range s.Ports {
		ports[key] = value
	}
	clone.Ports = ports
	return clone
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		switch typed := value.(type) {
		case map[string]any:
			output[key] = cloneAnyMap(typed)
		case []any:
			items := make([]any, len(typed))
			copy(items, typed)
			output[key] = items
		default:
			output[key] = value
		}
	}
	return output
}
