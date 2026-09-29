package steamcmd

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

type fakeRunner struct {
	mu      sync.Mutex
	args    []string
	exec    string
	dir     string
	process *fakeProcess
	err     error
}

func (f *fakeRunner) Start(ctx context.Context, executable string, args []string, dir string, output io.Writer) (CommandProcess, error) {
	f.mu.Lock()
	f.exec = executable
	f.dir = dir
	f.args = append([]string(nil), args...)
	process := f.process
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if process.output != "" {
		_, _ = io.WriteString(output, process.output)
	}
	if process.autoFinish {
		process.finish(nil)
	}
	return process, nil
}

type fakeProcess struct {
	mu           sync.Mutex
	done         chan struct{}
	waitErr      error
	termCount    int
	killCount    int
	autoFinish   bool
	termFinishes bool
	killFinishes bool
	output       string
}

func newFakeProcess() *fakeProcess { return &fakeProcess{done: make(chan struct{})} }
func (p *fakeProcess) Wait() error { <-p.done; p.mu.Lock(); defer p.mu.Unlock(); return p.waitErr }
func (p *fakeProcess) Terminate() error {
	p.mu.Lock()
	p.termCount++
	finish := p.termFinishes
	p.mu.Unlock()
	if finish {
		p.finish(errors.New("signal: terminated"))
	}
	return nil
}
func (p *fakeProcess) Kill() error {
	p.mu.Lock()
	p.killCount++
	finish := p.killFinishes
	p.mu.Unlock()
	if finish {
		p.finish(errors.New("signal: killed"))
	}
	return nil
}
func (p *fakeProcess) finish(err error) {
	p.mu.Lock()
	select {
	case <-p.done:
		p.mu.Unlock()
		return
	default:
	}
	p.waitErr = err
	close(p.done)
	p.mu.Unlock()
}

func defaultSpec() InstallSpec {
	return InstallSpec{Executable: "/synthetic/steamcmd", SteamDir: "/synthetic", InstallDir: filepath.Join(string(filepath.Separator)+"synthetic", "server files"), AppID: "380870", Validate: true}
}

func TestBuildArgsPreservesEachTypedTokenAndAdversarialBranch(t *testing.T) {
	branch := `beta'"` + "`$; &|()<>^%!\n$(touch /tmp/SHOULD_NOT_EXIST) %VAR%" + `\\tail`
	password := "branch secret $() ;"
	spec := defaultSpec()
	spec.Beta = branch
	spec.BetaPass = password
	want := []string{"+force_install_dir", filepath.Join(string(filepath.Separator)+"synthetic", "server files"), "+login", "anonymous", "+app_update", "380870", "-beta", branch, "-betapassword", password, "validate", "+quit"}
	if got := BuildArgs(spec); !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildArgs()=%#v want %#v", got, want)
	}
}

func TestInstallRunsTypedArgvParsesProgressAndClearsBusy(t *testing.T) {
	process := newFakeProcess()
	process.output = "Update state (0x61) downloading, progress: 44.5 (44 / 100)\nSuccess! App 380870 installed\n"
	process.autoFinish = true
	fake := &fakeRunner{process: process}
	runner := NewWithRunner(fake)
	var events []ports.Progress
	var logs []string
	err := runner.InstallSpec(context.Background(), defaultSpec(), func(line string) { logs = append(logs, line) }, func(progress ports.Progress) { events = append(events, progress) })
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Percent != 44.5 || events[1].Percent != 100 {
		t.Fatalf("unexpected progress events: %#v", events)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "downloading") || strings.Contains(strings.Join(logs, "\n"), "synthetic secret") {
		t.Fatalf("unexpected logs: %#v", logs)
	}
	state := runner.Snapshot()
	if state.Busy || state.Progress != 100 || state.Status != "Completed" {
		t.Fatalf("unexpected final state: %+v", state)
	}
	if fake.exec != defaultSpec().Executable || fake.dir != defaultSpec().SteamDir || !reflect.DeepEqual(fake.args, BuildArgs(defaultSpec())) {
		t.Fatalf("typed command differs: exec=%q dir=%q args=%#v", fake.exec, fake.dir, fake.args)
	}
}

