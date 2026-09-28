// Command gameserver is the Go control plane: it wires the application use
// cases to the outbound adapters and serves the HTTP/WebSocket transport.
// Subcommands cover the operator-only migration actions.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/mj8724/gameserver/internal/adapters/httpapi"
	"github.com/mj8724/gameserver/internal/adapters/instancefiles"
	"github.com/mj8724/gameserver/internal/adapters/localstate"
	"github.com/mj8724/gameserver/internal/adapters/migrate"
	"github.com/mj8724/gameserver/internal/adapters/oslock"
	"github.com/mj8724/gameserver/internal/adapters/process"
	"github.com/mj8724/gameserver/internal/adapters/pz"
	"github.com/mj8724/gameserver/internal/adapters/pztemplate"
	"github.com/mj8724/gameserver/internal/adapters/staticassets"
	"github.com/mj8724/gameserver/internal/adapters/steamcmd"
	"github.com/mj8724/gameserver/internal/adapters/systemclock"
	"github.com/mj8724/gameserver/internal/application"
	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
	"github.com/mj8724/gameserver/internal/version"
)

const (
	defaultInstance = "pz_01"
	shutdownGrace   = 15 * time.Second
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "recover":
			runRecover(os.Args[2:])
			return
		case "promote":
			runPromote(os.Args[2:])
			return
		case "backup":
			runBackup(os.Args[2:])
			return
		case "restore":
			runRestore(os.Args[2:])
			return
		case "export-back":
			runExportBack(os.Args[2:])
			return
		case "fix-permissions":
			runFixPermissions(os.Args[2:])
			return
		case "version", "--version", "-v":
			fmt.Println(version.String())
			return
		}
	}
	if err := serve(); err != nil {
		log.Fatalf("gameserver: %v", err)
	}
}

type runtimeConfig struct {
	Host         string
	Port         int
	DataRoot     string
	StaticDir    string
	TemplatesDir string
	Instance     domain.InstanceID
	AdminPass    string
	SecureCookie bool

	LaunchExecutable  string
	LaunchDirectExec  bool
	LaunchEvidenceRef string
}

func loadConfig() runtimeConfig {
	cfg := runtimeConfig{
		Host:         env("GAMESERVER_HOST", "127.0.0.1"),
		Port:         envInt("GAMESERVER_PORT", 8769),
		DataRoot:     env("GAMESERVER_DATA_ROOT", "data"),
		StaticDir:    env("GAMESERVER_STATIC_DIR", "static"),
		TemplatesDir: env("GAMESERVER_TEMPLATES_DIR", "templates"),
		Instance:     domain.InstanceID(env("GAMESERVER_INSTANCE", defaultInstance)),
		AdminPass:    os.Getenv("GAMESERVER_ADMIN_PASSWORD"),
		SecureCookie: os.Getenv("GAMESERVER_COOKIE_SECURE") == "1",

		LaunchExecutable:  os.Getenv("GAMESERVER_LAUNCH_EXECUTABLE"),
		LaunchDirectExec:  os.Getenv("GAMESERVER_LAUNCH_DIRECT_EXEC") == "1",
		LaunchEvidenceRef: os.Getenv("GAMESERVER_LAUNCH_EVIDENCE_REF"),
	}
	return cfg
}

func serve() error {
	cfg := loadConfig()
	rt, err := buildRuntime(cfg)
	if err != nil {
		return err
	}
	return rt.serveHTTP(cfg)
}

// runtime carries the wired component graph so integration tests can exercise
// the real adapters without binding a port.
type appRuntime struct {
	handler     *httpapi.Server
	supervisor  ports.ProcessSupervisor
	instance    domain.InstanceID
	serversRoot string
	serviceID   string
}

