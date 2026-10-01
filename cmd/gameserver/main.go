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
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mj8724/gameserver/internal/adapters/auditlog"
	"github.com/mj8724/gameserver/internal/adapters/httpapi"
	"github.com/mj8724/gameserver/internal/adapters/instancefiles"
	"github.com/mj8724/gameserver/internal/adapters/instanceregistry"
	"github.com/mj8724/gameserver/internal/adapters/localstate"
	"github.com/mj8724/gameserver/internal/adapters/migrate"
	"github.com/mj8724/gameserver/internal/adapters/nodestate"
	"github.com/mj8724/gameserver/internal/adapters/optioncatalog"
	"github.com/mj8724/gameserver/internal/adapters/oslock"
	"github.com/mj8724/gameserver/internal/adapters/process"
	"github.com/mj8724/gameserver/internal/adapters/pz"
	"github.com/mj8724/gameserver/internal/adapters/pztemplate"
	"github.com/mj8724/gameserver/internal/adapters/staticassets"
	"github.com/mj8724/gameserver/internal/adapters/steamcmd"
	"github.com/mj8724/gameserver/internal/adapters/systemclock"
	"github.com/mj8724/gameserver/internal/application"
	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/plugins"
	"github.com/mj8724/gameserver/internal/plugins/pzplugin"
	"github.com/mj8724/gameserver/internal/plugins/valheimplugin"
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

	SteamCMDExecutable string
	SteamCMDDir        string
	SteamAppID         string

	ReadinessMarker     string
	ReadinessPort       int
	CapacitySoftPercent float64
	CapacityHardPercent float64
	// ReadinessTimeout bounds the log-marker oracle when a Manifest records a
	// window other than the 60-second default (PZ cold starts exceed it).
	ReadinessTimeout time.Duration

	CatalogsDir string
	// PZHome points at the game's own configuration directory when the game
	// keeps it outside the instance tree (PZ on Windows uses the user profile
	// and ignores -cachedir for configuration).
	PZHome string

	LaunchVector   string
	ServerMemoryMB int
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

		SteamCMDExecutable: os.Getenv("GAMESERVER_STEAMCMD_EXECUTABLE"),
		SteamCMDDir:        os.Getenv("GAMESERVER_STEAMCMD_DIR"),
		SteamAppID:         os.Getenv("GAMESERVER_STEAM_APP_ID"),

		ReadinessMarker:     os.Getenv("GAMESERVER_READINESS_MARKER"),
		ReadinessPort:       envInt("GAMESERVER_READINESS_PORT", 0),
		CapacitySoftPercent: envFloat("GAMESERVER_CAPACITY_SOFT_PERCENT", 0),
		CapacityHardPercent: envFloat("GAMESERVER_CAPACITY_HARD_PERCENT", 0),
		ReadinessTimeout: readinessTimeout(
			envInt("GAMESERVER_READINESS_TIMEOUT", 0),
		),

		CatalogsDir: os.Getenv("GAMESERVER_CATALOGS_DIR"),
		PZHome:      os.Getenv("GAMESERVER_PZ_HOME"),

		LaunchVector:   os.Getenv("GAMESERVER_LAUNCH_VECTOR"),
		ServerMemoryMB: envInt("GAMESERVER_SERVER_MEMORY_MB", 0),
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

// runtimeOverrides lets integration tests replace individual outbound adapters
// while keeping the real wiring for everything else (transport, state store,
// PZ INI adapter, templates, lock manager).
type runtimeOverrides struct {
	Installer     ports.Installer
	Processes     ports.ProcessSupervisor
	ProcessStatus ports.ProcessStatusProvider
	Readiness     ports.ReadinessProbe
	Logs          ports.LogSource
}

func buildRuntime(cfg runtimeConfig) (*appRuntime, error) {
	return buildRuntimeWith(cfg, runtimeOverrides{})
}

