// Package valheimplugin registers Valheim as the second game plugin (M4.3).
// The declarative surface is intentionally small: Steam identity, ports,
// readiness and config targets. Launch/readiness wiring is supplied by the
// composition root from Target Manifest evidence, exactly like PZ.
package valheimplugin

import (
	"github.com/mj8724/gameserver/internal/ports"
)

// Plugin implements ports.Plugin for Valheim.
type Plugin struct{}

// New returns the Valheim plugin.
func New() *Plugin { return &Plugin{} }

var _ ports.Plugin = (*Plugin)(nil)

// Descriptor is the declarative identity consumed by shared orchestration.
func (p *Plugin) Descriptor() ports.Descriptor {
	return ports.Descriptor{
		ID:         "valheim",
		Name:       "Valheim",
		SteamAppID: "896660",
		Versions: []ports.VersionSpec{
			{Label: "public", Branch: "public", EvidenceRef: "docs/acceptance/TARGET-MANIFEST-windows.md"},
		},
		Ports: []ports.PortSpec{
			{Key: "SERVER_PORT", Label: "游戏端口", DefaultPort: 2456, Protocol: "udp", Query: true},
			{Key: "DIRECT_PORT", Label: "查询端口", DefaultPort: 2457, Protocol: "udp"},
		},
		Readiness: ports.ReadinessSpec{
			Mode:            "log-marker",
			PortKey:         "SERVER_PORT",
			Marker:          "Game server connected",
			DefaultTimeoutS: 120,
		},
		ConfigTargets: []string{"valheim-config"},
	}
}

// InstallerSource returns the Steam identity and installable versions.
func (p *Plugin) InstallerSource() (string, []ports.VersionSpec) {
	descriptor := p.Descriptor()
	return descriptor.SteamAppID, descriptor.Versions
}

// GameConfig/LaunchSpec/Readiness are supplied by the composition root from
// Target Manifest evidence; a plugin that has not been probed yet fails closed.
func (p *Plugin) GameConfig(ports.PluginDeps) (ports.GameConfig, error) { return nil, errNotWired }

// LaunchSpec returns nil until the launch vector is recorded in the manifest.
func (p *Plugin) LaunchSpec(ports.PluginDeps) (ports.LaunchSpecBuilder, error) {
	return nil, errNotWired
}

// Readiness returns nil until the oracle is recorded in the manifest.
func (p *Plugin) Readiness(ports.PluginDeps) (ports.ReadinessProbe, error) { return nil, errNotWired }

type notWiredError struct{}

func (notWiredError) Error() string {
	return "valheim plugin is not wired yet: record the launch vector and readiness oracle in the Target Manifest first"
}

var errNotWired = notWiredError{}