// buildRuntime wires adapters into the application services (ADR §2.1: cmd is
// the only composition root).
func buildRuntime(cfg runtimeConfig) (*appRuntime, error) {
	absoluteData, err := filepath.Abs(cfg.DataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve data root: %w", err)
	}
	serversRoot := filepath.Join(absoluteData, "servers")
	instance := cfg.Instance

	defaults, err := localstate.DefaultState(instance)
	if err != nil {
		return nil, fmt.Errorf("prepare default state: %w", err)
	}
	states, err := localstate.NewStore(absoluteData, defaults)
	if err != nil {
		return nil, fmt.Errorf("state store: %w", err)
	}
	files, err := instancefiles.New(serversRoot, runtime.GOOS)
	if err != nil {
		return nil, fmt.Errorf("instance files: %w", err)
	}
	templates, err := pztemplate.New(cfg.TemplatesDir)
	if err != nil {
		return nil, fmt.Errorf("template catalog: %w", err)
	}
	for _, warning := range templates.Warnings() {
		log.Printf("template warning: %s", warning)
	}
	gameConfig, err := pz.NewConfig(absoluteData, func(ctx context.Context, id domain.InstanceID) (string, error) {
		state, err := states.Load(ctx, id)
		if err != nil {
			return "", err
		}
		name, _ := state.Variables["SERVER_NAME"].(string)
		if name == "" {
			return "servertest", nil
		}
		return name, nil
	})
	if err != nil {
		return nil, fmt.Errorf("game config adapter: %w", err)
	}
	static, err := staticassets.New(cfg.StaticDir)
	if err != nil {
		return nil, fmt.Errorf("static assets: %w", err)
	}
	supervisor := process.NewForInstance(instance)
	clock := systemclock.New()
	serviceID, err := os.Hostname()
	if err != nil || serviceID == "" {
		serviceID = "gameserver"
	}
	serviceID = fmt.Sprintf("%s-%d", serviceID, os.Getpid())

	control, err := application.NewControlService(application.ServiceDeps{
		Instance:      instance,
		Platform:      runtime.GOOS,
		States:        states,
		Config:        gameConfig,
		Installer:     steamcmd.New(),
		Processes:     supervisor,
		ProcessStatus: supervisor,
		Logs:          supervisor,
		Files:         files,
		Clock:         clock,
		Locks:         &oslock.Locks{ServersRoot: serversRoot, ServiceID: serviceID},
		Templates:     templates,
		BuildLaunchSpec: func(ctx context.Context, state domain.InstanceState, input ports.LaunchInput) (ports.LaunchSpec, error) {
			return pz.BuildLaunchSpec(pz.LaunchConfig{
				InstanceID: instance,
				InstallDir: input.InstallDir,
				CacheDir:   input.CacheDir,
				Platform:   input.Platform,
				ServerName: input.ServerName,
				AdminPass:  input.AdminPass,
				ArtifactEvidence: pz.LaunchArtifactEvidence{
					Platform:          input.Platform,
					ExecutableName:    firstNonEmpty(input.ExecutableName, cfg.LaunchExecutable),
					FileType:          "reviewed-artifact",
					DirectExecutable:  input.DirectExecutable || cfg.LaunchDirectExec,
					ManifestReference: firstNonEmpty(input.EvidenceRef, cfg.LaunchEvidenceRef),
				},
			})
		},
		ApplyGameConfig: func(ctx context.Context, state domain.InstanceState) error {
			name, _ := state.Variables["SERVER_NAME"].(string)
			if name == "" {
				name = "servertest"
			}
			return gameConfig.ApplyNamed(ctx, state.ID, name, pz.ManagedINIUpdates(state.Variables, state.Ports, state.Mods))
		},
		LaunchEvidence: application.LaunchEvidence{
			ExecutableName:   cfg.LaunchExecutable,
			DirectExecutable: cfg.LaunchDirectExec,
			Reference:        cfg.LaunchEvidenceRef,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("control service: %w", err)
	}

	authenticator, err := application.NewAuthenticator(application.AuthConfig{AdminPassword: cfg.AdminPass}, clock)
	if err != nil {
		return nil, fmt.Errorf("authenticator: %w", err)
	}
	handler, err := httpapi.NewServer(control, authenticator, static, httpapi.Config{SecureCookies: cfg.SecureCookie})
	if err != nil {
		return nil, fmt.Errorf("http server: %w", err)
	}

	return &appRuntime{
		handler:     handler,
		supervisor:  supervisor,
		instance:    instance,
		serversRoot: serversRoot,
		serviceID:   serviceID,
	}, nil
}

// serveHTTP runs the wired graph until a signal or a fatal serve error.
func (r *appRuntime) serveHTTP(cfg runtimeConfig) error {
	handler := r.handler
	supervisor := r.supervisor
	instance := r.instance
	serversRoot := r.serversRoot
	serviceID := r.serviceID

	if lockManager, err := oslock.NewManager(serversRoot, serviceID); err == nil {
		inspection, inspectErr := lockManager.Inspect(instance)
		switch {
		case inspectErr != nil:
			log.Printf("ownership inspection failed: %v", inspectErr)
		case inspection.Record != nil:
			log.Printf("WARNING: previous ownership record present (pid alive=%t); mutating operations will be refused until `gameserver recover` is run", inspection.PIDAlive)
		case len(inspection.Candidates) > 0:
			log.Printf("WARNING: %d residual process(es) reference the instance root; mutating operations will be refused", len(inspection.Candidates))
		}
	}
	if migrator, err := migrate.New(migrate.Layout{ServersRoot: serversRoot, Instance: instance}); err == nil {
		if snapshot, snapshotErr := migrator.Inspect(); snapshotErr == nil && snapshot.Blocked() {
			log.Printf("WARNING: migration journal is in %s; start/install stay blocked until recovery", snapshot.Journal.State)
		}
	}
	if cfg.AdminPass == "" {
		log.Printf("WARNING: GAMESERVER_ADMIN_PASSWORD is not set; login and protected operations return 503")
	}

	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port)),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", server.Addr, err)
	}
	log.Printf("gameserver %s listening on http://%s (data root: %s, platform: %s)", version.String(), listener.Addr(), serversRoot, runtime.GOOS)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		handler.Close()
		return err
	case <-ctx.Done():
		log.Printf("shutdown requested; draining")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	handler.Close()
	if stopErr := supervisor.Stop(shutdownCtx, instance); stopErr != nil {
		_ = supervisor.Kill(shutdownCtx, instance)
	}
	return shutdownErr
}

