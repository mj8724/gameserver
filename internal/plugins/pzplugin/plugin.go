// Package pzplugin registers Project Zomboid as the first game plugin. It only
// wires existing adapters behind the plugin contract; behaviour is unchanged.
package pzplugin

import (
	"fmt"

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

// GameConfig builds the configuration adapter for one instance.
func (p *Plugin) GameConfig(deps ports.PluginDeps) (ports.GameConfig, error) {
	_ = deps
	return nil, fmt.Errorf("pzplugin.GameConfig requires the composition-site factory")
}

// LaunchSpec returns nil: the composition root supplies the typed builder
// because it owns launch evidence (vector, manifest reference, memory).
func (p *Plugin) LaunchSpec(ports.PluginDeps) (ports.LaunchSpecBuilder, error) {
	return nil, fmt.Errorf("pzplugin.LaunchSpec requires the composition-site builder")
}

// Readiness builds the log-marker oracle declared in the descriptor.
func (p *Plugin) Readiness(deps ports.PluginDeps) (ports.ReadinessProbe, error) {
	marker := p.Descriptor().Readiness.Marker
	return ports.ReadinessProbe(nil), fmt.Errorf("pzplugin.Readiness(%s) requires an injected log source", marker)
}

// Name reports the game id for logs.
func (p *Plugin) Name() string { return string(p.Descriptor().ID) }

// TemplateID is the template this plugin serves.
func (p *Plugin) TemplateID() domain.TemplateID { return p.Descriptor().ID }