func TestInstallFailureAndConflictHaveExitStatusAndClearBusy(t *testing.T) {
	process := newFakeProcess()
	process.finish(errors.New("exit status 7"))
	process.autoFinish = true
	runner := NewWithRunner(&fakeRunner{process: process})
	if err := runner.InstallSpec(context.Background(), defaultSpec(), nil, nil); err == nil {
		t.Fatal("non-zero status should return an error")
	}
	if got := runner.Snapshot(); got.Busy || got.Status != "Failed" {
		t.Fatalf("failure did not clear busy / set status: %+v", got)
	}

	blocked := newFakeProcess()
	fake := &fakeRunner{process: blocked}
	runner = NewWithRunner(fake)
	finished := make(chan error, 1)
	go func() { finished <- runner.InstallSpec(context.Background(), defaultSpec(), nil, nil) }()
	deadline := time.Now().Add(time.Second)
	for !runner.Snapshot().Busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !runner.Snapshot().Busy {
		t.Fatal("runner never marked itself busy")
	}
	if err := runner.InstallSpec(context.Background(), defaultSpec(), nil, nil); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("concurrent task should conflict: %v", err)
	}
	blocked.finish(nil)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if runner.Snapshot().Busy {
		t.Fatal("busy flag remained after operation")
	}
}

func TestInstallCancellationTerminatesWaitsAndClearsBusy(t *testing.T) {
	process := newFakeProcess()
	process.termFinishes = true
	fake := &fakeRunner{process: process}
	runner := NewWithRunner(fake)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- runner.InstallSpec(ctx, defaultSpec(), nil, nil) }()
	deadline := time.Now().Add(time.Second)
	for !runner.Snapshot().Busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if process.termCount != 1 {
		t.Fatalf("Terminate calls=%d want 1", process.termCount)
	}
	state := runner.Snapshot()
	if state.Busy || state.Status != "Cancelled" {
		t.Fatalf("cancellation did not clear state: %+v", state)
	}
}

func TestInstallDeadlineEscalatesToKillThenReaps(t *testing.T) {
	process := newFakeProcess()
	process.killFinishes = true
	fake := &fakeRunner{process: process}
	runner := NewWithRunner(fake)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	err := runner.InstallSpec(ctx, defaultSpec(), nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if process.termCount != 1 || process.killCount != 1 {
		t.Fatalf("termination escalation term=%d kill=%d", process.termCount, process.killCount)
	}
	if state := runner.Snapshot(); state.Busy || state.Status != "Cancelled" {
		t.Fatalf("not cleared after kill/reap: %+v", state)
	}
}

func TestStartFailureAndInvalidRequestFailClosed(t *testing.T) {
	runner := NewWithRunner(&fakeRunner{err: errors.New("missing local fixture")})
	if err := runner.InstallSpec(context.Background(), defaultSpec(), nil, nil); err == nil {
		t.Fatal("start failure returned nil")
	}
	if state := runner.Snapshot(); state.Busy || state.Status != "Failed to start SteamCMD" {
		t.Fatalf("unexpected start failure state: %+v", state)
	}
	bad := defaultSpec()
	bad.AppID = "380870; touch /tmp/unsafe"
	if err := runner.InstallSpec(context.Background(), bad, nil, nil); err == nil {
		t.Fatal("invalid app id accepted")
	}
	if err := New().Install(context.Background(), ports.InstallRequest{InstanceID: domain.InstanceID("pz_01")}, nil); err == nil {
		t.Fatal("unconfigured runner must fail closed")
	}
}

func TestNewConfiguredImplementsInstallerAndMapsInstancePath(t *testing.T) {
	process := newFakeProcess()
	process.autoFinish = true
	fake := &fakeRunner{process: process}
	runner, err := NewConfigured(fake, InstallConfig{
		Executable: "/synthetic/steamcmd", SteamDir: "/synthetic", AppID: "380870",
		InstallPath: func(id domain.InstanceID) (string, error) { return "/synthetic/servers/" + string(id), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	var installer ports.Installer = runner
	if err := installer.Install(context.Background(), ports.InstallRequest{InstanceID: "pz_02", Validate: true}, nil); err != nil {
		t.Fatal(err)
	}
	want := []string{"+force_install_dir", filepath.Join(string(filepath.Separator)+"synthetic", "servers", "pz_02"), "+login", "anonymous", "+app_update", "380870", "validate", "+quit"}
	if !reflect.DeepEqual(fake.args, want) {
		t.Fatalf("args=%#v want=%#v", fake.args, want)
	}
}

type _interfaceAssertions struct{}

var _ ports.Installer = (*Runner)(nil)
