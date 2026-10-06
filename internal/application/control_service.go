package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// LaunchEvidence is the reviewed mechanical evidence used to select the launch
// vector. It comes from the Target Manifest, never from template metadata.
type LaunchEvidence struct {
	ExecutableName   string
	DirectExecutable bool
	Reference        string
	Vector           string
}

// ServiceDeps contains every outbound dependency of the control service.
type ServiceDeps struct {
	Instance        domain.InstanceID
	Platform        string
	States          ports.StateStore
	Config          ports.GameConfig
	Installer       ports.Installer
	Processes       ports.ProcessSupervisor
	ProcessStatus   ports.ProcessStatusProvider
	Readiness       ports.ReadinessProbe
	Locks           ports.InstanceLock
	Templates       ports.TemplateCatalog
	Logs            ports.LogSource
	Files           ports.InstanceFiles
	Clock           ports.Clock
	BuildLaunchSpec ports.LaunchSpecBuilder
	ApplyGameConfig ports.StateConfigApplier
	Options         ports.OptionCatalog
	LaunchEvidence  LaunchEvidence
	Intents         ports.IntentLog
	Backup          ports.BackupOperator
	Workshop        ports.WorkshopDownloader
	Query           ports.GameQuerier
	Capacity        ports.CapacityChecker
	Registry        ports.InstanceRegistry
	Ports           ports.PortAllocator
	Nodes           ports.NodeStore
	Tasks           ports.TaskLedger
	Audit           ports.AuditLog
}

type diskUsageProvider interface {
	DiskUsageMB(domain.InstanceID) (float64, error)
}

// ControlService implements the Control use cases over injected ports.
type ControlService struct {
	deps ServiceDeps

	mu        sync.Mutex
	install   installTask
	ready     bool
	readiness string
	// readinessAt records when the current readiness state was entered.
	readinessAt string
	// pendingRestart records that a restart-requiring option was saved while the
	// server was running; it is cleared on the next successful start.
	pendingRestart atomic.Bool

	backupMu      sync.Mutex
	backupRunning bool
	lastBackup    autoBackupState
	backupSettle  time.Duration
	// modTask tracks the background install of workshop items that were added
	// through the configuration surface: adding a mod id must configure and
	// download it, not merely record it.
	modTask modInstallTask
	modMu   sync.Mutex
}

type modInstallTask struct {
	running  bool
	status   string
	progress float64
	message  string
	pending  []string
}

type autoBackupState struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
	At      string `json:"at,omitempty"`
}

// autoBackupSettleDelay waits for the PZ exit-time sandbox rewrite to settle
// before backing up so the captured configuration is the stable one.
const autoBackupSettleDelay = 5 * time.Second

type installTask struct {
	running  bool
	version  string
	status   string
	progress float64
	message  string
	errText  *string
	cancel   context.CancelFunc
}

// NewControlService validates the dependency set and returns a usable service.
func NewControlService(deps ServiceDeps) (*ControlService, error) {
	missing := make([]string, 0, 8)
	if deps.Instance == "" {
		missing = append(missing, "instance")
	}
	if deps.States == nil {
		missing = append(missing, "state store")
	}
	if deps.Files == nil {
		missing = append(missing, "instance files")
	}
	if deps.Templates == nil {
		missing = append(missing, "template catalog")
	}
	if deps.Processes == nil {
		missing = append(missing, "process supervisor")
	}
	if deps.ProcessStatus == nil {
		missing = append(missing, "process status")
	}
	if deps.Logs == nil {
		missing = append(missing, "log source")
	}
	if deps.Clock == nil {
		missing = append(missing, "clock")
	}
	if deps.BuildLaunchSpec == nil {
		missing = append(missing, "launch spec builder")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing control dependencies: %s", strings.Join(missing, ", "))
	}
	if deps.ApplyGameConfig == nil {
		deps.ApplyGameConfig = func(context.Context, domain.InstanceState) error { return nil }
	}
	service := &ControlService{
		deps:      deps,
		install:   installTask{status: "IDLE", message: "尚未运行"},
		readiness: "unknown",
	}
	return service, nil
}

func (s *ControlService) load(ctx context.Context) (domain.InstanceState, error) {
	state, err := s.deps.States.Load(ctx, s.deps.Instance)
	if err != nil {
		return domain.InstanceState{}, WrapError(CodeOperationFailed, "读取实例状态失败", err)
	}
	return state, nil
}

func (s *ControlService) template(state domain.InstanceState) (domain.Template, error) {
	template, ok := s.deps.Templates.Get(domain.TemplateID(state.TemplateID))
	if !ok {
		return domain.Template{}, NewError(CodeOperationFailed, "模板不可用")
	}
	return template, nil
}

func (s *ControlService) processStatus(ctx context.Context) ports.ProcessStatus {
	status, err := s.deps.ProcessStatus.ProcessStatus(ctx, s.deps.Instance)
	if err != nil {
		return ports.ProcessStatus{Status: "STOPPED"}
	}
	if status.Status == "" {
		status.Status = "STOPPED"
	}
	return status
}

