// Package ports declares outbound interfaces consumed by application use cases.
package ports

import (
	"context"
	"io/fs"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
)

// StateStore loads and atomically persists one instance's domain state.
type StateStore interface {
	Load(context.Context, domain.InstanceID) (domain.InstanceState, error)
	Save(context.Context, domain.InstanceState) error
}

// GameConfig reads and applies game-specific configuration.
type GameConfig interface {
	Read(context.Context, domain.InstanceID) (map[string]string, error)
	Apply(context.Context, domain.InstanceID, map[string]string) error
}

// InstallRequest describes a server installation without shell command text.
type InstallRequest struct {
	InstanceID domain.InstanceID
	Validate   bool
}

// Progress reports installer progress.
type Progress struct {
	Percent float64
	Message string
}

// Installer runs a cancellable server install/update operation.
type Installer interface {
	Install(context.Context, InstallRequest, func(Progress)) error
}

// LaunchSpec is an executable plus typed arguments and an explicit working directory.
type LaunchSpec struct {
	Executable string
	Args       []string
	WorkDir    string
	Env        map[string]string
}

// Process describes an owned child process.
type Process struct {
	PID int
}

// ProcessSupervisor owns the local game process lifecycle.
type ProcessSupervisor interface {
	Start(context.Context, LaunchSpec) (Process, error)
	Stop(context.Context, domain.InstanceID) error
	Kill(context.Context, domain.InstanceID) error
	SendInput(context.Context, domain.InstanceID, string) error
}

// ReadinessProbe checks whether a process is ready to accept game clients.
type ReadinessProbe interface {
	Ready(context.Context, domain.InstanceID) (bool, error)
}

// Lease represents exclusive ownership of an instance data root.
type Lease interface {
	Release() error
}

// InstanceLock acquires the cross-process single-writer lease for an instance.
type InstanceLock interface {
	Acquire(context.Context, string, domain.InstanceID) (Lease, error)
}

// StaticAssets opens a read-only static asset by a validated relative name.
type StaticAssets interface {
	Open(string) (fs.File, error)
}

// Clock abstracts wall time for deterministic application tests.
type Clock interface {
	Now() time.Time
}
