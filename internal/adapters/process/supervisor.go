// Package process supervises local game server processes with typed argv.
package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

const (
	maxLogLines     = 1000
	maxLogLineBytes = 64 * 1024
)

var adminPasswordArgument = regexp.MustCompile(`(?i)-adminpassword(?:=|\s+)(?:"([^"]*)"|'([^']*)'|(\S+))`)

// Options provides bounded timing knobs for deterministic process tests. A
// zero value uses production defaults.
type Options struct {
	StartupGrace time.Duration
	StopTimeout  time.Duration
}

// Supervisor owns one locally started process. It never invokes a shell and
// never parses template command strings.
type Supervisor struct {
	mu            sync.Mutex
	boundInstance domain.InstanceID
	starting      bool
	options       Options
	process       *ownedProcess
	status        string
	logs          []string
	listeners     map[uint64]func(string)
	nextListener  uint64
}

type ownedProcess struct {
	instance domain.InstanceID
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	writer   *lineWriter
	pid      int
	done     chan struct{}
	waitErr  error
	exitCode int
	secrets  []string
	started  time.Time
}

// New constructs a supervisor with ADR defaults (1000-line memory log and
// bounded 25-second graceful stop).
func New() *Supervisor { return NewWithOptions(Options{}) }

// NewWithOptions constructs a supervisor with injectable timing bounds.
func NewWithOptions(options Options) *Supervisor {
	if options.StartupGrace <= 0 {
		options.StartupGrace = 30 * time.Millisecond
	}
	if options.StopTimeout <= 0 {
		options.StopTimeout = 25 * time.Second
	}
	return &Supervisor{options: options, status: "STOPPED", listeners: make(map[uint64]func(string))}
}

// NewForInstance binds this supervisor to the one instance it controls. This
// adapts the current ProcessSupervisor.Start signature, which carries no ID.
func NewForInstance(instance domain.InstanceID) *Supervisor {
	return NewForInstanceWithOptions(instance, Options{})
}

// NewForInstanceWithOptions is NewForInstance with lifecycle timing controls.
func NewForInstanceWithOptions(instance domain.InstanceID, options Options) *Supervisor {
	supervisor := NewWithOptions(options)
	supervisor.boundInstance = instance
	return supervisor
}

// Start implements ports.ProcessSupervisor. Create success is accepted only if
// the child remains alive through a short startup grace and request context.
func (s *Supervisor) Start(ctx context.Context, spec ports.LaunchSpec) (ports.Process, error) {
	if s.boundInstance == "" {
		return ports.Process{}, errors.New("process supervisor must be bound to an instance")
	}
	return s.StartForInstance(ctx, s.boundInstance, spec)
}

