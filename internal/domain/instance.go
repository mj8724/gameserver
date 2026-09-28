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
}
