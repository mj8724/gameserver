package ports

import "github.com/mj8724/gameserver/internal/domain"

// Game plugin contract (M4). A plugin describes one game server and supplies
// the port implementations the shared orchestration consumes; registration is
// compile-time (no dynamic loading, no scripts, no exec of plugin code).
//
// Shared orchestration (application/ports) never names a game: everything
// game-specific arrives through this contract, and the archtest rules forbid
// game identifiers in those layers.

// PortSpec declares one game port the server binds.
type PortSpec struct {
	Key         string // template variable / state key, e.g. SERVER_PORT
	Label       string
	DefaultPort int
	Protocol    string // udp | tcp
	Query       bool   // participates in the A2S/query surface
}

// ReadinessSpec declares how a plugin decides "the server is up".
type ReadinessSpec struct {
	// Mode is "a2s" or "log-marker"; a log marker requires an evidence
	// reference recorded in the Target Manifest (unchanged rule).
	Mode            string
	PortKey         string
	Marker          string
	DefaultTimeoutS int
}

// VersionSpec is one installable server version/branch of a plugin.
type VersionSpec struct {
	Label       string
	Branch      string
	BuildID     string
	EvidenceRef string
}

// Descriptor is the declarative identity of a plugin.
type Descriptor struct {
	ID            domain.TemplateID
	Name          string
	SteamAppID    string
	Versions      []VersionSpec
	Ports         []PortSpec
	Readiness     ReadinessSpec
	ConfigTargets []string // e.g. ["ini", "sandboxvars"]
	// InstallMarkers are the artifact names whose presence proves the game is
	// installed. They replace game-specific detection in shared code.
	InstallMarkers []string
}

// Plugin supplies the adapters for one game. Dependency structs carry only
// what the plugin needs, so a plugin cannot reach into the whole runtime.
type Plugin interface {
	Descriptor() Descriptor

	// GameConfig returns the configuration adapter for this game.
	GameConfig(deps PluginDeps) (GameConfig, error)
	// LaunchSpec builds the typed executable argv for a launch request.
	LaunchSpec(deps PluginDeps) (LaunchSpecBuilder, error)
	// Readiness builds the readiness oracle from the descriptor.
	Readiness(deps PluginDeps) (ReadinessProbe, error)
	// InstallerSource is the Steam app identity the installer needs.
	InstallerSource() (appID string, versions []VersionSpec)
}

// PluginDeps is the game-agnostic dependency bundle handed to a plugin.
type PluginDeps struct {
	DataRoot          string
	ServersRoot       string
	Instance          domain.InstanceID
	Platform          string
	LaunchVector      string
	EvidenceRef       string
	MemoryMB          int
	ReadinessPort     int
	ReadinessTimeoutS int
	Resolver          func(ctx interface{ Done() <-chan struct{} }, id domain.InstanceID) (string, error)
}
