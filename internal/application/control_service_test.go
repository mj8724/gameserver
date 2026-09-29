package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

type fakeStates struct {
	state domain.InstanceState
	saves int
}

func (f *fakeStates) Load(context.Context, domain.InstanceID) (domain.InstanceState, error) {
	return f.state.Clone(), nil
}

func (f *fakeStates) Save(_ context.Context, state domain.InstanceState) error {
	f.state = state.Clone()
	f.saves++
	return nil
}

type fakeProcessStatus struct {
	status ports.ProcessStatus
}

func (f *fakeProcessStatus) ProcessStatus(context.Context, domain.InstanceID) (ports.ProcessStatus, error) {
	return f.status, nil
}

type fakeProcesses struct {
	inputs   []string
	started  int
	stopped  int
	startErr error
}

func (f *fakeProcesses) Start(context.Context, ports.LaunchSpec) (ports.Process, error) {
	if f.startErr != nil {
		return ports.Process{}, f.startErr
	}
	f.started++
	return ports.Process{PID: 4242}, nil
}

func (f *fakeProcesses) Stop(context.Context, domain.InstanceID) error { f.stopped++; return nil }
func (f *fakeProcesses) Kill(context.Context, domain.InstanceID) error { return nil }

func (f *fakeProcesses) SendInput(_ context.Context, _ domain.InstanceID, text string) error {
	f.inputs = append(f.inputs, text)
	return nil
}

type fakeLogs struct {
	lines       []string
	subscribers int
	closed      int
}

func (f *fakeLogs) Recent(limit int) []string {
	if limit > len(f.lines) {
		limit = len(f.lines)
	}
	return append([]string(nil), f.lines[len(f.lines)-limit:]...)
}

func (f *fakeLogs) Subscribe(int) ports.LogSubscription {
	f.subscribers++
	return &fakeSubscription{lines: make(chan string, 4), logs: f}
}

type fakeSubscription struct {
	lines chan string
	logs  *fakeLogs
}

func (s *fakeSubscription) Lines() <-chan string { return s.lines }
func (s *fakeSubscription) Close()               { s.logs.closed++ }

type fakeInstaller struct {
	calls  int
	err    error
	result error
	block  chan struct{}
	mu     sync.Mutex
	done   chan struct{}
}

// finish closes the completion signal exactly once so tests can wait for the
// background install goroutine instead of racing it.
func (f *fakeInstaller) finish() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done == nil {
		f.done = make(chan struct{})
	}
	select {
	case <-f.done:
	default:
		close(f.done)
	}
}

func (f *fakeInstaller) Install(ctx context.Context, _ ports.InstallRequest, progress func(ports.Progress)) error {
	f.calls++
	progress(ports.Progress{Percent: 30, Message: "downloading"})
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	defer f.finish()
	if f.err != nil {
		return f.err
	}
	return f.result
}

type fakeFiles struct {
	installed   bool
	fingerprint ports.ArtifactFingerprint
	usageMB     float64
}

func (f *fakeFiles) InstanceRoot(id domain.InstanceID) (string, error) {
	return "/tmp/" + string(id), nil
}
func (f *fakeFiles) InstallDir(domain.InstanceID) (string, error) { return "/tmp/install", nil }
func (f *fakeFiles) CacheDir(domain.InstanceID) (string, error)   { return "/tmp/cache", nil }
func (f *fakeFiles) IsInstalled(domain.InstanceID) bool           { return f.installed }
func (f *fakeFiles) DiskUsageMB(domain.InstanceID) (float64, error) {
	if f.usageMB > 0 {
		return f.usageMB, nil
	}
	return 12.5, nil
}
func (f *fakeFiles) Fingerprint(domain.InstanceID) (ports.ArtifactFingerprint, error) {
	if f.fingerprint.ManifestSHA != "" {
		return f.fingerprint, nil
	}
	if !f.installed {
		return ports.ArtifactFingerprint{}, nil
	}
	return ports.ArtifactFingerprint{BuildID: "25485538", ManifestSHA: "fake", ManifestName: "appmanifest_380870.acf", TotalBytes: 1024}, nil
}

