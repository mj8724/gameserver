// Package ports declares outbound interfaces consumed by application use cases.
package ports

import (
	"context"
	"io/fs"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
)

// OptionTarget names the physical file an option lives in.
type OptionTarget string

const (
	OptionTargetINI         OptionTarget = "ini"
	OptionTargetSandboxVars OptionTarget = "sandboxvars"
	OptionTargetLaunch      OptionTarget = "launch"
)

// Writable class of a catalogue entry. Exactly one writer owns a physical key:
// rw = the options path, ro = another path (variables/mods API), hidden = never.
const (
	OptionWritableRW     = "rw"
	OptionWritableRO     = "ro"
	OptionWritableHidden = "hidden"
)

// OptionSpec is one catalogue entry generated from the vendor configuration.
type OptionSpec struct {
	Target          OptionTarget
	Key             string // INI key
	Path            string // SandboxVars table path
	Label           string
	Type            string // bool|int|float|enum|string
	Secret          bool
	Default         string
	Min             *float64
	Max             *float64
	Enum            []string
	Group           string
	Description     string
	RequiresRestart bool
	Writable        string
	Clearable       bool
}

// Name is the physical key within its target.
func (o OptionSpec) Name() string {
	if o.Key != "" {
		return o.Key
	}
	return o.Path
}

// CatalogSource records where the catalogue came from so it can be re-verified.
type CatalogSource struct {
	BuildID     string
	Files       []string
	SHA256      []string
	ExtractedAt string
	Command     string
}

// OptionCatalog exposes the validated option catalogue for a template.
type OptionCatalog interface {
	Options() []OptionSpec
	Get(target OptionTarget, name string) (OptionSpec, bool)
	Source() CatalogSource
}

// StateStore loads and atomically persists one instance's domain state.
type StateStore interface {
	Load(context.Context, domain.InstanceID) (domain.InstanceState, error)
	Save(context.Context, domain.InstanceState) error
}

// GameConfig reads and applies game-specific configuration.
type GameConfig interface {
	Read(context.Context, domain.InstanceID) (map[string]string, error)
	Apply(context.Context, domain.InstanceID, map[string]string) error
	// ReadOptions returns the current value of every catalogue-relevant key
	// (INI plus sandbox) so the console can show file-backed values.
	ReadOptions(context.Context, domain.InstanceID) (map[string]string, error)
	// ApplyOptionValues writes catalogue-validated values, routing each name to
	// the file that currently defines it. Validation and ownership are decided
	// by the caller (application + catalogue), not by the adapter.
	ApplyOptionValues(context.Context, domain.InstanceID, map[string]string) error
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
// Implementations are pre-configured with the servers root so that application
// code never handles filesystem paths.
type InstanceLock interface {
	Acquire(context.Context, domain.InstanceID) (Lease, error)
}

// StaticAssets opens a read-only static asset by a validated relative name.
type StaticAssets interface {
	Open(string) (fs.File, error)
}

// Clock abstracts wall time for deterministic application tests.
type Clock interface {
	Now() time.Time
}

// TemplateCatalog exposes the declarative game templates shipped with the
// application. Implementations must never enable an unreviewed template.
type TemplateCatalog interface {
	List() []domain.TemplateSummary
	Get(domain.TemplateID) (domain.Template, bool)
}

// LogSubscription streams new process log lines until Close is called.
// Close is idempotent and must unregister the underlying listener.
type LogSubscription interface {
	Lines() <-chan string
	Close()
}

// LogSource exposes the bounded in-memory log buffer and live subscriptions.
type LogSource interface {
	Recent(limit int) []string
	Subscribe(buffer int) LogSubscription
}

// ProcessStatus is the observable process state used by status projections.
type ProcessStatus struct {
	Running       bool
	Status        string
	PID           int
	CPUPercent    float64
	MemoryMB      float64
	UptimeSeconds int
}

// ProcessStatusProvider reports process state without control authority.
type ProcessStatusProvider interface {
	ProcessStatus(context.Context, domain.InstanceID) (ProcessStatus, error)
}

// InstanceFiles resolves instance data locations and install presence.
type InstanceFiles interface {
	InstanceRoot(domain.InstanceID) (string, error)
	InstallDir(domain.InstanceID) (string, error)
	CacheDir(domain.InstanceID) (string, error)
	IsInstalled(domain.InstanceID) bool
}

// LaunchInput carries the state-derived launch choices for one start request.
// Executable selection is decided by reviewed artifact evidence, never by
// template metadata.
type LaunchInput struct {
	InstallDir       string
	CacheDir         string
	Platform         string
	ServerName       string
	AdminPass        string
	ExecutableName   string
	DirectExecutable bool
	EvidenceRef      string
	Vector           string
	MemoryMB         int
}

// LaunchSpecBuilder builds a typed launch spec from instance state.
type LaunchSpecBuilder func(context.Context, domain.InstanceState, LaunchInput) (LaunchSpec, error)

// StateConfigApplier synchronizes persisted instance state into the game's own
// configuration files (for example the PZ INI).
type StateConfigApplier func(context.Context, domain.InstanceState) error