func (s *ControlService) installSnapshot() InstallStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return InstallStatus{
		Version:  s.install.version,
		Status:   s.install.status,
		Progress: s.install.progress,
		Message:  s.install.message,
		Error:    s.install.errText,
	}
}

// Status implements Control.
func (s *ControlService) Status(ctx context.Context) (StatusResponse, error) {
	state, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	process := s.processStatus(ctx)
	install := s.installSnapshot()
	s.mu.Lock()
	lastBackup := s.lastBackup
	s.mu.Unlock()

	status := process.Status
	if install.Status == "INSTALLING" {
		status = "INSTALLING"
	}
	pid := any(nil)
	if process.PID > 0 {
		pid = process.PID
	}
	mods := state.Mods
	if mods == nil {
		mods = map[string]any{"workshop_ids": []any{}, "mod_names": []any{}}
	}
	billing := state.Billing
	if billing == nil {
		billing = map[string]any{}
	}
	quota := state.QuotaGB
	if quota <= 0 {
		quota = 30
	}
	disk := map[string]any{"used_mb": 0.0, "quota_gb": quota, "usage_percent": 0.0}
	if provider, ok := s.deps.Files.(diskUsageProvider); ok {
		if used, err := provider.DiskUsageMB(s.deps.Instance); err == nil {
			disk["used_mb"] = round1(used)
			if quota > 0 {
				disk["usage_percent"] = round1(used / (quota * 1024) * 100)
			}
		}
	}

	s.mu.Lock()
	ready := s.ready
	readiness := s.readiness
	readinessTimeline := map[string]any{"state": readiness, "since": s.readinessAt}
	s.modMu.Lock()
	modTask := s.modTask
	s.modMu.Unlock()
	s.mu.Unlock()

	return StatusResponse{
		"instance_id":    string(s.deps.Instance),
		"name":           state.Name,
		"template_id":    state.TemplateID,
		"is_installed":   s.deps.Files.IsInstalled(s.deps.Instance),
		"status":         status,
		"running":        process.Running,
		"last_backup":    lastBackup,
		"pid":            pid,
		"cpu_percent":    round1(process.CPUPercent),
		"memory_mb":      round1(process.MemoryMB),
		"uptime_seconds": process.UptimeSeconds,
		"disk":           disk,
		"ports":          state.Ports,
		"variables":      state.Variables,
		"mods":           mods,
		"billing":        billing,
		"steamcmd": map[string]any{
			"busy":        install.Status == "INSTALLING",
			"progress":    install.Progress,
			"status_text": install.Message,
		},
		"platform":           s.deps.Platform,
		"ready":              ready,
		"readiness":          readiness,
		"readiness_timeline": readinessTimeline,
		"mod_task": map[string]any{
			"status": modTask.status, "progress": modTask.progress, "message": modTask.message,
		},
		"game_query":      s.queryGame(ctx),
		"capacity":        s.capacityStatus(ctx),
		"pending_restart": s.pendingRestart.Load(),
	}, nil
}

// Templates implements Control.
func (s *ControlService) Templates(context.Context) ([]TemplateSummary, error) {
	summaries := s.deps.Templates.List()
	result := make([]TemplateSummary, 0, len(summaries))
	for _, summary := range summaries {
		entry := TemplateSummary{
			ID:          string(summary.ID),
			Name:        summary.Name,
			Category:    summary.Category,
			Icon:        summary.Icon,
			Author:      summary.Author,
			Version:     summary.Version,
			Description: summary.Description,
			SupportedOS: summary.SupportedOS,
			AppID:       summary.AppID,
		}
		if template, ok := s.deps.Templates.Get(summary.ID); ok {
			for _, version := range template.Versions {
				entry.Versions = append(entry.Versions, TemplateVersion{
					Label: version.Label, Branch: version.Branch, BuildID: version.BuildID,
					Default: version.Default, EvidenceRef: version.EvidenceRef,
				})
			}
		}
		result = append(result, entry)
	}
	return result, nil
}

