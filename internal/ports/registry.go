package ports

import (
	"context"
	"errors"

	"github.com/mj8724/gameserver/internal/domain"
)

// ErrInstanceExists is returned when a registry entry already exists.
var ErrInstanceExists = errors.New("instance already registered")

// ErrInstanceUnknown is returned when no entry matches the instance id.
var ErrInstanceUnknown = errors.New("instance is not registered")

// InstanceRecord is the registry entry for one game server instance (M5).
// The instance state document stays the source of truth for variables and
// ports; this record only answers "which instances exist and where do they
// live", so single-instance deployments keep working unchanged.
type InstanceRecord struct {
	ID         domain.InstanceID `json:"instance_id"`
	TemplateID string            `json:"template_id"`
	Name       string            `json:"name"`
	DataRoot   string            `json:"data_root"`
	CreatedAt  string            `json:"created_at"`
}

// InstanceRegistry lists and manages instances on this host.
type InstanceRegistry interface {
	List(ctx context.Context) ([]InstanceRecord, error)
	Get(ctx context.Context, id domain.InstanceID) (InstanceRecord, error)
	Add(ctx context.Context, record InstanceRecord) error
	Remove(ctx context.Context, id domain.InstanceID) error
}

// PortAllocator hands out free port pairs for a new instance.
type PortAllocator interface {
	// Allocate returns a primary and direct port that no other registered
	// instance uses and that the OS reports free.
	Allocate(ctx context.Context, primaryKey, directKey string) (map[string]int, error)
}

// InstanceLister projects the registry plus live state for the control surface.
// It is a separate port so the legacy Control interface stays unchanged.
type InstanceLister interface {
	ListInstances(ctx context.Context) ([]InstanceView, error)
}

// InstanceView is one row of the multi-instance listing.
type InstanceView struct {
	Record  InstanceRecord
	Running bool
	Ready   bool
	Status  string
	Ports   map[string]int
	Active  bool
}