func testCatalog(t *testing.T) *fakeCatalog {
	t.Helper()
	minimum, maximum := 1.0, 64.0
	template := domain.Template{
		Summary: domain.TemplateSummary{ID: "project_zomboid", Name: "Project Zomboid", SupportedOS: []string{"windows"}},
		Variables: []domain.TemplateVariable{
			{Key: "SERVER_NAME", Label: "name", Type: "string", Default: "servertest", Required: true, UserEditable: true},
			{Key: "ADMIN_PASSWORD", Label: "admin", Type: "password", Required: true, UserEditable: true},
			{Key: "MAX_PLAYERS", Label: "players", Type: "number", Default: 16, UserEditable: true, Min: &minimum, Max: &maximum},
		},
		Ports: []domain.TemplatePort{{Key: "SERVER_PORT", Default: 16261, IsPrimary: true}},
	}
	return &fakeCatalog{templates: map[domain.TemplateID]domain.Template{"project_zomboid": template}}
}

type fakeCatalog struct {
	templates map[domain.TemplateID]domain.Template
}

func (c *fakeCatalog) List() []domain.TemplateSummary {
	summaries := make([]domain.TemplateSummary, 0, len(c.templates))
	for _, template := range c.templates {
		summaries = append(summaries, template.Summary)
	}
	return summaries
}

func (c *fakeCatalog) Get(id domain.TemplateID) (domain.Template, bool) {
	template, ok := c.templates[id]
	return template, ok
}