// buildRuntimeWith wires adapters into the application services (ADR §2.1: cmd
// is the only composition root).
func buildRuntimeWith(cfg runtimeConfig, overrides runtimeOverrides) (*appRuntime, error) {
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
	intentLog, err := localstate.NewIntentLog(absoluteData)
	if err != nil {
		return nil, fmt.Errorf("intent log: %w", err)
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
	instanceRegistry, registryErr := instanceregistry.New(absoluteData)
	if registryErr != nil {
		return nil, fmt.Errorf("instance registry: %w", registryErr)
	}
	nodeStore, nodeErr := nodestate.New(filepath.Join(absoluteData, "nodes"))
	if nodeErr != nil {
		return nil, fmt.Errorf("node store: %w", nodeErr)
	}
	auditLog, auditErr := auditlog.New(absoluteData)
	if auditErr != nil {
		return nil, fmt.Errorf("audit log: %w", auditErr)
	}
	gamePlugins := plugins.New()
	gamePlugins.Register(pzplugin.New())
	gamePlugins.Register(valheimplugin.New())
	if state, err := states.Load(context.Background(), instance); err == nil {
		if plugin, ok := gamePlugins.Lookup(domain.TemplateID(state.TemplateID)); ok {
			descriptor := plugin.Descriptor()
			log.Printf("game plugin %s (%s, app %s, %d port(s))", descriptor.ID, descriptor.Name, descriptor.SteamAppID, len(descriptor.Ports))
			if len(descriptor.InstallMarkers) > 0 {
				files.SetInstallMarkers(descriptor.InstallMarkers...)
			}
		} else {
			log.Printf("WARNING: no game plugin registered for template %q; install/launch stay fail-closed", state.TemplateID)
		}
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
	if home := strings.TrimSpace(cfg.PZHome); home != "" {
		gameConfig.SetHome(home)
		log.Printf("PZ configuration home override: %s", home)
	}
	if dir := strings.TrimSpace(cfg.CatalogsDir); dir != "" {
		seed := filepath.Join(dir, "project_zomboid.SandboxVars.lua")
		if _, err := os.Stat(seed); err == nil {
			gameConfig.SetSandboxSeed(seed)
		}
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

	workshopDownloader := ports.WorkshopDownloader(nil)
	installer, err := resolveInstaller(cfg, serversRoot, instance, states, templates, files, gamePlugins)
	if err != nil {
		return nil, fmt.Errorf("steamcmd installer: %w", err)
	}
	if runner, ok := installer.(ports.WorkshopDownloader); ok {
		workshopDownloader = runner
	}
	processStatus := ports.ProcessStatusProvider(supervisor)
	if overrides.ProcessStatus != nil {
		processStatus = overrides.ProcessStatus
	}
	logs := ports.LogSource(supervisor)
	if overrides.Logs != nil {
		logs = overrides.Logs
	}
	readiness := ports.ReadinessProbe(nil)
	// Readiness defaults come from the resolved game plugin descriptor; an
	// explicit environment marker still wins (Target Manifest override).
	marker := strings.TrimSpace(cfg.ReadinessMarker)
	if marker == "" {
		marker = pluginReadinessMarker(gamePlugins, states, instance)
	}
	if marker != "" {
		readiness = newLogMarkerReadiness(marker, supervisor, cfg.ReadinessTimeout)
	}
	if overrides.Readiness != nil {
		readiness = overrides.Readiness
	}
	var supervisorPort ports.ProcessSupervisor = &process.LoggingSupervisor{Inner: supervisor}
	if overrides.Processes != nil {
		supervisorPort = overrides.Processes
	}
	if overrides.Installer != nil {
		installer = overrides.Installer
	}
	autoBackup := ports.BackupOperator(nil)
	if backupTool, backupErr := migrate.New(migrate.Layout{ServersRoot: serversRoot, Instance: instance}); backupErr == nil {
		backupDir := os.Getenv("GAMESERVER_BACKUP_DIR")
		if backupDir != "" {
			autoBackup = migrate.NewAutoBackup(backupTool, backupDir, os.Getenv("GAMESERVER_AUTO_BACKUP") == "1")
		}
	}
	optionCatalog, catalogErr := loadOptionCatalog(cfg, states, templates)
	if catalogErr != nil {
		log.Printf("WARNING: option catalogue unavailable (%v); the console shows options as degraded", catalogErr)
	}
	control, err := application.NewControlService(application.ServiceDeps{
		Options:       optionCatalog,
		Intents:       intentLog,
		Backup:        autoBackup,
		Workshop:      workshopDownloader,
		Query:         pzQueryAdapter{},
		Capacity:      buildCapacity(cfg, states, files),
		Registry:      instanceRegistry,
		Ports:         &instanceregistry.PortAllocator{Registry: instanceRegistry},
		Nodes:         nodeStore,
		Tasks:         nodeStore.Ledger(),
		Audit:         auditLog,
		Instance:      instance,
		Platform:      runtime.GOOS,
		States:        states,
		Config:        gameConfig,
		Installer:     installer,
		Processes:     supervisorPort,
		ProcessStatus: processStatus,
		Logs:          logs,
		Readiness:     readiness,
		Files:         files,
		Clock:         clock,
		Locks:         &oslock.Locks{ServersRoot: serversRoot, ServiceID: serviceID},
		Templates:     templates,
		BuildLaunchSpec: func(ctx context.Context, state domain.InstanceState, input ports.LaunchInput) (ports.LaunchSpec, error) {
			// A plugin that owns its argv (for example a direct-executable game
			// with no launcher descriptor) builds the spec itself; otherwise the
			// PZ paths below apply.
			if plugin, ok := gamePlugins.Lookup(domain.TemplateID(state.TemplateID)); ok {
				builder, buildErr := plugin.LaunchSpec(ports.PluginDeps{
					DataRoot: absoluteData, ServersRoot: serversRoot, Instance: instance,
					Platform: runtime.GOOS, LaunchVector: launchVector(cfg),
					EvidenceRef: cfg.LaunchEvidenceRef, MemoryMB: serverMemoryMB(cfg, input.InstallDir),
				})
				if buildErr != nil {
					return ports.LaunchSpec{}, buildErr
				}
				if builder != nil {
					return builder(ctx, state, input)
				}
			}
			if input.Vector == pz.VectorLauncherDescriptor {
				descriptor, err := pz.LoadLauncherDescriptor(input.InstallDir)
				if err != nil {
					return ports.LaunchSpec{}, err
				}
				spec, err := pz.DescriptorLaunchSpec(descriptor, pz.DescriptorConfig{
					InstallDir: input.InstallDir,
					CacheDir:   input.CacheDir,
					ServerName: input.ServerName,
					AdminPass:  input.AdminPass,
					MemoryMB:   serverMemoryMB(cfg, input.InstallDir),
					WindowsVer: windowsVersion(),
					AllowedArgs: []string{
						"-Djava.awt.headless=true", "-Dzomboid.steam=1", "-Dzomboid.znetlog=1",
						"-Djava.library.path=natives/", "-XX:-CreateCoredumpOnCrash",
						"-XX:-OmitStackTraceInFastThrow", "-XX:+UseG1GC", "-XX:+UseZGC",
					},
				})
				if err != nil {
					return ports.LaunchSpec{}, err
				}
				for _, rewrite := range spec.Rewrites {
					log.Printf("launcher descriptor rewrite: %s", rewrite)
				}
				return ports.LaunchSpec{Executable: spec.Executable, Args: spec.Args, WorkDir: spec.WorkDir}, nil
			}
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
			// Only games that declare a managed INI target get a config sync;
			// an argv-configured game (for example one launched by a plugin that
			// owns its command line) has nothing to write here.
			if !pluginManagesINI(gamePlugins, state.TemplateID) {
				return nil
			}
			name, _ := state.Variables["SERVER_NAME"].(string)
			if name == "" {
				name = "servertest"
			}
			return gameConfig.ApplyNamed(ctx, state.ID, name, pz.VariableOwnedUpdates(pz.ManagedINIUpdates(state.Variables, state.Ports, state.Mods)))
		},
		LaunchEvidence: application.LaunchEvidence{
			ExecutableName:   launchExecutableName(cfg),
			DirectExecutable: cfg.LaunchDirectExec,
			Reference:        cfg.LaunchEvidenceRef,
			Vector:           launchVector(cfg),
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
	// M6 disconnect reconciliation: a task left in flight by a node that went
	// away is closed as interrupted before the service starts serving.
	if affected, reconcileErr := nodeStore.Ledger().Reconcile(context.Background(), 0); reconcileErr == nil && len(affected) > 0 {
		log.Printf("task ledger reconciliation: %d interrupted task(s) closed", len(affected))
	}
	handler.WithInstanceLister(control).WithNodeStore(nodeStore).WithTaskLedger(nodeStore.Ledger())

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
	includeSaves := flags.Bool("include-saves", false, "include the instance save games (Zomboid/Saves) in the backup")
	dataRoot := flags.String("data-root", "data", "data root containing servers/")
	instance := flags.String("instance", defaultInstance, "instance id")
	destination := flags.String("dest", "", "backup destination root")
	_ = flags.Parse(args)
	if *destination == "" {
		fail(errors.New("--dest is required"))
	}

	tool, err := migrate.New(migrate.Layout{ServersRoot: filepath.Join(*dataRoot, "servers"), Instance: domain.InstanceID(*instance)})
	fail(err)
	record, err := tool.BackupWithOptions(*destination, migrate.BackupOptions{IncludeSaves: *includeSaves})
	if err != nil {
		fail(fmt.Errorf("backup: %w", err))
	}
	fmt.Printf("backup: path=%s files=%d checksum=%s includes=%v\n", record.Path, len(record.Files), record.Checksum, record.Includes)
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

// steamcmdInstallConfig resolves the real installer configuration. It reports
// ok=false when the operator has not provided an executable or an app id, so the
// service fails closed instead of pretending installs can run.
func steamcmdInstallConfig(cfg runtimeConfig, serversRoot string, instance domain.InstanceID, appID string,
	installDir func(domain.InstanceID) (string, error)) (steamcmd.InstallConfig, bool) {
	executable := strings.TrimSpace(cfg.SteamCMDExecutable)
	appID = strings.TrimSpace(appID)
	if executable == "" || appID == "" {
		return steamcmd.InstallConfig{}, false
	}
	steamDir := strings.TrimSpace(cfg.SteamCMDDir)
	if steamDir == "" {
		// ADR §4: the SteamCMD tree never moves with state promotion.
		steamDir = filepath.Join(serversRoot, string(instance), "steamcmd")
	}
	return steamcmd.InstallConfig{
		Executable:  executable,
		SteamDir:    steamDir,
		InstallPath: installDir,
		AppID:       appID,
	}, true
}

// resolveInstaller prefers template metadata for the app id, with an explicit
// operator override, and falls back to the fail-closed unconfigured installer.
func resolveInstaller(cfg runtimeConfig, serversRoot string, instance domain.InstanceID,
	states ports.StateStore, templates ports.TemplateCatalog, files ports.InstanceFiles,
	registry ports.PluginRegistry) (ports.Installer, error) {
	appID := strings.TrimSpace(cfg.SteamAppID)
	if appID == "" {
		// The game plugin owns the Steam identity; the template remains the
		// legacy fallback so existing deployments keep working.
		appID = pluginSteamAppID(registry, states, instance)
	}
	if appID == "" {
		if state, err := states.Load(context.Background(), instance); err == nil {
			if template, ok := templates.Get(domain.TemplateID(state.TemplateID)); ok {
				appID = strings.TrimSpace(template.Summary.AppID)
			}
		}
	}
	config, ok := steamcmdInstallConfig(cfg, serversRoot, instance, appID, files.InstallDir)
	if !ok {
		log.Printf("WARNING: SteamCMD installer is not configured (set GAMESERVER_STEAMCMD_EXECUTABLE and either GAMESERVER_STEAM_APP_ID or a template app id); install requests fail closed")
		return steamcmd.New(), nil
	}
	installer, err := steamcmd.NewConfigured(nil, config)
	if err != nil {
		return nil, err
	}
	log.Printf("SteamCMD installer configured (app %s, dir %s)", config.AppID, config.SteamDir)
	return installer, nil
}

// newLogMarkerReadiness builds the Target-Manifest-approved log fallback oracle:
// PZ's SERVER_PORT is UDP, so the approved oracle is the console marker recorded
// in the manifest, observed within the same 60-second window used for A2S.
func newLogMarkerReadiness(marker string, logs ports.LogSource, window time.Duration) ports.ReadinessProbe {
	if window <= 0 {
		window = defaultReadinessWindow
	}
	const (
		interval = 2 * time.Second
		// scanAll reads the whole bounded ring buffer: the log source returns
		// nothing for a non-positive limit, which silently hid the marker.
		scanAll = 1000
	)
	return readinessFunc(func(ctx context.Context, _ domain.InstanceID) (bool, error) {
		deadline := time.Now().Add(window)
		for {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			for _, line := range logs.Recent(scanAll) {
				if strings.Contains(line, marker) {
					return true, nil
				}
			}
			if time.Now().After(deadline) {
				return false, nil
			}
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(interval):
			}
		}
	})
}

// defaultReadinessWindow is the approved A2S window; a Manifest may record a
// longer one for builds whose console marker arrives later.
const defaultReadinessWindow = 60 * time.Second

// readinessTimeout converts the configured seconds into a bounded duration:
// absent or zero keeps the default, values above one hour are clamped.
func readinessTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultReadinessWindow
	}
	if seconds > 3600 {
		seconds = 3600
	}
	return time.Duration(seconds) * time.Second
}

type readinessFunc func(context.Context, domain.InstanceID) (bool, error)

func (f readinessFunc) Ready(ctx context.Context, instance domain.InstanceID) (bool, error) {
	return f(ctx, instance)
}

// launchVector resolves the effective vector without runtime probing: the
// default keeps the historical direct-executable behaviour, and the descriptor
// vector must be selected explicitly (its evidence is reviewed separately).
func launchVector(cfg runtimeConfig) string {
	if strings.TrimSpace(cfg.LaunchVector) == pz.VectorLauncherDescriptor {
		return pz.VectorLauncherDescriptor
	}
	return pz.VectorDirectExecutable
}

// launchExecutableName reports the artifact name recorded in the evidence.
func launchExecutableName(cfg runtimeConfig) string {
	if launchVector(cfg) == pz.VectorLauncherDescriptor {
		return filepath.ToSlash(filepath.Join("jre64", "bin", "java.exe"))
	}
	return cfg.LaunchExecutable
}

// serverMemoryMB resolves the JVM heap for the descriptor vector: explicit
// configuration wins, otherwise the vendor descriptor's own -Xmx is reused.
func serverMemoryMB(cfg runtimeConfig, installDir string) int {
	if cfg.ServerMemoryMB > 0 {
		return cfg.ServerMemoryMB
	}
	if launchVector(cfg) != pz.VectorLauncherDescriptor {
		return 0
	}
	if descriptor, err := pz.LoadLauncherDescriptor(installDir); err == nil {
		if size := descriptor.HeapMB(); size > 0 {
			return size
		}
	}
	return 0
}

// windowsVersion reports the OS version used for descriptor platform rules.
func windowsVersion() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	if out, err := exec.Command("cmd", "/c", "ver").Output(); err == nil {
		if match := regexp.MustCompile(`(\d+\.\d+\.\d+)`).FindStringSubmatch(string(out)); match != nil {
			return match[1]
		}
	}
	return ""
}

// loadOptionCatalog resolves the option catalogue for the instance's template.
// A missing catalogue is not fatal: the console degrades and says so.
func loadOptionCatalog(cfg runtimeConfig, states ports.StateStore, templates ports.TemplateCatalog) (ports.OptionCatalog, error) {
	dir := strings.TrimSpace(cfg.CatalogsDir)
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(filepath.Dir(exe), "catalogs")
		if _, err := os.Stat(dir); err != nil {
			dir = filepath.Join("catalogs")
		}
	}
	state, err := states.Load(context.Background(), cfg.Instance)
	if err != nil {
		return nil, err
	}
	templateID := strings.TrimSpace(state.TemplateID)
	if templateID == "" {
		if template, ok := templates.Get("project_zomboid"); ok {
			templateID = string(template.Summary.ID)
		}
	}
	catalog, err := optioncatalog.New(dir, templateID)
	if err != nil {
		return nil, err
	}
	return catalog, nil
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

func envFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
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

// pzQueryAdapter adapts the pz A2S query parser to ports.GameQuerier.
type pzQueryAdapter struct{}

func (pzQueryAdapter) Query(ctx context.Context, host string, port int) (ports.GameQueryInfo, error) {
	info, err := pz.QueryA2SInfo(ctx, host, port)
	if err != nil {
		return ports.GameQueryInfo{}, err
	}
	return ports.GameQueryInfo{Name: info.Name, Map: info.Map, Players: info.Players, Max: info.Max}, nil
}

// buildCapacity wires the M3.5 capacity policy when thresholds are configured:
// quota comes from the instance state, usage from the instance-files walk.
// Without thresholds (or on a state read failure) the policy is disabled and
// the historical no-limit behaviour stays in place.
func buildCapacity(cfg runtimeConfig, states ports.StateStore, files ports.InstanceFiles) ports.CapacityChecker {
	if cfg.CapacitySoftPercent <= 0 && cfg.CapacityHardPercent <= 0 {
		return nil
	}
	instance, err := states.Load(context.Background(), domain.InstanceID(cfg.Instance))
	if err != nil {
		return nil
	}
	quota := instance.QuotaGB
	if quota <= 0 {
		return nil
	}
	return application.NewCapacity(cfg.CapacitySoftPercent, cfg.CapacityHardPercent, quota, files)
}

// pluginSteamAppID resolves the Steam app id from the registered game plugin so
// shared wiring never hardcodes a game identity.
func pluginSteamAppID(registry ports.PluginRegistry, states ports.StateStore, instance domain.InstanceID) string {
	if registry == nil {
		return ""
	}
	state, err := states.Load(context.Background(), instance)
	if err != nil {
		return ""
	}
	plugin, ok := registry.Lookup(domain.TemplateID(state.TemplateID))
	if !ok {
		return ""
	}
	appID, _ := plugin.InstallerSource()
	return strings.TrimSpace(appID)
}

// pluginReadinessMarker resolves the manifest-recorded readiness marker from
// the plugin descriptor.
func pluginReadinessMarker(registry ports.PluginRegistry, states ports.StateStore, instance domain.InstanceID) string {
	if registry == nil {
		return ""
	}
	state, err := states.Load(context.Background(), instance)
	if err != nil {
		return ""
	}
	plugin, ok := registry.Lookup(domain.TemplateID(state.TemplateID))
	if !ok {
		return ""
	}
	spec := plugin.Descriptor().Readiness
	if spec.Mode != "log-marker" {
		return ""
	}
	return strings.TrimSpace(spec.Marker)
}

// pluginManagesINI reports whether the resolved game plugin declares a managed
// INI configuration target; games configured purely through argv return false
// so the PZ writer never runs against a foreign install directory.
func pluginManagesINI(registry ports.PluginRegistry, templateID string) bool {
	if registry == nil {
		return true // legacy wiring keeps the historical behaviour
	}
	plugin, ok := registry.Lookup(domain.TemplateID(templateID))
	if !ok {
		return true
	}
	for _, target := range plugin.Descriptor().ConfigTargets {
		if target == "ini" {
			return true
		}
	}
	return false
}
