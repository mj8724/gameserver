// Package plugins is the compile-time game plugin registry (M4). Plugins
// register themselves from their own package; the composition root imports
// them explicitly, so there is no dynamic loading and no plugin code exec.
package plugins

import (
	"sort"
	"sync"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// Registry is the process-wide, write-once plugin table.
type Registry struct {
	mu      sync.RWMutex
	plugins map[domain.TemplateID]ports.Plugin
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{plugins: map[domain.TemplateID]ports.Plugin{}}
}

// Register adds a plugin. Registering the same template twice panics: it is a
// programming error surfaced at startup, never a runtime condition.
func (r *Registry) Register(plugin ports.Plugin) {
	descriptor := plugin.Descriptor()
	if descriptor.ID == "" {
		panic("plugin registration without a template id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[descriptor.ID]; exists {
		panic("duplicate plugin registration for " + string(descriptor.ID))
	}
	r.plugins[descriptor.ID] = plugin
}

// Lookup resolves a template id, failing closed when unknown.
func (r *Registry) Lookup(id domain.TemplateID) (ports.Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plugin, ok := r.plugins[id]
	return plugin, ok
}

// List returns the registered descriptors in stable order.
func (r *Registry) List() []ports.Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	descriptors := make([]ports.Descriptor, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		descriptors = append(descriptors, plugin.Descriptor())
	}
	sort.Slice(descriptors, func(i, j int) bool { return descriptors[i].ID < descriptors[j].ID })
	return descriptors
}

var _ ports.PluginRegistry = (*Registry)(nil)