type harness struct {
	service   *ControlService
	states    *fakeStates
	processes *fakeProcesses
	status    *fakeProcessStatus
	logs      *fakeLogs
	files     *fakeFiles
	installer *fakeInstaller
	workshop  *fakeWorkshop
	applied   int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	states := &fakeStates{state: domain.InstanceState{
		ID:         "pz_01",
		Name:       "Project Zomboid",
		TemplateID: "project_zomboid",
		Variables:  map[string]any{"SERVER_NAME": "servertest", "ADMIN_PASSWORD": "sentinel-secret", "MAX_PLAYERS": 16},
		Ports:      map[string]int{"SERVER_PORT": 16261},
		Mods:       map[string]any{"workshop_ids": []string{}, "mod_names": []string{}},
		Billing:    map[string]any{"expire_days_left": 28, "status": "ACTIVE"},
		QuotaGB:    30,
	}}
	processes := &fakeProcesses{}
	status := &fakeProcessStatus{status: ports.ProcessStatus{Status: "STOPPED"}}
	logs := &fakeLogs{lines: []string{"boot", "ready"}}
	files := &fakeFiles{installed: true}
	installer := &fakeInstaller{}
	h := &harness{states: states, processes: processes, status: status, logs: logs, files: files, installer: installer}
	service, err := NewControlService(ServiceDeps{
		Instance:      "pz_01",
		Platform:      "windows",
		States:        states,
		Processes:     processes,
		ProcessStatus: status,
		Templates:     testCatalog(t),
		Logs:          logs,
		Files:         files,
		Clock:         &fakeClock{now: time.Unix(0, 0)},
		Installer:     installer,
		BuildLaunchSpec: func(context.Context, domain.InstanceState, ports.LaunchInput) (ports.LaunchSpec, error) {
			return ports.LaunchSpec{Executable: "ProjectZomboid64.exe"}, nil
		},
		ApplyGameConfig: func(context.Context, domain.InstanceState) error { h.applied++; return nil },
		LaunchEvidence:  LaunchEvidence{ExecutableName: "ProjectZomboid64.exe", DirectExecutable: true, Reference: "manifest#1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.service = service
	return h
}

func TestControlStatusProjectsStateAndSecrets(t *testing.T) {
	h := newHarness(t)
	response, err := h.service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response["status"] != "STOPPED" || response["running"] != false || response["is_installed"] != true {
		t.Fatalf("unexpected status: %+v", response)
	}
	if response["platform"] != "windows" || response["ready"] != false {
		t.Fatalf("unexpected platform/ready: %+v", response)
	}
	variables, ok := response["variables"].(map[string]any)
	if !ok || variables["ADMIN_PASSWORD"] != "sentinel-secret" {
		t.Fatalf("application must expose raw state; transport redacts: %+v", response["variables"])
	}
	disk, ok := response["disk"].(map[string]any)
	if !ok || disk["used_mb"] != 12.5 {
		t.Fatalf("unexpected disk projection: %+v", response["disk"])
	}
}

func TestControlInstallLifecycleAndConflicts(t *testing.T) {
	h := newHarness(t)
	accepted, err := h.service.BeginInstall(context.Background(), "")
	if err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	if accepted.Status != "INSTALLING" {
		t.Fatalf("unexpected accepted response: %+v", accepted)
	}
	if _, err := h.service.BeginInstall(context.Background(), ""); !isCode(err, CodeInstallAlreadyRunning) {
		t.Fatalf("second install error = %v, want CodeInstallAlreadyRunning", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state, _ := h.service.InstallState(context.Background())
		if state.Status == "COMPLETED" {
			if state.Progress != 100 || state.Error != nil {
				t.Fatalf("unexpected completed state: %+v", state)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if state, _ := h.service.InstallState(context.Background()); state.Status != "COMPLETED" {
		t.Fatalf("install did not complete: %+v", state)
	}

	h.status.status = ports.ProcessStatus{Running: true, Status: "RUNNING", PID: 99}
	if _, err := h.service.BeginInstall(context.Background(), ""); !isCode(err, CodeInstallWhileRunning) {
		t.Fatalf("install while running error = %v, want CodeInstallWhileRunning", err)
	}
	start, err := h.service.Start(context.Background())
	if err != nil {
		t.Fatalf("start while running: %v", err)
	}
	if !start.Running || h.processes.started != 0 {
		t.Fatalf("already-running start must not spawn: %+v started=%d", start, h.processes.started)
	}
}

func TestControlStartRequiresInstallAndEvidence(t *testing.T) {
	h := newHarness(t)
	h.files.installed = false
	if _, err := h.service.Start(context.Background()); !isCode(err, CodeServerNotInstalled) {
		t.Fatalf("start without install = %v, want CodeServerNotInstalled", err)
	}
	h.files.installed = true
	failing, err := NewControlService(ServiceDeps{
		Instance: "pz_01", Platform: "windows", States: h.states, Processes: h.processes,
		ProcessStatus: h.status, Templates: testCatalog(t), Logs: h.logs, Files: h.files,
		Clock: &fakeClock{}, BuildLaunchSpec: func(context.Context, domain.InstanceState, ports.LaunchInput) (ports.LaunchSpec, error) {
			return ports.LaunchSpec{}, errors.New("no vector")
		},
		LaunchEvidence: LaunchEvidence{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failing.Start(context.Background()); !isCode(err, CodeOperationFailed) {
		t.Fatalf("start without evidence = %v, want CodeOperationFailed", err)
	}
}

func TestControlConfigValidationAndConsoleInput(t *testing.T) {
	h := newHarness(t)
	snapshot, err := h.service.Config(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.AllowedPorts) != 1 || snapshot.AllowedPorts[0] != "SERVER_PORT" {
		t.Fatalf("unexpected allowed ports: %+v", snapshot.AllowedPorts)
	}
	for _, field := range snapshot.Fields {
		if field.Key == "ADMIN_PASSWORD" && (field.Value != nil || field.Default != nil) {
			t.Fatalf("password field must not project values: %+v", field)
		}
	}
	if _, err := h.service.UpdateConfig(context.Background(), ConfigUpdate{Variables: map[string]any{"MAX_PLAYERS": 100}}); !isCode(err, CodeValidation) {
		t.Fatalf("range violation error = %v, want CodeValidation", err)
	}
	if _, err := h.service.UpdateConfig(context.Background(), ConfigUpdate{Variables: map[string]any{"UNKNOWN": "x"}}); !isCode(err, CodeValidation) {
		t.Fatalf("unknown field error = %v, want CodeValidation", err)
	}
	result, err := h.service.UpdateConfig(context.Background(), ConfigUpdate{Variables: map[string]any{"MAX_PLAYERS": 32}})
	if err != nil {
		t.Fatalf("valid update: %v", err)
	}
	if result.Message != "配置已保存并同步" || h.states.saves != 1 || h.applied != 1 {
		t.Fatalf("unexpected update result: %+v saves=%d applied=%d", result, h.states.saves, h.applied)
	}

	running, err := h.service.SendConsoleInput(context.Background(), "say hi")
	if err != nil || running {
		t.Fatalf("stopped console input = %v/%v, want false,nil", running, err)
	}
	h.status.status = ports.ProcessStatus{Running: true, Status: "RUNNING", PID: 7}
	if running, err = h.service.SendConsoleInput(context.Background(), "say hi"); err != nil || !running {
		t.Fatalf("running console input = %v/%v, want true,nil", running, err)
	}
	if len(h.processes.inputs) != 1 || h.processes.inputs[0] != "say hi" {
		t.Fatalf("unexpected inputs: %+v", h.processes.inputs)
	}
	if _, err := h.service.SendCommand(context.Background(), "quit"); err != nil {
		t.Fatalf("command while running: %v", err)
	}
	h.status.status = ports.ProcessStatus{}
	if _, err := h.service.SendCommand(context.Background(), "quit"); !isCode(err, CodeServerNotRunning) {
		t.Fatalf("command while stopped = %v, want CodeServerNotRunning", err)
	}
}

func TestControlModsRenewAndConsoleSubscription(t *testing.T) {
	h := newHarness(t)
	name := "modA"
	if _, err := h.service.AddMod(context.Background(), AddModRequest{WorkshopID: "123", ModName: &name}); err != nil {
		t.Fatal(err)
	}
	mods := h.states.state.Mods
	if ids := stringSlice(mods["workshop_ids"]); len(ids) != 1 || ids[0] != "123" {
		t.Fatalf("unexpected mods after add: %+v", mods)
	}
	if _, err := h.service.RemoveMod(context.Background(), "123"); err != nil {
		t.Fatal(err)
	}
	if ids := stringSlice(h.states.state.Mods["workshop_ids"]); len(ids) != 0 {
		t.Fatalf("mods not removed: %+v", ids)
	}
	renewal, err := h.service.Renew(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if renewal.Billing["expire_days_left"] != 88 || renewal.Billing["status"] != "ACTIVE" {
		t.Fatalf("unexpected billing: %+v", renewal.Billing)
	}
	if _, err := h.service.Renew(context.Background(), 25); !isCode(err, CodeRenewalOutOfRange) {
		t.Fatalf("renewal range error = %v, want CodeRenewalOutOfRange", err)
	}

	subscription, err := h.service.SubscribeConsole(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(subscription.Replay()) != 1 || subscription.Replay()[0] != "ready" {
		t.Fatalf("unexpected replay: %+v", subscription.Replay())
	}
	subscription.Close()
	if h.logs.closed != 1 || h.logs.subscribers != 1 {
		t.Fatalf("subscription not cleaned: %+v", h.logs)
	}
}

func isCode(err error, code ErrorCode) bool {
	actual, ok := ErrorCodeOf(err)
	return ok && actual == code
}

func (f *fakeInstaller) wait(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	done := f.done
	f.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("installer did not finish in time")
	}
}

// M3.2: after a clean stop, an enabled automatic backup runs in the background
// and reports through the appended last_backup field, while the stop result
// itself stays untouched.
func TestAutomaticBackupAfterStopDoesNotChangeStopResult(t *testing.T) {
	h := newHarness(t)
	h.service.backupSettle = 0
	backup := &fakeBackup{enabled: true}
	h.service.deps.Backup = backup
	if _, err := h.service.Stop(context.Background()); err != nil {
		t.Fatalf("stop without backup deps must keep working: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if backup.callCount() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if backup.callCount() != 1 {
		t.Fatalf("automatic backup did not run after stop, calls=%d", backup.callCount())
	}
	status, err := h.service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lb, _ := status["last_backup"].(autoBackupState)
	if lb.State != "completed" || lb.At == "" {
		t.Fatalf("last_backup projection missing or wrong: %+v", lb)
	}
}

// A failing automatic backup never changes the stop contract and surfaces its
// failure through last_backup instead of the stop response.
func TestAutomaticBackupFailureDoesNotChangeStopResult(t *testing.T) {
	h := newHarness(t)
	h.service.backupSettle = 0
	h.service.deps.Backup = &fakeBackup{enabled: true, err: errors.New("disk full")}
	result, err := h.service.Stop(context.Background())
	if err != nil {
		t.Fatalf("stop must not fail because the backup fails: %v", err)
	}
	if !result.Success || result.Message != "服务器已停止" {
		t.Fatalf("stop contract changed: %+v", result)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.service.mu.Lock()
		lb := h.service.lastBackup
		h.service.mu.Unlock()
		if lb.State == "failed" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("backup failure was never projected into last_backup")
}

// fakeBackup simulates the automatic backup seam.
type fakeBackup struct {
	mu      sync.Mutex
	enabled bool
	err     error
	calls   int
}

func (f *fakeBackup) Enabled() bool { return f.enabled }
func (f *fakeBackup) BackupNow() (ports.BackupResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return ports.BackupResult{}, f.err
	}
	return ports.BackupResult{Path: "/tmp/backup", Files: 3, Checksum: "abc"}, nil
}

func (f *fakeBackup) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// M3.3: the legacy registration endpoint stays verbatim — DownloadMod is a
// separate path and AddMod never downloads.
func TestLegacyModsEndpointUnchanged(t *testing.T) {
	h := newHarness(t)
	h.workshop = &fakeWorkshop{contentDir: "/tmp/workshop"}
	result, err := h.service.AddMod(context.Background(), AddModRequest{WorkshopID: "281990", ModName: strptr("my mod")})
	if err != nil {
		t.Fatalf("AddMod: %v", err)
	}
	if result.Message != "模组已登记（尚未下载）" {
		t.Fatalf("legacy message changed: %q", result.Message)
	}
	if h.workshop != nil && h.workshop.calls != 0 {
		t.Fatalf("legacy registration must not download, calls=%d", h.workshop.calls)
	}
	ids := stringSlice(result.Mods["workshop_ids"])
	if len(ids) != 1 || ids[0] != "281990" {
		t.Fatalf("legacy registration shape changed: %+v", result.Mods)
	}
}

// A failed download must not leave a registered-but-missing mod behind: the
// state save happens only after the content check succeeded.
func TestModDownloadRegistersOnlyAfterSuccess(t *testing.T) {
	h := newHarness(t)
	h.workshop = &fakeWorkshop{err: errors.New("network unreachable")}
	if _, err := h.service.DownloadMod(context.Background(), "281990", nil); err == nil {
		t.Fatal("download must fail")
	}
	state, err := h.service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mods, _ := state["mods"].(map[string]any)
	if mods != nil {
		if ids := stringSlice(mods["workshop_ids"]); len(ids) != 0 {
			t.Fatalf("failed download must not register ids: %+v", ids)
		}
	}
}

// A download while an install is in flight conflicts (single in-flight slot).
func TestModDownloadConflictsWithInstall(t *testing.T) {
	h := newHarness(t)
	h.service.deps.Workshop = &fakeWorkshop{contentDir: "/tmp/x"}
	if _, err := h.service.BeginInstall(context.Background(), ""); err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	// The install is in memory as INSTALLING (single in-flight slot).
	h.service.mu.Lock()
	h.service.install.status = "INSTALLING"
	h.service.install.running = true
	h.service.mu.Unlock()
	if _, err := h.service.DownloadMod(context.Background(), "281990", nil); !isCode(err, CodeInstallAlreadyRunning) {
		t.Fatalf("download during install = %v, want CodeInstallAlreadyRunning", err)
	}
}

// Untrusted workshop content: the item path is validated as a numeric id and
// the content check only parses a structural mod.info line — nothing executes.
func TestModContentPathTraversalRejected(t *testing.T) {
	h := newHarness(t)
	h.workshop = &fakeWorkshop{}
	for _, bad := range []string{"../evil", "a/b", "-1", "1;rm"} {
		if _, err := h.service.DownloadMod(context.Background(), bad, nil); err == nil {
			t.Fatalf("path traversal id %q must be rejected", bad)
		}
	}
}

func strptr(value string) *string { return &value }

type fakeWorkshop struct {
	calls      int
	contentDir string
	err        error
}

func (f *fakeWorkshop) DownloadWorkshopItem(_ context.Context, _ domain.InstanceID, workshopID string, _ func(ports.Progress)) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.contentDir + "/" + workshopID, nil
}

// M3.4: the query fields degrade to "unavailable" when the game is unreachable
// and ready/readiness semantics stay untouched (additive projection only).
func TestQueryFieldsDegradeToUnavailable(t *testing.T) {
	h := newHarness(t)
	query := &fakeQuery{err: errors.New("no A2S")}
	h.service.deps.Query = query
	h.status.status = ports.ProcessStatus{Running: true, Status: "RUNNING", PID: 7}
	response, err := h.service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	queryFields, ok := response["game_query"].(map[string]any)
	if !ok || queryFields["players"] != "unavailable" || queryFields["map"] != "unavailable" {
		t.Fatalf("unreachable game must degrade to unavailable: %+v", response["game_query"])
	}
	if response["ready"] != false || response["readiness"] != "unknown" {
		t.Fatalf("query must not touch ready/readiness: %+v", response)
	}
	timeline, ok := response["readiness_timeline"].(map[string]any)
	if !ok {
		t.Fatalf("readiness timeline missing: %+v", response)
	}
	if _, hasSince := timeline["since"]; !hasSince {
		t.Fatalf("timeline must carry a timestamp: %+v", timeline)
	}
}

// With an answering game the query surface projects the parsed A2S fields.
func TestQueryFieldsProjectWhenA2SAnswers(t *testing.T) {
	h := newHarness(t)
	query := &fakeQuery{info: ports.GameQueryInfo{Name: "servertest", Map: "Muldraugh, KY", Players: 3, Max: 49}}
	h.service.deps.Query = query
	h.status.status = ports.ProcessStatus{Running: true, Status: "RUNNING", PID: 7}
	response, err := h.service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	queryFields := response["game_query"].(map[string]any)
	if queryFields["players"] != 3 || queryFields["name"] != "servertest" || queryFields["map"] != "Muldraugh, KY" {
		t.Fatalf("A2S fields not projected: %+v", queryFields)
	}
}

type fakeQuery struct {
	info ports.GameQueryInfo
	err  error
}

func (f *fakeQuery) Query(context.Context, string, int) (ports.GameQueryInfo, error) {
	if f.err != nil {
		return ports.GameQueryInfo{}, f.err
	}
	return f.info, nil
}
