// Package valheimplugin registers Valheim as the second game plugin (M4.3).
// The launch vector is a direct executable with typed argv and a stdout log
// marker as the readiness oracle; both were established by a read-only probe
// of the vendor artifacts on the target host (see the M4 evidence file).
package valheimplugin

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// SteamAppIDEnv is the game id the vendor script exports; the dedicated server
// app id (896660) is only the install identity.
const SteamAppIDEnv = "892970"

// ServerExecutable is the vendor executable name inside the install directory.
const ServerExecutable = "valheim_server.exe"

// ReadinessMarker is the stdout line Valheim prints once the world is live.
const ReadinessMarker = "Game server connected"

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
			{Label: "public", Branch: "public", EvidenceRef: "docs/acceptance/evidence/M4-valheim-probe.md"},
		},
		Ports: []ports.PortSpec{
			{Key: "SERVER_PORT", Label: "游戏端口", DefaultPort: 2456, Protocol: "udp", Query: true},
			{Key: "DIRECT_PORT", Label: "查询端口", DefaultPort: 2457, Protocol: "udp"},
		},
		Readiness: ports.ReadinessSpec{
			Mode:            "log-marker",
			PortKey:         "SERVER_PORT",
			Marker:          ReadinessMarker,
			DefaultTimeoutS: 180,
		},
		ConfigTargets:  []string{"argv"},
		InstallMarkers: []string{ServerExecutable},
	}
}

// InstallerSource returns the Steam identity and installable versions.
func (p *Plugin) InstallerSource() (string, []ports.VersionSpec) {
	descriptor := p.Descriptor()
	return descriptor.SteamAppID, descriptor.Versions
}

// GameConfig is a no-op for Valheim: the dedicated server is configured through
// argv (and optional BepInEx files), not through a managed INI/SandboxVars.
func (p *Plugin) GameConfig(ports.PluginDeps) (ports.GameConfig, error) {
	return nil, nil
}

// LaunchSpec builds the vendor argv. Every value is a separate argv element, so
// quotes and shell syntax in a server name or password stay inert.
func (p *Plugin) LaunchSpec(ports.PluginDeps) (ports.LaunchSpecBuilder, error) {
	return func(_ context.Context, state domain.InstanceState, input ports.LaunchInput) (ports.LaunchSpec, error) {
		if strings.TrimSpace(input.InstallDir) == "" {
			return ports.LaunchSpec{}, errors.New("valheim launch requires an install directory")
		}
		// The process adapter resolves a bare name through PATH, so the spec
		// carries the fully qualified path inside the instance install directory.
		exe := filepath.Join(input.InstallDir, ServerExecutable)
		name := firstNonEmpty(input.ServerName, "gameserver")
		password := firstNonEmpty(input.AdminPass, "")
		if len(password) < 5 {
			// The vendor script documents the minimum; failing closed beats a
			// server that silently refuses to start.
			return ports.LaunchSpec{}, fmt.Errorf("valheim requires a password of at least 5 characters (got %d)", len(password))
		}
		port := 0
		if state.Ports != nil {
			port = state.Ports["SERVER_PORT"]
		}
		if port == 0 {
			port = 2456
		}
		args := []string{
			"-nographics",
			"-batchmode",
			"-name", name,
			"-port", strconv.Itoa(port),
			"-world", name,
			"-password", password,
			"-public", "0",
		}
		return ports.LaunchSpec{
			Executable: exe,
			Args:       args,
			WorkDir:    input.InstallDir,
			Env:        map[string]string{"SteamAppId": SteamAppIDEnv},
		}, nil
	}, nil
}

// Readiness reports the descriptor marker: the same log-marker rule as PZ, with
// the window recorded in the Target Manifest (default 180s for world gen).
func (p *Plugin) Readiness(ports.PluginDeps) (ports.ReadinessProbe, error) {
	return nil, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
