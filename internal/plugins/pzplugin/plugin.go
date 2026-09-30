// Package pzplugin registers Project Zomboid as the first game plugin. It only
// wires existing adapters behind the plugin contract; behaviour is unchanged.
package pzplugin

import (
	"github.com/mj8724/gameserver/internal/adapters/pz"
	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// Plugin implements ports.Plugin for Project Zomboid.
type Plugin struct {
	// Config is built lazily per composition so the registry stays stateless.
	ConfigFactory func(deps ports.PluginDeps) (*pz.Config, error)
}

// New returns the Project Zomboid plugin.
func New() *Plugin { return &Plugin{} }

var _ ports.Plugin = (*Plugin)(nil)

// Descriptor is the declarative identity consumed by shared orchestration.
func (p *Plugin) Descriptor() ports.Descriptor {
	return ports.Descriptor{
		ID:         "project_zomboid",
		Name:       "Project Zomboid",
		SteamAppID: "380870",
		Versions: []ports.VersionSpec{
			{Label: "public", Branch: "public", BuildID: "25485538", EvidenceRef: "docs/acceptance/evidence/pz-appinfo-380870.txt"},
			{Label: "unstable", Branch: "unstable", BuildID: "25485538", EvidenceRef: "docs/acceptance/evidence/pz-appinfo-380870.txt"},
			{Label: "legacy41", Branch: "legacy41", BuildID: "24928750", EvidenceRef: "docs/acceptance/evidence/pz-appinfo-380870.txt"},
			{Label: "42.19", Branch: "42.19", BuildID: "24929695", EvidenceRef: "docs/acceptance/evidence/pz-appinfo-380870.txt"},
		},
		Ports: []ports.PortSpec{
			{Key: "SERVER_PORT", Label: "游戏端口", DefaultPort: 16261, Protocol: "udp", Query: true},
			{Key: "DIRECT_PORT", Label: "直连端口", DefaultPort: 16262, Protocol: "udp"},
		},
		Readiness: ports.ReadinessSpec{
			Mode:            "log-marker",
			PortKey:         "SERVER_PORT",
			Marker:          "*** SERVER STARTED ***",
			DefaultTimeoutS: 60,
		},
		ConfigTargets: []string{"ini", "sandboxvars"},
	}
}

// InstallerSource returns the Steam identity and installable versions.
func (p *Plugin) InstallerSource() (string, []ports.VersionSpec) {
	descriptor := p.Descriptor()
	return descriptor.SteamAppID, descriptor.Versions
}

// GameConfig, LaunchSpec and Readiness return (nil, nil): Project Zomboid's
// INI/SandboxVars writer, launcher-descriptor vector and log-marker oracle are
// assembled by the composition root, which owns the Target Manifest evidence
// and the configuration ownership matrix. A nil result means "the plugin does
// not supply this port"; an error would mean the plugin tried and failed.
func (p *Plugin) GameConfig(ports.PluginDeps) (ports.GameConfig, error) { return nil, nil }

// LaunchSpec is supplied by the composition root (see the note above).
func (p *Plugin) LaunchSpec(ports.PluginDeps) (ports.LaunchSpecBuilder, error) { return nil, nil }

// Readiness is supplied by the composition root (log-marker oracle with the
// manifest-recorded window).
func (p *Plugin) Readiness(ports.PluginDeps) (ports.ReadinessProbe, error) { return nil, nil }

// Name reports the game id for logs.
func (p *Plugin) Name() string { return string(p.Descriptor().ID) }

// TemplateID is the template this plugin serves.
func (p *Plugin) TemplateID() domain.TemplateID { return p.Descriptor().ID }