// StartForInstance is the instance-aware form recommended for the ports
// contract; it prevents starting a child under an implicit ownership identity.
func (s *Supervisor) StartForInstance(ctx context.Context, instance domain.InstanceID, spec ports.LaunchSpec) (ports.Process, error) {
	if err := ctx.Err(); err != nil {
		return ports.Process{}, err
	}
	if instance == "" {
		return ports.Process{}, errors.New("instance id is required")
	}
	if err := validateLaunchSpec(spec); err != nil {
		return ports.Process{}, err
	}
	s.mu.Lock()
	if s.starting || (s.process != nil && isAlive(s.process)) {
		s.mu.Unlock()
		return ports.Process{}, errors.New("a process is already running or starting")
	}
	s.starting = true
	s.status = "STARTING"
	s.process = nil
	s.mu.Unlock()
	startFailed := true
	defer func() {
		if startFailed {
			s.mu.Lock()
			s.starting = false
			if s.process == nil {
				s.status = "CRASHED"
			}
			s.mu.Unlock()
		}
	}()

	secrets := secretsFromArgs(spec.Args)
	redacted := redactArgs(spec.Args, secrets)
	writer := &lineWriter{emit: func(line string) { s.appendLog(redactText(line, secrets)) }}
	cmd := exec.Command(spec.Executable, spec.Args...)
	cmd.Dir = spec.WorkDir
	if len(spec.Env) != 0 {
		cmd.Env = mergeEnv(os.Environ(), spec.Env)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		s.setStatus("CRASHED")
		return ports.Process{}, fmt.Errorf("create process input: %w", err)
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := configureProcessGroup(cmd); err != nil {
		_ = stdin.Close()
		s.setStatus("CRASHED")
		return ports.Process{}, fmt.Errorf("configure process group: %w", err)
	}
	s.appendLog("[Supervisor] exec " + quoteArgs(spec.Executable, redacted))
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		s.setStatus("CRASHED")
		s.appendLog("[Supervisor] process start failed")
		return ports.Process{}, fmt.Errorf("start process: %w", err)
	}
	owned := &ownedProcess{
		instance: instance,
		cmd:      cmd,
		stdin:    stdin,
		writer:   writer,
		pid:      cmd.Process.Pid,
		done:     make(chan struct{}),
		secrets:  secrets,
		started:  time.Now(),
	}
	s.mu.Lock()
	s.process = owned
	s.status = "STARTING"
	s.starting = false
	s.mu.Unlock()
	startFailed = false
	go s.waitFor(owned)

	timer := time.NewTimer(s.options.StartupGrace)
	defer timer.Stop()
	select {
	case <-owned.done:
		s.setStatus("CRASHED")
		return ports.Process{}, fmt.Errorf("process exited during startup (exit code %d)", owned.exitCode)
	case <-ctx.Done():
		_ = killProcessTree(cmd.Process)
		<-owned.done
		s.setStatus("CRASHED")
		return ports.Process{}, ctx.Err()
	case <-timer.C:
	}
	if !isAlive(owned) {
		<-owned.done
		s.setStatus("CRASHED")
		return ports.Process{}, fmt.Errorf("process exited during startup (exit code %d)", owned.exitCode)
	}
	s.setStatus("RUNNING")
	s.appendLog(fmt.Sprintf("[Supervisor] process started (pid %d)", owned.pid))
	return ports.Process{PID: owned.pid}, nil
}

// Stop implements ports.ProcessSupervisor. It sends the fixed in-band PZ
// command, waits to the earlier of the caller deadline and StopTimeout, then
// kills the entire process group and waits for reap.
func (s *Supervisor) Stop(ctx context.Context, instance domain.InstanceID) error {
	owned, err := s.ownedFor(instance)
	if err != nil {
		return err
	}
	select {
	case <-owned.done:
		s.setStatus("STOPPED")
		return nil
	default:
	}
	s.setStatus("STOPPING")
	if err := writeInput(owned, "quit"); err != nil {
		// Graceful input can fail when the child closed stdin; continue into wait
		// rather than treating an already-stopping process as a hard failure.
		s.appendLog("[Supervisor] graceful stop input unavailable")
	}

	timeout := s.options.StopTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}
	if timeout < 0 {
		timeout = 0
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-owned.done:
		s.setStatus("STOPPED")
		return nil
	case <-ctx.Done():
		return s.forceAndReap(context.Background(), owned)
	case <-timer.C:
		s.appendLog("[Supervisor] graceful stop timed out; killing process tree")
		return s.forceAndReap(context.Background(), owned)
	}
}

// Kill implements ports.ProcessSupervisor; repeated calls are idempotent and
// successful only after the process is reaped.
func (s *Supervisor) Kill(ctx context.Context, instance domain.InstanceID) error {
	owned, err := s.ownedFor(instance)
	if err != nil {
		if errors.Is(err, errNoOwnedProcess) {
			s.setStatus("STOPPED")
			return nil
		}
		return err
	}
	return s.forceAndReap(ctx, owned)
}

// SendInput writes one console line if the owned process is alive.
func (s *Supervisor) SendInput(ctx context.Context, instance domain.InstanceID, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owned, err := s.ownedFor(instance)
	if err != nil {
		return err
	}
	select {
	case <-owned.done:
		return errors.New("server process is not running")
	default:
	}
	if err := writeInput(owned, text); err != nil {
		return err
	}
	s.appendLog("> " + redactText(strings.TrimSpace(text), owned.secrets))
	return nil
}

