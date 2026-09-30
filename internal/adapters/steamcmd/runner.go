// Package steamcmd implements cancellable SteamCMD installation operations.
package steamcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

var progressPattern = regexp.MustCompile(`Update state \(0x(?P<state>[0-9a-fA-F]+)\) downloading, progress:\s*(?P<percent>\d+(?:\.\d+)?)\s*\((?P<current>\d+)\s*/\s*(?P<total>\d+)\)`)

// CommandRunner starts a child process using explicit argv and returns a
// handle that the installer always waits/reaps, including cancellation paths.
type CommandRunner interface {
	Start(context.Context, string, []string, string, io.Writer) (CommandProcess, error)
}

type CommandProcess interface {
	Wait() error
	Terminate() error
	Kill() error
}

type execRunner struct{}
type execProcess struct{ cmd *exec.Cmd }

func (execRunner) Start(ctx context.Context, executable string, args []string, dir string, output io.Writer) (CommandProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command(executable, args...)
	cmd.Dir = dir
	cmd.Stdout = output
	cmd.Stderr = output
	if err := configureProcessGroup(cmd); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcess{cmd: cmd}, nil
}
func (p *execProcess) Wait() error      { return p.cmd.Wait() }
func (p *execProcess) Terminate() error { return terminateProcessTree(p.cmd.Process) }
func (p *execProcess) Kill() error      { return killProcessTree(p.cmd.Process) }

// InstallSpec is a typed description of a SteamCMD operation. Every argv
// element is assembled as a separate token; no shell or command-string parse.
type InstallSpec struct {
	Executable string
	SteamDir   string
	InstallDir string
	AppID      string
	Validate   bool
	Beta       string
	BetaPass   string
	// WorkshopID switches the command from an app update to one workshop item
	// download (M3.3). It is validated as digits before use.
	WorkshopID string
}

// InstallConfig supplies filesystem paths and app identity for the port-level
// Installer call. Paths are resolved in code, never from a shell command.
type InstallConfig struct {
	Executable  string
	SteamDir    string
	InstallPath func(domain.InstanceID) (string, error)
	AppID       string
}

// State is a safe status snapshot for the in-memory installer operation.
type State struct {
	Busy     bool
	Progress float64
	Status   string
}

// Runner implements ports.Installer and owns exactly one in-flight command.
type Runner struct {
	command CommandRunner
	config  *InstallConfig

	mu        sync.Mutex
	busy      bool
	startGate chan struct{}
	progress  float64
	status    string
}

// New constructs a fail-closed SteamCMD runner. Configure it with
// NewConfigured before using the ports.Installer method. New deliberately
// cannot infer a path or download/bootstrap SteamCMD.
func New() *Runner { return NewWithRunner(execRunner{}) }

// NewWithRunner injects a command runner for local fixtures and tests.
func NewWithRunner(command CommandRunner) *Runner {
	if command == nil {
		command = execRunner{}
	}
	return &Runner{command: command, status: "Idle", startGate: make(chan struct{}, 1)}
}

// NewConfigured constructs a port-ready SteamCMD installer without downloading
// or bootstrapping SteamCMD. Callers supply the executable and instance path.
func NewConfigured(command CommandRunner, config InstallConfig) (*Runner, error) {
	if strings.TrimSpace(config.Executable) == "" || strings.TrimSpace(config.SteamDir) == "" || config.InstallPath == nil || strings.TrimSpace(config.AppID) == "" {
		return nil, errors.New("SteamCMD executable, working directory, install path, and app id are required")
	}
	runner := NewWithRunner(command)
	runner.config = &config
	return runner, nil
}

// Snapshot returns a concurrency-safe progress snapshot.
func (r *Runner) Snapshot() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return State{Busy: r.busy, Progress: r.progress, Status: r.status}
}

