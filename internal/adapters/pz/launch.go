package pz

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// LaunchArtifactEvidence records the precondition used to select an executable
// vector. It is populated from a reviewed Target Manifest after inspecting the
// actual installed artifact; template environments are not evidence.
type LaunchArtifactEvidence struct {
	Platform          string
	ExecutableName    string
	FileType          string
	DirectExecutable  bool
	ManifestReference string
}

// LaunchConfig contains only code-controlled launch choices and state-derived
// arguments. Template environments are intentionally not represented.
type LaunchConfig struct {
	InstanceID       domain.InstanceID
	InstallDir       string
	CacheDir         string
	Platform         string
	ServerName       string
	AdminPass        string
	ArtifactEvidence LaunchArtifactEvidence
}

// BuildLaunchSpec creates the direct executable + typed argv. The admin
// password travels as two argv tokens (-adminpassword <value>) because PZ
// 42.21 rejects the single-token form; the value is redacted from logs
// to logs by this package. Environment template strings are not accepted.
func BuildLaunchSpec(config LaunchConfig) (ports.LaunchSpec, error) {
	if !instancePattern.MatchString(string(config.InstanceID)) {
		return ports.LaunchSpec{}, errors.New("invalid instance id")
	}
	if strings.TrimSpace(config.InstallDir) == "" || strings.TrimSpace(config.CacheDir) == "" {
		return ports.LaunchSpec{}, errors.New("install and cache directories are required")
	}
	if strings.TrimSpace(config.Platform) == "" {
		return ports.LaunchSpec{}, errors.New("launch platform is required")
	}
	if err := validateArtifactEvidence(config.Platform, config.ArtifactEvidence); err != nil {
		return ports.LaunchSpec{}, err
	}
	if !serverNamePattern.MatchString(config.ServerName) {
		return ports.LaunchSpec{}, errors.New("invalid server name")
	}
	if config.AdminPass == "" {
		return ports.LaunchSpec{}, errors.New("admin password is required")
	}
	if strings.ContainsRune(config.AdminPass, '\x00') {
		return ports.LaunchSpec{}, errors.New("admin password contains an unsupported NUL byte")
	}
	installDir, err := filepath.Abs(config.InstallDir)
	if err != nil {
		return ports.LaunchSpec{}, fmt.Errorf("resolve install directory: %w", err)
	}
	cacheDir, err := filepath.Abs(config.CacheDir)
	if err != nil {
		return ports.LaunchSpec{}, fmt.Errorf("resolve cache directory: %w", err)
	}
	platform := config.Platform
	var executable string
	switch platform {
	case "linux", "darwin":
		// The ADR allows this only after direct-binary type is attested in the
		// Target Manifest; the evidence is validated above. Template metadata is
		// never used to pick this path.
		executable = filepath.Join(installDir, config.ArtifactEvidence.ExecutableName)
	case "windows":
		executable = filepath.Join(installDir, config.ArtifactEvidence.ExecutableName)
	default:
		return ports.LaunchSpec{}, fmt.Errorf("unsupported launch platform %q", platform)
	}
	args := []string{
		"-cachedir=" + cacheDir,
		"-servername=" + config.ServerName,
		// PZ 42.21 accepts the two-token form only: the single-token form is
		// logged as "unknown option" and the server then blocks on an
		// interactive password prompt (verified on the Windows target).
		"-adminpassword", config.AdminPass,
	}
	env := make(map[string]string, 1)
	if platform == "linux" {
		libDir := filepath.Join(installDir, "linux64")
		runtimePath := os.Getenv("LD_LIBRARY_PATH")
		env["LD_LIBRARY_PATH"] = libDir + string(filepath.ListSeparator) + installDir
		if runtimePath != "" {
			env["LD_LIBRARY_PATH"] += string(filepath.ListSeparator) + runtimePath
		}
	}
	return ports.LaunchSpec{Executable: executable, Args: args, WorkDir: installDir, Env: env}, nil
}

func validateArtifactEvidence(platform string, evidence LaunchArtifactEvidence) error {
	if evidence.Platform != platform || strings.TrimSpace(evidence.ManifestReference) == "" || strings.TrimSpace(evidence.FileType) == "" {
		return errors.New("launch artifact evidence must include the selected platform, file type, and manifest reference")
	}
	if !evidence.DirectExecutable {
		return errors.New("launch artifact is not attested as directly executable; interpreter vectors are disabled")
	}
	want := ""
	switch platform {
	case "linux", "darwin":
		want = "ProjectZomboid64"
	case "windows":
		want = "ProjectZomboid64.exe"
	default:
		return fmt.Errorf("unsupported launch platform %q", platform)
	}
	if evidence.ExecutableName != want {
		return fmt.Errorf("artifact executable %q is not the code-approved direct vector for %s", evidence.ExecutableName, platform)
	}
	return nil
}

