// Command gameserver is the Go control plane: it wires the application use
// cases to the outbound adapters and serves the HTTP/WebSocket transport.
// Subcommands cover the operator-only migration actions.
package main

import (
	"context"
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
	absoluteData, err := filepath.Abs(cfg.DataRoot)
	if err != nil {
		return fmt.Errorf("resolve data root: %w", err)
	}
	serversRoot := filepath.Join(absoluteData, "servers")
	instance := cfg.Instance

	defaults, err := localstate.DefaultState(instance)
	if err != nil {
		return fmt.Errorf("prepare default state: %w", err)
	}
	states, err := localstate.NewStore(absoluteData, defaults)
	if err != nil {
		return fmt.Errorf("state store: %w", err)
	}
	files, err := instancefiles.New(serversRoot, runtime.GOOS)
	if err != nil {
		return fmt.Errorf("instance files: %w", err)
	}
	templates, err := pztemplate.New(cfg.TemplatesDir)
	if err != nil {
		return fmt.Errorf("template catalog: %w", err)
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
		return fmt.Errorf("game config adapter: %w", err)
	}
	static, err := staticassets.New(cfg.StaticDir)
	if err != nil {
		return fmt.Errorf("static assets: %w", err)
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
		return fmt.Errorf("control service: %w", err)
	}

	authenticator, err := application.NewAuthenticator(application.AuthConfig{AdminPassword: cfg.AdminPass}, clock)
	if err != nil {
		return fmt.Errorf("authenticator: %w", err)
	}
	handler, err := httpapi.NewServer(control, authenticator, static, httpapi.Config{SecureCookies: cfg.SecureCookie})
	if err != nil {
		return fmt.Errorf("http server: %w", err)
	}

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
