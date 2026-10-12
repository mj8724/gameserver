package ports

import (
	"errors"

	"github.com/mj8724/gameserver/internal/domain"
)

// PluginRegistry resolves game plugins at composition time. Registration is
// explicit and compile-time; an unknown template id fails closed.
type PluginRegistry interface {
	Lookup(id domain.TemplateID) (Plugin, bool)
	List() []Descriptor
}

// ErrUnknownPlugin is returned when no plugin is registered for a template.
var ErrUnknownPlugin = errors.New("no game plugin registered for template")