func runRecover(args []string) {
	flags := flag.NewFlagSet("recover", flag.ExitOnError)
	dataRoot := flags.String("data-root", "data", "data root containing servers/")
	instance := flags.String("instance", defaultInstance, "instance id")
	force := flags.Bool("force", false, "clear ownership even if the recorded process is alive")
	_ = flags.Parse(args)

	manager, err := oslock.NewManager(filepath.Join(*dataRoot, "servers"), "gameserver-recover")
	fail(err)
	result, err := manager.Recover(domain.InstanceID(*instance), *force)
	if err != nil {
		fail(fmt.Errorf("recover refused: %w", err))
	}
	fmt.Printf("recover: action=%s alive=%t\n", result.Action, result.PIDAlive)
}

func runPromote(args []string) {
	flags := flag.NewFlagSet("promote", flag.ExitOnError)
	dataRoot := flags.String("data-root", "data", "data root containing servers/")
	instance := flags.String("instance", defaultInstance, "instance id")
	source := flags.String("source", "", "directory holding converted state content")
	_ = flags.Parse(args)
	if *source == "" {
		fail(errors.New("--source is required"))
	}

	tool, err := migrate.New(migrate.Layout{ServersRoot: filepath.Join(*dataRoot, "servers"), Instance: domain.InstanceID(*instance)})
	fail(err)
	manifest, err := tool.Import(*source)
	if err != nil {
		fail(fmt.Errorf("import: %w", err))
	}
	fmt.Printf("import: files=%d checksum=%s\n", len(manifest.Files), manifest.Checksum)
	result, err := tool.Promote()
	if err != nil {
		fail(fmt.Errorf("promote: %w", err))
	}
	fmt.Printf("promote: action=%s state=%s\n", result.Action, result.Journal.State)
}

func runBackup(args []string) {
	flags := flag.NewFlagSet("backup", flag.ExitOnError)
	dataRoot := flags.String("data-root", "data", "data root containing servers/")
	instance := flags.String("instance", defaultInstance, "instance id")
	destination := flags.String("dest", "", "backup destination root")
	_ = flags.Parse(args)
	if *destination == "" {
		fail(errors.New("--dest is required"))
	}

	tool, err := migrate.New(migrate.Layout{ServersRoot: filepath.Join(*dataRoot, "servers"), Instance: domain.InstanceID(*instance)})
	fail(err)
	record, err := tool.Backup(*destination)
	if err != nil {
		fail(fmt.Errorf("backup: %w", err))
	}
	fmt.Printf("backup: path=%s files=%d checksum=%s\n", record.Path, len(record.Files), record.Checksum)
}

// runRestore verifies a backup and stages its state content, leaving the
// actual switch to the journaled promotion path.
func runRestore(args []string) {
	flags := flag.NewFlagSet("restore", flag.ExitOnError)
	dataRoot := flags.String("data-root", "data", "data root containing servers/")
	instance := flags.String("instance", defaultInstance, "instance id")
	backup := flags.String("backup", "", "backup directory produced by the backup command")
	_ = flags.Parse(args)
	if *backup == "" {
		fail(errors.New("--backup is required"))
	}

	tool, err := migrate.New(migrate.Layout{ServersRoot: filepath.Join(*dataRoot, "servers"), Instance: domain.InstanceID(*instance)})
	fail(err)
	manifest, err := tool.RestoreBackup(*backup)
	if err != nil {
		fail(fmt.Errorf("restore: %w", err))
	}
	fmt.Printf("restore: staged %d file(s) checksum=%s\n", len(manifest.Files), manifest.Checksum)
	result, err := tool.Promote()
	if err != nil {
		fail(fmt.Errorf("promote after restore: %w", err))
	}
	fmt.Printf("restore: promote action=%s state=%s\n", result.Action, result.Journal.State)
}