// Logs returns the most recent lines (at most 1000), oldest first.
func (s *Supervisor) Logs(limit int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit < 1 || len(s.logs) == 0 {
		return []string{}
	}
	if limit > maxLogLines {
		limit = maxLogLines
	}
	if limit > len(s.logs) {
		limit = len(s.logs)
	}
	return append([]string(nil), s.logs[len(s.logs)-limit:]...)
}

// Status returns the current lifecycle status and process PID if available.
func (s *Supervisor) Status() (string, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.process == nil {
		return s.status, 0, false
	}
	select {
	case <-s.process.done:
		return s.status, 0, false
	default:
		return s.status, s.process.pid, true
	}
}

// AddListener adds a callback for newly appended log lines and returns an
// idempotent remover.
func (s *Supervisor) AddListener(listener func(string)) func() {
	if listener == nil {
		return func() {}
	}
	s.mu.Lock()
	s.nextListener++
	id := s.nextListener
	s.listeners[id] = listener
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { s.mu.Lock(); delete(s.listeners, id); s.mu.Unlock() }) }
}

func (s *Supervisor) waitFor(owned *ownedProcess) {
	err := owned.cmd.Wait()
	owned.writer.Flush()
	_ = owned.stdin.Close()
	owned.waitErr = err
	if err == nil {
		owned.exitCode = 0
	} else {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			owned.exitCode = exitErr.ExitCode()
		} else {
			owned.exitCode = -1
		}
	}
	close(owned.done)

	s.mu.Lock()
	if s.process == owned {
		if s.status != "STOPPING" {
			if owned.exitCode == 0 {
				s.status = "STOPPED"
			} else {
				s.status = "CRASHED"
			}
		}
	}
	s.mu.Unlock()
	s.appendLog(fmt.Sprintf("[Supervisor] process exited (code %d)", owned.exitCode))
}

func (s *Supervisor) forceAndReap(_ context.Context, owned *ownedProcess) error {
	select {
	case <-owned.done:
		s.setStatus("STOPPED")
		return nil
	default:
	}
	s.setStatus("STOPPING")
	if err := killProcessTree(owned.cmd.Process); err != nil {
		s.setStatus("CRASHED")
		return fmt.Errorf("kill process tree: %w", err)
	}
	// Go's signal helper test process may have inherited SIGTERM=ignored; the
	// implementation sends SIGKILL here. Wait must still finish and reap.
	<-owned.done
	s.setStatus("STOPPED")
	return nil
}

var errNoOwnedProcess = errors.New("no process is owned by this supervisor")

func (s *Supervisor) ownedFor(instance domain.InstanceID) (*ownedProcess, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.process == nil {
		return nil, errNoOwnedProcess
	}
	if s.process.instance != instance {
		return nil, errors.New("instance does not match owned process")
	}
	return s.process, nil
}

func (s *Supervisor) appendLog(line string) {
	s.mu.Lock()
	if len(s.logs) == maxLogLines {
		copy(s.logs, s.logs[1:])
		s.logs[maxLogLines-1] = line
	} else {
		s.logs = append(s.logs, line)
	}
	listeners := make([]func(string), 0, len(s.listeners))
	for _, listener := range s.listeners {
		listeners = append(listeners, listener)
	}
	s.mu.Unlock()
	for _, listener := range listeners {
		func() {
			defer func() { _ = recover() }()
			listener(line)
		}()
	}
}

func (s *Supervisor) setStatus(status string) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

func isAlive(owned *ownedProcess) bool {
	select {
	case <-owned.done:
		return false
	default:
		return true
	}
}

func writeInput(owned *ownedProcess, input string) error {
	text := strings.TrimSpace(input)
	if strings.ContainsAny(text, "\x00\r\n") {
		return errors.New("console input must contain one line")
	}
	if _, err := io.WriteString(owned.stdin, text+"\n"); err != nil {
		return fmt.Errorf("write console input: %w", err)
	}
	return nil
}