// Install implements ports.Installer using the configured instance-to-path
// mapping. No network/bootstrap operation is performed by the adapter.
func (r *Runner) Install(ctx context.Context, request ports.InstallRequest, onProgress func(ports.Progress)) error {
	if r.config == nil {
		return errors.New("SteamCMD installer is not configured")
	}
	installDir, err := r.config.InstallPath(request.InstanceID)
	if err != nil {
		return fmt.Errorf("resolve install directory: %w", err)
	}
	return r.InstallSpec(ctx, InstallSpec{
		Executable: r.config.Executable,
		SteamDir:   r.config.SteamDir,
		InstallDir: installDir,
		AppID:      r.config.AppID,
		Validate:   request.Validate,
		Beta:       request.Version,
	}, nil, onProgress)
}

// InstallSpec executes the specified operation. Beta/password values are each
// passed as independent argv elements, so shell syntax is inert. If the context
// is cancelled, the process group is terminated, then killed if necessary, and
// Wait always runs to completion before busy state is cleared.
func (r *Runner) InstallSpec(ctx context.Context, spec InstallSpec, onLog func(string), onProgress func(ports.Progress)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateSpec(spec); err != nil {
		return err
	}
	r.mu.Lock()
	if r.busy {
		r.mu.Unlock()
		return errors.New("steamcmd task already running")
	}
	if r.startGate == nil {
		r.startGate = make(chan struct{}, 1)
	}
	r.busy = true
	r.progress = 0
	r.status = "Preparing SteamCMD"
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.busy = false
		r.mu.Unlock()
	}()

	args := BuildArgs(spec)
	logSafe := func(line string) {
		if onLog != nil {
			onLog(line)
		}
	}
	logSafe(fmt.Sprintf("[SteamCMD] starting app %s", spec.AppID))
	output := &progressReader{secret: spec.BetaPass, onLine: func(line string) {
		logSafe(line)
		r.consumeLine(line, onProgress)
	}}
	// Serialize the interval in which Start creates a child so cancellation
	// cannot race a context check and leave a newly spawned process unowned.
	r.startGate <- struct{}{}
	if err := ctx.Err(); err != nil {
		<-r.startGate
		r.setStatus("Cancelled")
		return err
	}
	process, err := r.command.Start(ctx, spec.Executable, args, spec.SteamDir, output)
	<-r.startGate
	if err != nil {
		r.setStatus("Failed to start SteamCMD")
		logSafe("[SteamCMD] failed to start process")
		return fmt.Errorf("start SteamCMD: %w", err)
	}

	waitDone := make(chan error, 1)
	go func() {
		err := process.Wait()
		output.Flush()
		waitDone <- err
	}()
	select {
	case err := <-waitDone:
		if err != nil {
			r.setStatus("Failed")
			logSafe("[SteamCMD] process exited with failure")
			return fmt.Errorf("SteamCMD exited unsuccessfully: %w", err)
		}
		r.setStatus("Completed")
		logSafe("[SteamCMD] completed successfully")
		return nil
	case <-ctx.Done():
		// Wait for child creation to finish before issuing termination; otherwise
		// a cancellation during Start could orphan a process.
		r.startGate <- struct{}{}
		<-r.startGate
		r.setStatus("Cancelling")
		logSafe("[SteamCMD] cancellation requested")
		_ = process.Terminate()
		timer := time.NewTimer(2 * time.Second)
		select {
		case waitErr := <-waitDone:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			if waitErr != nil && !isExpectedCancellationError(waitErr) {
				return fmt.Errorf("SteamCMD cancelled and exited with error: %w", waitErr)
			}
		case <-timer.C:
			_ = process.Kill()
			waitErr := <-waitDone
			if waitErr != nil && !isExpectedCancellationError(waitErr) {
				return fmt.Errorf("SteamCMD killed after cancellation: %w", waitErr)
			}
		}
		r.setStatus("Cancelled")
		return ctx.Err()
	}
}

func isExpectedCancellationError(err error) bool {
	if err == nil {
		return true
	}
	return errors.Is(err, os.ErrProcessDone) || strings.Contains(strings.ToLower(err.Error()), "signal: terminated") || strings.Contains(strings.ToLower(err.Error()), "killed")
}