// ReadinessMode selects only the approved A2S oracle or a manifest-recorded
// log marker fallback. Template start arguments are deliberately unavailable.
type ReadinessMode string

const (
	ReadinessA2S       ReadinessMode = "a2s"
	ReadinessLogMarker ReadinessMode = "log-marker"
)

// ReadinessProbeFunc is injected in tests and composition; it need not use the
// network (local synthetic probes are encouraged). Marker is only meaningful
// for a Target-Manifest-recorded log fallback.
type ReadinessProbeFunc func(context.Context, ReadinessMode, string, int, string) (bool, error)

const DefaultReadinessPortKey = "SERVER_PORT"

// Readiness implements ports.ReadinessProbe with an injected A2S/log oracle,
// timeout, and interval. Port selection uses the code-owned SERVER_PORT
// contract, never the template start_arguments string.
type Readiness struct {
	Ports    map[string]int
	PortKey  string
	Port     int // optional preselected port; when zero, selected from Ports
	Host     string
	Mode     ReadinessMode
	Marker   string
	Timeout  time.Duration
	Interval time.Duration
	Probe    ReadinessProbeFunc
	Alive    func(context.Context, domain.InstanceID) (bool, error)
}

// Await returns true after a successful probe while the process remains alive.
// A2S defaults to the approved 60-second deadline. Timeout does not stop process.
// Ready implements ports.ReadinessProbe.
func (r Readiness) Ready(ctx context.Context, instance domain.InstanceID) (bool, error) {
	return r.Await(ctx, instance)
}

// Await returns the readiness oracle result; it is also exposed as a descriptive
// method for tests and call sites which want to distinguish polling from Ready.
func (r Readiness) Await(ctx context.Context, instance domain.InstanceID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	port := r.Port
	if port == 0 {
		key := r.PortKey
		if key == "" {
			key = DefaultReadinessPortKey
		}
		port = r.Ports[key]
	}
	if port < 1 || port > 65535 {
		return false, errors.New("readiness port must be between 1 and 65535")
	}
	if r.Probe == nil {
		return false, errors.New("readiness probe is required")
	}
	if r.Alive == nil {
		return false, errors.New("process liveness probe is required")
	}
	mode := r.Mode
	if mode == "" {
		mode = ReadinessA2S
	}
	if mode != ReadinessA2S && mode != ReadinessLogMarker {
		return false, errors.New("readiness mode must be A2S or a manifest-recorded log marker")
	}
	if mode == ReadinessLogMarker && strings.TrimSpace(r.Marker) == "" {
		return false, errors.New("readiness log marker is required")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	interval := r.Interval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	host := r.Host
	if host == "" {
		host = "127.0.0.1"
	}
	for {
		ready, err := r.Probe(deadlineCtx, mode, host, port, r.Marker)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			return false, err
		}
		if ready {
			alive, aliveErr := r.Alive(deadlineCtx, instance)
			if aliveErr != nil {
				return false, aliveErr
			}
			if !alive {
				return false, nil
			}
			return true, nil
		}
		select {
		case <-deadlineCtx.Done():
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return false, nil
		case <-ticker.C:
		}
	}
}

// NetworkProbe adapts A2SInfoProbe to ReadinessProbeFunc. Log-marker mode is
// intentionally not implemented here; it requires a build-specific adapter
// supplied from recorded Target Manifest evidence.
func NetworkProbe(ctx context.Context, mode ReadinessMode, host string, port int, marker string) (bool, error) {
	if mode == ReadinessLogMarker {
		return false, errors.New("log-marker readiness requires an injected manifest-specific probe")
	}
	return A2SInfoProbe(ctx, host, port)
}

// A2SInfoProbe is a minimal UDP Source Engine query implementation. It sends
// an A2S_INFO request and considers a response valid only when it starts with
// the expected response byte. It is not used by unit tests or automatically
// invoked; tests inject synthetic probes instead.
func A2SInfoProbe(ctx context.Context, host string, port int) (bool, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if port < 1 || port > 65535 {
		return false, errors.New("invalid A2S port")
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	request := []byte{0xff, 0xff, 0xff, 0xff, 0x54, 'S', 'o', 'u', 'r', 'c', 'e', ' ', 'E', 'n', 'g', 'i', 'n', 'e', ' ', 'Q', 'u', 'e', 'r', 'y', 0x00}
	if _, err := conn.Write(request); err != nil {
		return false, err
	}
	response := make([]byte, 1400)
	n, err := conn.Read(response)
	if err != nil {
		return false, err
	}
	return n >= 5 && response[4] == 0x49, nil
}