func validateLaunchSpec(spec ports.LaunchSpec) error {
	if strings.TrimSpace(spec.Executable) == "" || strings.ContainsRune(spec.Executable, '\x00') {
		return errors.New("executable is required")
	}
	if strings.TrimSpace(spec.WorkDir) == "" || strings.ContainsRune(spec.WorkDir, '\x00') {
		return errors.New("working directory is required")
	}
	for _, arg := range spec.Args {
		if strings.ContainsRune(arg, '\x00') {
			return errors.New("argument contains NUL")
		}
	}
	for key, value := range spec.Env {
		if key == "" || strings.ContainsAny(key, "=\x00\r\n") || strings.ContainsRune(value, '\x00') {
			return errors.New("invalid process environment entry")
		}
	}
	return nil
}

func mergeEnv(current []string, overrides map[string]string) []string {
	merged := make(map[string]string, len(current)+len(overrides))
	for _, item := range current {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			merged[key] = value
		}
	}
	for key, value := range overrides {
		merged[key] = value
	}
	out := make([]string, 0, len(merged))
	for key, value := range merged {
		out = append(out, key+"="+value)
	}
	return out
}

func secretsFromArgs(args []string) []string {
	secrets := make([]string, 0, 1)
	for i, arg := range args {
		lower := strings.ToLower(arg)
		if lower == "-adminpassword" && i+1 < len(args) && args[i+1] != "" {
			secrets = append(secrets, args[i+1])
		} else if strings.HasPrefix(lower, "-adminpassword=") {
			value := arg[strings.IndexByte(arg, '=')+1:]
			if value != "" {
				secrets = append(secrets, value)
			}
		}
	}
	return secrets
}

func redactArgs(args, secrets []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		lower := strings.ToLower(arg)
		if lower == "-adminpassword" {
			out[i] = "-adminpassword"
			if i+1 < len(args) {
				out[i+1] = "[REDACTED]"
			}
		} else if strings.HasPrefix(lower, "-adminpassword=") {
			out[i] = "-adminpassword=[REDACTED]"
		} else {
			out[i] = redactText(arg, secrets)
		}
	}
	return out
}

func redactText(text string, secrets []string) string {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, "[REDACTED]")
		// Output is line-buffered, so an argv secret containing newline bytes
		// may be split across log lines. Redact segments only when they form a
		// unique contiguous portion of the full sensitive value.
		segments := strings.FieldsFunc(secret, func(r rune) bool { return r == '\r' || r == '\n' })
		for _, segment := range segments {
			if len(segment) >= 8 && strings.Contains(secret, segment) {
				text = strings.ReplaceAll(text, segment, "[REDACTED]")
			}
		}
	}
	return adminPasswordArgument.ReplaceAllString(text, "-adminpassword=[REDACTED]")
}

func quoteArgs(executable string, args []string) string {
	all := append([]string{executable}, args...)
	quoted := make([]string, len(all))
	for i, arg := range all {
		quoted[i] = strconvQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func strconvQuote(value string) string {
	return fmt.Sprintf("%q", value)
}

type lineWriter struct {
	mu     sync.Mutex
	buffer []byte
	emit   func(string)
}

func (w *lineWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buffer = append(w.buffer, data...)
	for {
		index := -1
		for i, b := range w.buffer {
			if b == '\n' {
				index = i
				break
			}
		}
		if index < 0 {
			if len(w.buffer) > maxLogLineBytes {
				w.buffer = append([]byte(nil), w.buffer[len(w.buffer)-maxLogLineBytes:]...)
			}
			break
		}
		lineBytes := w.buffer[:index]
		if len(lineBytes) > maxLogLineBytes {
			lineBytes = lineBytes[len(lineBytes)-maxLogLineBytes:]
		}
		line := strings.TrimSuffix(string(lineBytes), "\r")
		w.buffer = append([]byte(nil), w.buffer[index+1:]...)
		if line != "" && w.emit != nil {
			w.emit(line)
		}
	}
	return len(data), nil
}

func (w *lineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buffer) != 0 && w.emit != nil {
		w.emit(strings.TrimSuffix(string(w.buffer), "\r"))
	}
	w.buffer = nil
}

var _ ports.ProcessSupervisor = (*Supervisor)(nil)