// BeginInstall implements Control.
func (s *ControlService) BeginInstall(ctx context.Context, version string) (InstallAccepted, error) {
	if s.deps.Installer == nil {
		return InstallAccepted{}, NewError(CodeOperationFailed, "安装器不可用")
	}
	if err := ctx.Err(); err != nil {
		return InstallAccepted{}, WrapError(CodeOperationFailed, "安装已取消", err)
	}
	if s.processStatus(ctx).Running {
		return InstallAccepted{}, NewError(CodeInstallWhileRunning, "")
	}

	if err := s.checkCapacity(ctx); err != nil {
		return InstallAccepted{}, err
	}
	lease, lockErr := s.ensureLock(ctx)
	if lockErr != nil {
		return InstallAccepted{}, lockErr
	}
	if lease != nil {
		defer lease.Release()
	}
	if err := s.reconcileInstallIntent(ctx); err != nil {
		return InstallAccepted{}, err
	}
	s.mu.Lock()
	if s.install.running {
		s.mu.Unlock()
		return InstallAccepted{}, NewError(CodeInstallAlreadyRunning, "")
	}
	taskCtx, cancel := context.WithCancel(context.Background())
	s.install = installTask{running: true, status: "INSTALLING", progress: 0, message: "准备安装", cancel: cancel}
	s.mu.Unlock()
	if err := s.persistIntentPhase(ports.PhaseRequested, "", "", s.deps.Clock.Now(), "", 0, "准备安装"); err != nil {
		s.clearInstall()
		return InstallAccepted{}, err
	}

	// Version selection happens after the conflict check so a rejected request
	// can never disturb an install that is already running.
	state, err := s.load(ctx)
	if err != nil {
		s.clearInstall()
		return InstallAccepted{}, err
	}
	template, err := s.template(state)
	if err != nil {
		s.clearInstall()
		return InstallAccepted{}, err
	}
	branch, err := resolveInstallVersion(template, version)
	if err != nil {
		s.clearInstall()
		return InstallAccepted{}, err
	}
	s.mu.Lock()
	s.install.version = branch
	s.mu.Unlock()
	buildID := branchBuildID(branch, template)
	pre, fpErr := s.deps.Files.Fingerprint(s.deps.Instance)
	if fpErr != nil {
		s.clearInstall()
		return InstallAccepted{}, WrapError(CodeOperationFailed, "无法核对安装产物", fpErr)
	}
	if err := s.persistIntentPhase(ports.PhaseRunning, branch, buildID, s.deps.Clock.Now(), pre.ManifestSHA, pre.TotalBytes, "安装中"); err != nil {
		s.clearInstall()
		return InstallAccepted{}, err
	}

	go func(branch string) {
		defer cancel()
		progress := func(value ports.Progress) {
			s.mu.Lock()
			if value.Percent >= 0 {
				s.install.progress = value.Percent
			}
			if value.Message != "" {
				s.install.message = value.Message
			}
			s.mu.Unlock()
		}
		err := s.deps.Installer.Install(taskCtx, ports.InstallRequest{InstanceID: s.deps.Instance, Validate: true, Version: branch}, progress)

		s.mu.Lock()
		defer s.mu.Unlock()
		s.install.running = false
		s.install.cancel = nil
		switch {
		case err == nil:
			// Fold the success through the intent so a crash between the last
			// progress report and the terminal update reconciles as DONE.
			post, verifyErr := s.deps.Files.Fingerprint(s.deps.Instance)
			if verifyErr == nil {
				_ = s.persistIntentPhase(ports.PhaseVerifying, branch, buildID, s.deps.Clock.Now(), post.ManifestSHA, post.TotalBytes, "校验中")
			}
			s.install.status = "COMPLETED"
			s.install.progress = 100
			s.install.message = "安装完成"
			s.install.errText = nil
			_ = s.persistIntentPhase(ports.PhaseDone, branch, buildID, s.deps.Clock.Now(), post.ManifestSHA, post.TotalBytes, "安装完成")
			_ = s.clearIntent(context.Background())
		default:
			message := "安装失败，请查看日志"
			if errors.Is(err, context.Canceled) {
				message = "安装已取消"
			}
			s.install.status = "FAILED"
			s.install.message = message
			text := err.Error()
			s.install.errText = &text
			_ = s.persistIntentPhase(ports.PhaseFailed, branch, buildID, s.deps.Clock.Now(), "", 0, message)
			_ = s.clearIntent(context.Background())
		}
	}(branch)
	return InstallAccepted{Message: "安装/更新任务已启动", Status: "INSTALLING"}, nil
}

// InstallState implements Control.
func (s *ControlService) InstallState(context.Context) (InstallStatus, error) {
	return s.installSnapshot(), nil
}

func (s *ControlService) ensureLock(ctx context.Context) (ports.Lease, error) {
	if s.deps.Locks == nil {
		return nil, nil
	}
	lease, err := s.deps.Locks.Acquire(ctx, s.deps.Instance)
	if err != nil {
		return nil, mapLockError(err)
	}
	return lease, nil
}

func mapLockError(err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "owned by another process"):
		return WrapError(CodeInstanceOwned, "", err)
	case strings.Contains(message, "recovery required"):
		return WrapError(CodeRecoveryRequired, "", err)
	default:
		return WrapError(CodeOperationFailed, "无法取得实例锁", err)
	}
}