func (r *Runner) consumeLine(line string, callback func(ports.Progress)) {
	match := progressPattern.FindStringSubmatch(line)
	if len(match) == 0 {
		if strings.Contains(line, "Success! App") {
			r.mu.Lock()
			r.progress = 100
			r.status = "Completed"
			r.mu.Unlock()
			if callback != nil {
				callback(ports.Progress{Percent: 100, Message: "Success"})
			}
		}
		return
	}
	percent, _ := strconv.ParseFloat(match[progressPattern.SubexpIndex("percent")], 64)
	r.mu.Lock()
	r.progress = percent
	r.status = fmt.Sprintf("Downloading: %.1f%%", percent)
	status := r.status
	r.mu.Unlock()
	if callback != nil {
		callback(ports.Progress{Percent: percent, Message: status})
	}
}

func (r *Runner) setStatus(status string) {
	r.mu.Lock()
	r.status = status
	r.mu.Unlock()
}

// BuildArgs returns the exact SteamCMD argv tokens. It is intentionally pure
// and testable; no token is split or interpreted by a shell.
func BuildArgs(spec InstallSpec) []string {
	args := []string{"+force_install_dir", filepath.Clean(spec.InstallDir), "+login", "anonymous"}
	if spec.WorkshopID != "" {
		// One workshop item, then exit: never an app update in the same run.
		args = append(args, "+workshop_download_item", spec.AppID, spec.WorkshopID)
		args = append(args, "+quit")
		return args
	}
	args = append(args, "+app_update", spec.AppID)
	if spec.Beta != "" {
		args = append(args, "-beta", spec.Beta)
		if spec.BetaPass != "" {
			args = append(args, "-betapassword", spec.BetaPass)
		}
	}
	if spec.Validate {
		args = append(args, "validate")
	}
	args = append(args, "+quit")
	return args
}

func validateSpec(spec InstallSpec) error {
	if strings.TrimSpace(spec.Executable) == "" || strings.ContainsRune(spec.Executable, '\x00') {
		return errors.New("SteamCMD executable is required")
	}
	if strings.TrimSpace(spec.InstallDir) == "" || strings.ContainsRune(spec.InstallDir, '\x00') || strings.ContainsRune(spec.SteamDir, '\x00') {
		return errors.New("SteamCMD install directory is required")
	}
	if strings.TrimSpace(spec.AppID) == "" {
		return errors.New("Steam app id is required")
	}
	for _, character := range spec.AppID {
		if character < '0' || character > '9' {
			return errors.New("Steam app id must contain only digits")
		}
	}
	if strings.ContainsAny(spec.Beta, "\x00\r\n") || strings.ContainsAny(spec.BetaPass, "\x00\r\n") {
		return errors.New("Steam branch credentials contain invalid control characters")
	}
	return nil
}

func redactLine(line, secret string) string {
	if secret != "" {
		line = strings.ReplaceAll(line, secret, "[REDACTED]")
	}
	return line
}

type progressReader struct {
	mu     sync.Mutex
	buffer []byte
	onLine func(string)
	secret string
}

func (r *progressReader) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buffer = append(r.buffer, data...)
	for {
		index := -1
		for i, b := range r.buffer {
			if b == '\n' {
				index = i
				break
			}
		}
		if index < 0 {
			break
		}
		line := strings.TrimSuffix(string(r.buffer[:index]), "\r")
		line = redactLine(line, r.secret)
		remaining := append([]byte(nil), r.buffer[index+1:]...)
		r.buffer = remaining
		if line != "" && r.onLine != nil {
			r.onLine(line)
		}
	}
	return len(data), nil
}

func (r *progressReader) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buffer) != 0 && r.onLine != nil {
		r.onLine(redactLine(strings.TrimSpace(string(r.buffer)), r.secret))
	}
	r.buffer = nil
}