// runExportBack writes Go state back into the legacy Python-compatible
// instance.json so the Python baseline can resume after a rollback. It never
// deletes the Go state file.
func runExportBack(args []string) {
	flags := flag.NewFlagSet("export-back", flag.ExitOnError)
	dataRoot := flags.String("data-root", "data", "data root containing servers/")
	instance := flags.String("instance", defaultInstance, "instance id")
	_ = flags.Parse(args)

	absolute, err := filepath.Abs(*dataRoot)
	fail(err)
	id := domain.InstanceID(*instance)
	defaults, err := localstate.DefaultState(id)
	fail(err)
	states, err := localstate.NewStore(absolute, defaults)
	fail(err)
	state, err := states.Load(context.Background(), id)
	fail(err)

	document := map[string]any{
		"instance_id": string(state.ID),
		"name":        state.Name,
		"template_id": state.TemplateID,
		"variables":   state.Variables,
		"ports":       state.Ports,
	}
	if state.Mods != nil {
		document["mods"] = state.Mods
	}
	if state.Billing != nil {
		document["billing"] = state.Billing
	}
	if state.QuotaGB > 0 {
		document["quota_gb"] = state.QuotaGB
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	fail(err)
	legacy := filepath.Join(absolute, "servers", string(id), "instance.json")
	fail(os.MkdirAll(filepath.Dir(legacy), 0o700))
	fail(os.WriteFile(legacy, append(encoded, '\n'), 0o600))
	fmt.Printf("export-back: wrote %s (fields: %d)\n", legacy, len(document))
}

// runFixPermissions normalizes the ADR §1.7 modes for an existing data root.
// It never happens implicitly during normal service operation, which is why
// this is an explicit operator command with an audit summary.
func runFixPermissions(args []string) {
	flags := flag.NewFlagSet("fix-permissions", flag.ExitOnError)
	dataRoot := flags.String("data-root", "data", "data root containing servers/")
	instance := flags.String("instance", defaultInstance, "instance id")
	_ = flags.Parse(args)

	if runtime.GOOS == "windows" {
		fmt.Println("fix-permissions: POSIX modes do not apply on windows; nothing changed")
		return
	}
	changed, err := normalizePermissions(*dataRoot, domain.InstanceID(*instance))
	fail(err)
	fmt.Printf("fix-permissions: %d path(s) updated\n", changed)
}

// normalizePermissions applies the ADR §1.7 modes to an existing data root and
// returns how many paths actually changed.
func normalizePermissions(dataRoot string, instance domain.InstanceID) (int, error) {
	absolute, err := filepath.Abs(dataRoot)
	if err != nil {
		return 0, err
	}
	servers := filepath.Join(absolute, "servers")
	root := filepath.Join(servers, string(instance))
	changed := 0

	fixDir := func(path string) {
		if info, err := os.Stat(path); err == nil && info.IsDir() && info.Mode().Perm() != 0o700 {
			if os.Chmod(path, 0o700) == nil {
				changed++
				log.Printf("chmod 0700 %s", path)
			}
		}
	}
	fixFile := func(path string) {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm() != 0o600 {
			if os.Chmod(path, 0o600) == nil {
				changed++
				log.Printf("chmod 0600 %s", path)
			}
		}
	}

	for _, dir := range []string{servers, filepath.Join(servers, ".locks"), filepath.Join(servers, ".owners"), root,
		filepath.Join(root, "state"), filepath.Join(root, "Zomboid"), filepath.Join(root, "Zomboid", "Server")} {
		fixDir(dir)
	}
	for _, file := range []string{filepath.Join(root, "instance.json"), filepath.Join(root, "state", "instance.json")} {
		fixFile(file)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "Zomboid", "Server")); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				fixFile(filepath.Join(root, "Zomboid", "Server", entry.Name()))
			}
		}
	}
	return changed, nil
}

func fail(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "gameserver: %v\n", err)
	os.Exit(1)
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed := 0
	for _, character := range value {
		if character < '0' || character > '9' {
			return fallback
		}
		parsed = parsed*10 + int(character-'0')
	}
	if parsed == 0 || parsed > 65535 {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