// Start implements Control.
func (s *ControlService) Start(ctx context.Context) (StartResult, error) {
	if s.processStatus(ctx).Running {
		return StartResult{Message: "服务器已在运行中", Running: true}, nil
	}
	if s.installSnapshot().Status == "INSTALLING" {
		return StartResult{}, NewError(CodeServerInstalling, "")
	}
	if !s.deps.Files.IsInstalled(s.deps.Instance) {
		return StartResult{}, NewError(CodeServerNotInstalled, "")
	}
	evidence := s.deps.LaunchEvidence
	switch evidence.Vector {
	case "launcher-descriptor":
		// The descriptor vector executes the bundled JRE, so it needs both a
		// reviewed vector name and a manifest reference; nothing else counts.
		if evidence.ExecutableName == "" || strings.TrimSpace(evidence.Reference) == "" {
			return StartResult{}, NewError(CodeOperationFailed, "缺少已审核的启动向量证据（Target Manifest）")
		}
	default:
		if !evidence.DirectExecutable && evidence.ExecutableName == "" {
			return StartResult{}, NewError(CodeOperationFailed, "缺少已审核的启动向量证据（Target Manifest）")
		}
	}

	state, err := s.load(ctx)
	if err != nil {
		return StartResult{}, err
	}
	installDir, err := s.deps.Files.InstallDir(s.deps.Instance)
	if err != nil {
		return StartResult{}, WrapError(CodeOperationFailed, "无法定位安装目录", err)
	}
	cacheDir, err := s.deps.Files.CacheDir(s.deps.Instance)
	if err != nil {
		return StartResult{}, WrapError(CodeOperationFailed, "无法定位缓存目录", err)
	}
	adminPass, _ := state.Variables["ADMIN_PASSWORD"].(string)
	serverName, _ := state.Variables["SERVER_NAME"].(string)
	spec, err := s.deps.BuildLaunchSpec(ctx, state, ports.LaunchInput{
		InstallDir:       installDir,
		CacheDir:         cacheDir,
		Platform:         s.deps.Platform,
		ServerName:       serverName,
		AdminPass:        adminPass,
		ExecutableName:   s.deps.LaunchEvidence.ExecutableName,
		DirectExecutable: s.deps.LaunchEvidence.DirectExecutable,
		EvidenceRef:      s.deps.LaunchEvidence.Reference,
		Vector:           s.deps.LaunchEvidence.Vector,
	})
	if err != nil {
		return StartResult{}, WrapError(CodeStartFailed, "", err)
	}
	lease, err := s.ensureLock(ctx)
	if err != nil {
		return StartResult{}, err
	}
	if lease != nil {
		defer lease.Release()
	}
	if err := s.reconcileInstallIntent(ctx); err != nil {
		return StartResult{}, err
	}
	if err := s.deps.ApplyGameConfig(ctx, state); err != nil {
		return StartResult{}, WrapError(CodeStartFailed, "同步配置失败", err)
	}
	if _, err := s.deps.Processes.Start(ctx, spec); err != nil {
		return StartResult{}, WrapError(CodeStartFailed, "", err)
	}
	s.pendingRestart.Store(false)
	s.scheduleReadiness(ctx)
	return StartResult{Message: "启动指令已执行", Running: true}, nil
}

func (s *ControlService) scheduleReadiness(ctx context.Context) {
	if s.deps.Readiness == nil {
		return
	}
	s.mu.Lock()
	s.ready = false
	s.readiness = "checking"
	s.readinessAt = s.deps.Clock.Now().Format(time.RFC3339)
	s.mu.Unlock()
	go func() {
		probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
		defer cancel()
		ready, err := s.deps.Readiness.Ready(probeCtx, s.deps.Instance)
		s.mu.Lock()
		defer s.mu.Unlock()
		now := s.deps.Clock.Now().Format(time.RFC3339)
		if err != nil {
			s.readiness = "failed"
			s.readinessAt = now
			return
		}
		s.ready = ready
		if ready {
			s.readiness = "ready"
		} else {
			s.readiness = "timeout"
		}
		s.readinessAt = now
	}()
}

// Stop implements Control.
func (s *ControlService) Stop(ctx context.Context) (OperationResult, error) {
	if err := s.deps.Processes.Stop(ctx, s.deps.Instance); err != nil {
		return OperationResult{}, WrapError(CodeStopFailed, "", err)
	}
	s.mu.Lock()
	s.ready = false
	s.readiness = "unknown"
	s.mu.Unlock()
	s.scheduleAutomaticBackup()
	return OperationResult{Message: "服务器已停止", Success: true}, nil
}

// scheduleAutomaticBackup runs one best-effort backup after a clean stop. It
// never delays or fails the stop: the backup waits for the PZ config rewrite
// (known deviation 2) to settle, records its own state in last_backup and is
// skipped when a backup is already in flight or disabled.
func (s *ControlService) scheduleAutomaticBackup() {
	if s.deps.Backup == nil || !s.deps.Backup.Enabled() {
		return
	}
	// Self-stimulation guard: a backup whose own footprint would cross the hard
	// limit is deferred (recorded as pending), never attempted and never failed.
	if err := s.checkCapacity(context.Background()); err != nil {
		s.mu.Lock()
		s.lastBackup = autoBackupState{State: "pending", Message: err.Error(), At: s.deps.Clock.Now().Format(time.RFC3339)}
		s.mu.Unlock()
		return
	}
	s.backupMu.Lock()
	if s.backupRunning {
		s.backupMu.Unlock()
		return
	}
	s.backupRunning = true
	s.backupMu.Unlock()
	go func() {
		defer func() {
			s.backupMu.Lock()
			s.backupRunning = false
			s.backupMu.Unlock()
		}()
		// Give the game process time to rewrite its sandbox file after exit.
		sleep := s.backupSettle
		if sleep < 0 {
			sleep = autoBackupSettleDelay
		}
		time.Sleep(sleep)
		state := autoBackupState{At: s.deps.Clock.Now().Format(time.RFC3339)}
		if _, err := s.deps.Backup.BackupNow(); err != nil {
			state.State = "failed"
			state.Message = err.Error()
		} else {
			state.State = "completed"
			state.Message = "自动备份完成"
		}
		s.mu.Lock()
		s.lastBackup = state
		s.mu.Unlock()
	}()
}

