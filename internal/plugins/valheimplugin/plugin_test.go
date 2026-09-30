package valheimplugin

import (
	"context"
	"strings"
	"testing"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

func build(t *testing.T, dir, name, password string, port int) (ports.LaunchSpec, error) {
	t.Helper()
	builder, err := New().LaunchSpec(ports.PluginDeps{})
	if err != nil {
		t.Fatalf("LaunchSpec: %v", err)
	}
	if builder == nil {
		t.Fatal("valheim must own its launch argv")
	}
	return builder(context.Background(), domain.InstanceState{
		ID:    "valheim_01",
		Ports: map[string]int{"SERVER_PORT": port},
	}, ports.LaunchInput{InstallDir: dir, ServerName: name, AdminPass: password})
}

// The vendor argv shape is reproduced with a direct executable vector: no
// launcher descriptor, no shell, and the game id exported as SteamAppId.
func TestValheimLaunchSpecUsesVendorArgv(t *testing.T) {
	spec, err := build(t, "/games/valheim", "My server", "secret1", 2456)
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	if spec.Executable != ServerExecutable {
		t.Fatalf("executable = %q", spec.Executable)
	}
	if spec.WorkDir != "/games/valheim" {
		t.Fatalf("workdir = %q", spec.WorkDir)
	}
	if spec.Env["SteamAppId"] != SteamAppIDEnv {
		t.Fatalf("SteamAppId env missing: %+v", spec.Env)
	}
	joined := strings.Join(spec.Args, " ")
	for _, want := range []string{"-nographics", "-batchmode", "-name My server", "-port 2456", "-world My server", "-password secret1", "-public 0"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("argv missing %q: %v", want, spec.Args)
		}
	}
}

// A name with shell syntax stays one inert argv element.
func TestValheimLaunchSpecKeepsValuesInert(t *testing.T) {
	spec, err := build(t, "/games/valheim", `evil " & rm -rf /`, "secret1", 2456)
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	found := false
	for _, arg := range spec.Args {
		if arg == `evil " & rm -rf /` {
			found = true
		}
	}
	if !found {
		t.Fatalf("server name must stay a single argv element: %v", spec.Args)
	}
	if strings.Contains(strings.Join(spec.Args, " "), "cmd.exe") {
		t.Fatal("no shell interpreter may appear in the argv")
	}
}

// The vendor minimum password length fails closed instead of starting a server
// that silently refuses to run.
func TestValheimLaunchSpecRejectsShortPassword(t *testing.T) {
	if _, err := build(t, "/games/valheim", "srv", "1234", 2456); err == nil {
		t.Fatal("short password must be rejected")
	}
}

// Ports come from the instance state so multi-instance allocation is honoured.
func TestValheimLaunchSpecUsesStatePort(t *testing.T) {
	spec, err := build(t, "/games/valheim", "srv", "secret1", 3456)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(spec.Args, " "), "-port 3456") {
		t.Fatalf("state port not honoured: %v", spec.Args)
	}
}

// Readiness is the stdout marker recorded by the probe.
func TestValheimDescriptorReadiness(t *testing.T) {
	spec := New().Descriptor().Readiness
	if spec.Mode != "log-marker" || spec.Marker != ReadinessMarker {
		t.Fatalf("unexpected readiness spec: %+v", spec)
	}
	if spec.DefaultTimeoutS < 120 {
		t.Fatalf("world generation needs a wide window, got %ds", spec.DefaultTimeoutS)
	}
}