// Restart implements Control.
func (s *ControlService) Restart(ctx context.Context) (OperationResult, error) {
	if s.processStatus(ctx).Running {
		if _, err := s.Stop(ctx); err != nil {
			return OperationResult{}, NewError(CodeRestartStopFailed, "")
		}
	}
	if !s.deps.Files.IsInstalled(s.deps.Instance) {
		return OperationResult{}, NewError(CodeRestartNotInstalled, "")
	}
	if _, err := s.Start(ctx); err != nil {
		return OperationResult{}, NewError(CodeRestartFailed, "")
	}
	return OperationResult{Message: "服务器已重启", Success: true}, nil
}

// Kill implements Control.
func (s *ControlService) Kill(ctx context.Context) (OperationResult, error) {
	if err := s.deps.Processes.Kill(ctx, s.deps.Instance); err != nil {
		return OperationResult{Message: "强制终止失败", Success: false}, nil
	}
	s.mu.Lock()
	s.ready = false
	s.readiness = "unknown"
	s.mu.Unlock()
	return OperationResult{Message: "强制终止指令已执行", Success: true}, nil
}

// SendCommand implements Control.
func (s *ControlService) SendCommand(ctx context.Context, command string) (CommandResult, error) {
	if !s.processStatus(ctx).Running {
		return CommandResult{}, NewError(CodeServerNotRunning, "")
	}
	if err := s.deps.Processes.SendInput(ctx, s.deps.Instance, command); err != nil {
		return CommandResult{}, WrapError(CodeOperationFailed, "发送指令失败", err)
	}
	return CommandResult{Success: true}, nil
}

// SendConsoleInput implements Control for the WebSocket console. It reports
// running=false instead of failing so the transport can emit the D3 error
// frame while keeping the connection open.
func (s *ControlService) SendConsoleInput(ctx context.Context, text string) (bool, error) {
	if !s.processStatus(ctx).Running {
		return false, nil
	}
	if err := s.deps.Processes.SendInput(ctx, s.deps.Instance, text); err != nil {
		return true, WrapError(CodeOperationFailed, "发送控制台输入失败", err)
	}
	return true, nil
}

// Logs implements Control.
func (s *ControlService) Logs(_ context.Context, limit int) ([]string, error) {
	return s.deps.Logs.Recent(limit), nil
}

// Config implements Control.
func (s *ControlService) Config(ctx context.Context) (ConfigSnapshot, error) {
	state, err := s.load(ctx)
	if err != nil {
		return ConfigSnapshot{}, err
	}
	template, err := s.template(state)
	if err != nil {
		return ConfigSnapshot{}, err
	}
	snapshot := ConfigSnapshot{
		Variables: state.Variables,
		Ports:     state.Ports,
		Template:  ConfigTemplate{ID: string(template.Summary.ID), Name: template.Summary.Name},
	}
	for _, variable := range template.Variables {
		field := ConfigField{
			Key:          variable.Key,
			Label:        variable.Label,
			Type:         variable.Type,
			Description:  variable.Description,
			Required:     variable.Required,
			UserEditable: variable.UserEditable,
			Default:      variable.Default,
			Value:        state.Variables[variable.Key],
		}
		if field.Value == nil {
			field.Value = variable.Default
		}
		if variable.Type == "password" {
			field.Value = nil
			field.Default = nil
		}
		if variable.Min != nil || variable.Max != nil {
			validation := map[string]any{}
			if variable.Min != nil {
				validation["min"] = *variable.Min
			}
			if variable.Max != nil {
				validation["max"] = *variable.Max
			}
			field.Validation = validation
		}
		snapshot.Fields = append(snapshot.Fields, field)
	}
	for _, port := range template.Ports {
		snapshot.AllowedPorts = append(snapshot.AllowedPorts, port.Key)
	}
	s.applyOptions(ctx, state, &snapshot)
	return snapshot, nil
}

// applyOptions projects the validated option catalogue with file-backed values.
// A missing or unreadable catalogue degrades the console (flag + empty list)
// instead of failing the whole configuration view.
func (s *ControlService) applyOptions(ctx context.Context, state domain.InstanceState, snapshot *ConfigSnapshot) {
	if s.deps.Options == nil {
		snapshot.CatalogDegraded = true
		return
	}
	values, err := s.deps.Config.ReadOptions(ctx, state.ID)
	if err != nil {
		snapshot.CatalogDegraded = true
		return
	}
	groups := map[string]bool{}
	for _, spec := range s.deps.Options.Options() {
		option := ConfigOption{
			Key: spec.Name(), Target: string(spec.Target), Label: spec.Label, Type: spec.Type,
			Secret: spec.Secret, Group: spec.Group, Description: spec.Description,
			RequiresRestart: spec.RequiresRestart, Writable: spec.Writable, Clearable: spec.Clearable,
			Min: spec.Min, Max: spec.Max, Enum: spec.Enum, Source: "file",
		}
		if raw, ok := values[spec.Name()]; ok {
			option.Value = optionValue(spec, raw)
		} else {
			option.Value = optionValue(spec, spec.Default)
		}
		if spec.Secret {
			// Secrets are never echoed, not even their default.
			option.Value = nil
			option.Default = nil
		} else {
			option.Default = optionValue(spec, spec.Default)
		}
		if option.Group != "" {
			groups[option.Group] = true
		}
		snapshot.Options = append(snapshot.Options, option)
	}
	for group := range groups {
		snapshot.Groups = append(snapshot.Groups, group)
	}
	sort.Strings(snapshot.Groups)
}

// optionValue renders a catalogue value with its declared type so the console
// receives numbers and booleans rather than strings.
func optionValue(spec ports.OptionSpec, raw string) any {
	trimmed := strings.TrimSpace(raw)
	switch spec.Type {
	case "bool":
		return trimmed == "true"
	case "int":
		if number, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return number
		}
	case "float":
		if number, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return number
		}
	}
	if len(trimmed) >= 2 && strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") {
		return trimmed[1 : len(trimmed)-1]
	}
	return raw
}

// UpdateConfig implements Control.
func (s *ControlService) UpdateConfig(ctx context.Context, update ConfigUpdate) (ConfigUpdateResult, error) {
	snapshot, err := s.Config(ctx)
	if err != nil {
		return ConfigUpdateResult{}, err
	}
	if err := ValidateConfigUpdate(snapshot, update); err != nil {
		return ConfigUpdateResult{}, err
	}
	if len(update.Options) > 0 {
		if err := ValidateOptionUpdate(s.deps.Options, update.Options); err != nil {
			return ConfigUpdateResult{}, err
		}
	}
	state, err := s.load(ctx)
	if err != nil {
		return ConfigUpdateResult{}, err
	}
	updated := state.Clone()
	for key, value := range update.Variables {
		if text, ok := value.(string); ok {
			updated.Variables[key] = text
			continue
		}
		updated.Variables[key] = value
	}
	for key, value := range update.Ports {
		updated.Ports[key] = value
	}
	lease, err := s.ensureLock(ctx)
	if err != nil {
		return ConfigUpdateResult{}, err
	}
	if lease != nil {
		defer lease.Release()
	}
	if err := s.deps.States.Save(ctx, updated); err != nil {
		return ConfigUpdateResult{}, WrapError(CodeOperationFailed, "保存配置失败", err)
	}
	if err := s.deps.ApplyGameConfig(ctx, updated); err != nil {
		return ConfigUpdateResult{}, WrapError(CodeOperationFailed, "同步游戏配置失败", err)
	}
	if len(update.Options) > 0 {
		values := make(map[string]string, len(update.Options))
		for name, value := range update.Options {
			values[name] = value
		}
		targets := OptionTargets(s.deps.Options, values)
		if err := s.deps.Config.ApplyOptionValues(ctx, updated.ID, values, targets); err != nil {
			// The underlying cause is surfaced (key names only, never values) so a failed
			// option write is diagnosable from the API response itself.
			detail := "保存配置项失败"
			if err != nil && err.Error() != "" {
				detail = "保存配置项失败: " + err.Error()
			}
			return ConfigUpdateResult{}, WrapError(CodeOperationFailed, detail, err)
		}
		if OptionRequiresRestart(s.deps.Options, update.Options) && s.processStatus(ctx).Status == "RUNNING" {
			s.pendingRestart.Store(true)
		}
	}
	return ConfigUpdateResult{Message: "配置已保存并同步", State: stateMap(updated)}, nil
}

// AddMod implements Control. Mods are registered only; nothing is downloaded.
func (s *ControlService) AddMod(ctx context.Context, request AddModRequest) (ModsResult, error) {
	state, err := s.load(ctx)
	if err != nil {
		return ModsResult{}, err
	}
	updated := state.Clone()
	mods := ensureMods(updated)
	ids := stringSlice(mods["workshop_ids"])
	if !containsString(ids, request.WorkshopID) {
		ids = append(ids, request.WorkshopID)
	}
	mods["workshop_ids"] = ids
	if request.ModName != nil && *request.ModName != "" {
		names := stringSlice(mods["mod_names"])
		if !containsString(names, *request.ModName) {
			names = append(names, *request.ModName)
		}
		mods["mod_names"] = names
	}
	if err := s.deps.States.Save(ctx, updated); err != nil {
		return ModsResult{}, WrapError(CodeOperationFailed, "保存模组失败", err)
	}
	s.scheduleModDownload([]string{request.WorkshopID})
	return ModsResult{Message: "模组已登记（尚未下载）", Mods: mods}, nil
}

// DownloadMod downloads the workshop item first and registers it only after
// the content landed and was structurally checked. A failure never leaves a
// registered-but-missing mod behind; the legacy registration endpoint stays
// untouched (M3.3, D11).
func (s *ControlService) DownloadMod(ctx context.Context, workshopID string, modName *string) (ModsResult, error) {
	if err := s.checkCapacity(ctx); err != nil {
		return ModsResult{}, err
	}
	if s.deps.Workshop == nil {
		return ModsResult{}, NewError(CodeOperationFailed, "下载器不可用")
	}
	if strings.TrimSpace(workshopID) == "" {
		return ModsResult{}, NewError(CodeOperationFailed, "workshop id 不能为空")
	}
	if s.installSnapshot().Status == "INSTALLING" {
		return ModsResult{}, NewError(CodeInstallAlreadyRunning, "")
	}
	state, err := s.load(ctx)
	if err != nil {
		return ModsResult{}, err
	}
	if _, err := s.deps.Workshop.DownloadWorkshopItem(ctx, s.deps.Instance, workshopID, nil); err != nil {
		message := err.Error()
		switch {
		case strings.Contains(message, "steamcmd task already running"):
			return ModsResult{}, NewError(CodeOperationFailed, "已有安装/下载任务在运行中")
		default:
			return ModsResult{}, NewError(CodeOperationFailed, "模组下载失败："+message)
		}
	}
	if err := s.checkCapacity(ctx); err != nil {
		return ModsResult{}, err
	}
	updated := state.Clone()
	mods := ensureMods(updated)
	ids := stringSlice(mods["workshop_ids"])
	if !containsString(ids, workshopID) {
		ids = append(ids, workshopID)
	}
	mods["workshop_ids"] = ids
	if modName != nil && *modName != "" {
		names := stringSlice(mods["mod_names"])
		if !containsString(names, *modName) {
			names = append(names, *modName)
		}
		mods["mod_names"] = names
	}
	if err := s.deps.States.Save(ctx, updated); err != nil {
		return ModsResult{}, WrapError(CodeOperationFailed, "保存模组失败", err)
	}
	return ModsResult{Message: "模组已下载并登记", Mods: mods}, nil
}

// RemoveMod implements Control.
func (s *ControlService) RemoveMod(ctx context.Context, workshopID string) (ModsResult, error) {
	state, err := s.load(ctx)
	if err != nil {
		return ModsResult{}, err
	}
	updated := state.Clone()
	mods := ensureMods(updated)
	ids := stringSlice(mods["workshop_ids"])
	names := stringSlice(mods["mod_names"])
	for index, id := range ids {
		if id != workshopID {
			continue
		}
		ids = append(ids[:index], ids[index+1:]...)
		if index < len(names) {
			names = append(names[:index], names[index+1:]...)
		}
		break
	}
	mods["workshop_ids"] = ids
	mods["mod_names"] = names
	if err := s.deps.States.Save(ctx, updated); err != nil {
		return ModsResult{}, WrapError(CodeOperationFailed, "保存模组失败", err)
	}
	return ModsResult{Message: "模组已移除", Mods: mods}, nil
}

// Renew implements Control. This is a local state simulation, not billing.
func (s *ControlService) Renew(ctx context.Context, months int) (RenewalResult, error) {
	if months < 1 || months > 24 {
		return RenewalResult{}, NewError(CodeRenewalOutOfRange, "")
	}
	state, err := s.load(ctx)
	if err != nil {
		return RenewalResult{}, err
	}
	updated := state.Clone()
	if updated.Billing == nil {
		updated.Billing = map[string]any{}
	}
	current := intValue(updated.Billing["expire_days_left"])
	updated.Billing["expire_days_left"] = current + months*30
	updated.Billing["status"] = "ACTIVE"
	if err := s.deps.States.Save(ctx, updated); err != nil {
		return RenewalResult{}, WrapError(CodeOperationFailed, "保存续费状态失败", err)
	}
	return RenewalResult{
		Message: fmt.Sprintf("模拟续费成功，剩余天数: %d 天", current+months*30),
		Billing: updated.Billing,
	}, nil
}

// SubscribeConsole implements Control.
func (s *ControlService) SubscribeConsole(_ context.Context, replay int) (ConsoleSubscription, error) {
	if replay < 0 {
		replay = 0
	}
	inner := s.deps.Logs.Subscribe(128)
	return &consoleSubscription{inner: inner, replay: s.deps.Logs.Recent(replay)}, nil
}

type consoleSubscription struct {
	inner  ports.LogSubscription
	replay []string
}

func (c *consoleSubscription) Replay() []string      { return c.replay }
func (c *consoleSubscription) Events() <-chan string { return c.inner.Lines() }
func (c *consoleSubscription) Close()                { c.inner.Close() }

func ensureMods(state domain.InstanceState) map[string]any {
	if state.Mods == nil {
		return map[string]any{}
	}
	return state.Mods
}

func stateMap(state domain.InstanceState) map[string]any {
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
	return document
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return []string{}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func round1(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

var _ Control = (*ControlService)(nil)

// resolveInstallVersion validates the requested branch against the template's
// evidence-backed version list. An unknown branch is a validation error, never a
// silent fallback (plan D-E).
// clearInstall releases the install slot after a pre-flight failure.
func (s *ControlService) clearInstall() {
	s.mu.Lock()
	cancel := s.install.cancel
	s.install = installTask{status: "IDLE", message: "尚未运行"}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func resolveInstallVersion(template domain.Template, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		for _, version := range template.Versions {
			if version.Default {
				return version.Branch, nil
			}
		}
		return "", nil
	}
	for _, version := range template.Versions {
		if version.Branch == requested {
			return version.Branch, nil
		}
	}
	return "", NewError(CodeValidation, "所选服务端版本不在模板清单中")
}

// branchBuildID resolves the template-declared build id for the branch, or ""
// when the template does not declare one.
func branchBuildID(branch string, template domain.Template) string {
	for _, version := range template.Versions {
		if version.Branch == branch {
			return version.BuildID
		}
	}
	return ""
}

// queryGame reports the live game query fields (players, map, name) when the
// declaration target answers A2S; otherwise every field is "unavailable". The
// projection is strictly additive and never changes ready/readiness semantics.
func (s *ControlService) queryGame(ctx context.Context) map[string]any {
	result := map[string]any{"players": "unavailable", "map": "unavailable", "name": "unavailable"}
	if s.deps.Query == nil || !s.processStatus(context.Background()).Running {
		return result
	}
	state, err := s.load(ctx)
	if err != nil {
		return result
	}
	port := 0
	if value, ok := state.Ports["SERVER_PORT"]; ok {
		port = value
	}
	if port < 1 || port > 65535 {
		return result
	}
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	info, err := s.deps.Query.Query(probeCtx, "127.0.0.1", port)
	if err != nil {
		return result
	}
	result["players"] = info.Players
	result["max"] = info.Max
	result["map"] = info.Map
	result["name"] = info.Name
	return result
}

// checkCapacity runs the policy entry check; the second (pre-write) check is
// the call at the write site. A hard-limit breach surfaces as 409.
func (s *ControlService) checkCapacity(ctx context.Context) error {
	if s.deps.Capacity == nil {
		return nil
	}
	err := s.deps.Capacity.CheckBeforeWrite(ctx, s.deps.Instance)
	if err == nil {
		return nil
	}
	if errors.Is(err, errCapacityExceeded) {
		return NewError(CodeCapacityExceeded, err.Error())
	}
	return WrapError(CodeOperationFailed, "", err)
}

func (s *ControlService) capacityStatus(ctx context.Context) map[string]any {
	result := map[string]any{"state": "disabled"}
	if s.deps.Capacity == nil {
		return result
	}
	status, err := s.deps.Capacity.Status(ctx, s.deps.Instance)
	if err != nil {
		result["state"] = "unknown"
		result["message"] = err.Error()
		return result
	}
	result["state"] = status.State
	result["used_mb"] = round1(status.UsedMB)
	if status.QuotaGB > 0 {
		result["quota_gb"] = status.QuotaGB
	}
	if status.HardPercent > 0 {
		result["hard_percent"] = status.HardPercent
	}
	if status.SoftPercent > 0 {
		result["soft_percent"] = status.SoftPercent
	}
	return result
}

// ScheduleModDownload lets the composition root trigger the automatic install
// after a configuration write that changed the workshop item list.
func (s *ControlService) ScheduleModDownload(ids []string) { s.scheduleModDownload(ids) }

// scheduleModDownload installs workshop items in the background through the
// shared single in-flight gate; failures are recorded, never fatal.
func (s *ControlService) scheduleModDownload(ids []string) {
	if s.deps.Workshop == nil || len(ids) == 0 {
		return
	}
	s.modMu.Lock()
	if s.modTask.running {
		s.modTask.pending = append(s.modTask.pending, ids...)
		s.modMu.Unlock()
		return
	}
	s.modTask = modInstallTask{running: true, status: "DOWNLOADING", message: "准备下载模组"}
	s.modMu.Unlock()
	go func(queue []string) {
		for _, id := range queue {
			s.modMu.Lock()
			s.modTask.message = "正在下载模组 " + id
			s.modMu.Unlock()
			_, err := s.deps.Workshop.DownloadWorkshopItem(context.Background(), s.deps.Instance, id, func(progress ports.Progress) {
				s.modMu.Lock()
				if progress.Percent >= 0 {
					s.modTask.progress = progress.Percent
				}
				if progress.Message != "" {
					s.modTask.message = progress.Message
				}
				s.modMu.Unlock()
			})
			s.modMu.Lock()
			if err != nil {
				s.modTask.status = "FAILED"
				s.modTask.message = "模组 " + id + " 下载失败：" + err.Error()
			} else {
				s.modTask.status = "COMPLETED"
				s.modTask.progress = 100
				s.modTask.message = "模组 " + id + " 已下载"
			}
			s.modMu.Unlock()
		}
		s.modMu.Lock()
		pending := s.modTask.pending
		s.modTask.pending = nil
		s.modTask.running = false
		s.modMu.Unlock()
		if len(pending) > 0 {
			s.scheduleModDownload(pending)
		}
	}(append([]string(nil), ids...))
}
