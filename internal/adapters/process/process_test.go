package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mj8724/gameserver/internal/ports"
)

// TestHelperProcess is invoked as a short-lived local child process by the
// supervisor tests. It never accesses the network or repository data/.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GAMESERVER_PROCESS_HELPER") != "1" {
		return
	}
	mode := os.Getenv("GAMESERVER_PROCESS_HELPER_MODE")
	switch mode {
	case "fast-exit":
		fmt.Println("helper fast exit")
		os.Exit(17)
	case "print-args":
		fmt.Println(strings.Join(os.Args, " "))
		fmt.Println("visible secret sentinel from child")
		for {
			line, err := readInputLine()
			if err != nil || line == "quit" {
				return
			}
		}
	case "capture-args":
		path := os.Getenv("GAMESERVER_PROCESS_ARG_FILE")
		encoded, err := json.Marshal(os.Args)
		if err != nil || path == "" || os.WriteFile(path, encoded, 0o600) != nil {
			os.Exit(21)
		}
		for {
			line, err := readInputLine()
			if err != nil || line == "quit" {
				return
			}
		}
	case "echo":
		fmt.Println("READY")
		for {
			line, err := readInputLine()
			if err != nil {
				return
			}
			fmt.Println("INPUT:" + line)
			if line == "quit" {
				return
			}
		}
	case "lines":
		for i := 0; i < 1105; i++ {
			fmt.Printf("line-%04d\n", i)
		}
		for {
			line, err := readInputLine()
			if err != nil || line == "quit" {
				return
			}
		}
	case "ignore-quit":
		// Stay alive even after the fixed graceful "quit" line. The parent must
		// enforce its bounded timeout and kill/reap this helper.
		for {
			time.Sleep(time.Second)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown helper mode")
		os.Exit(19)
	}
}

func readInputLine() (string, error) {
	var input string
	for {
		var one [1]byte
		n, err := os.Stdin.Read(one[:])
		if n != 0 {
			if one[0] == '\n' {
				return strings.TrimSuffix(input, "\r"), nil
			}
			input += string(one[0])
		}
		if err != nil {
			return input, err
		}
	}
}

func helperSpec(mode string, args ...string) ports.LaunchSpec {
	allArgs := []string{"-test.run=^TestHelperProcess$", "--"}
	allArgs = append(allArgs, args...)
	return ports.LaunchSpec{
		Executable: os.Args[0],
		Args:       allArgs,
		WorkDir:    filepath.Dir(os.Args[0]),
		Env: map[string]string{
			"GAMESERVER_PROCESS_HELPER":      "1",
			"GAMESERVER_PROCESS_HELPER_MODE": mode,
		},
	}
}

func waitForLog(t *testing.T, supervisor *Supervisor, needle string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, line := range supervisor.Logs(maxLogLines) {
			if strings.Contains(line, needle) {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("log %q not observed; logs=%#v", needle, supervisor.Logs(30))
}

func waitForStopped(t *testing.T, supervisor *Supervisor, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, _, running := supervisor.Status(); !running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("process did not stop before timeout")
}

func TestStartFailureAndFastExitDoNotReportSuccess(t *testing.T) {
	supervisor := NewForInstanceWithOptions("pz_01", Options{StartupGrace: 30 * time.Millisecond, StopTimeout: 50 * time.Millisecond})
	_, err := supervisor.Start(context.Background(), ports.LaunchSpec{Executable: filepath.Join(t.TempDir(), "missing"), WorkDir: t.TempDir()})
	if err == nil {
		t.Fatal("missing executable reported start success")
	}
	if status, pid, running := supervisor.Status(); status != "CRASHED" || pid != 0 || running {
		t.Fatalf("start failure status=(%s,%d,%v)", status, pid, running)
	}
	_, err = supervisor.Start(context.Background(), helperSpec("fast-exit"))
	if err == nil || !strings.Contains(err.Error(), "exited during startup") {
		t.Fatalf("fast exit must not report started: %v", err)
	}
	if _, _, running := supervisor.Status(); running {
		t.Fatal("fast-exit process is still reported running")
	}
}

func TestInputGracefulStopAndIdempotentKillReapChild(t *testing.T) {
	supervisor := NewForInstanceWithOptions("pz_01", Options{StartupGrace: 40 * time.Millisecond, StopTimeout: 250 * time.Millisecond})
	proc, err := supervisor.Start(context.Background(), helperSpec("echo"))
	if err != nil {
		t.Fatal(err)
	}
	if proc.PID <= 0 {
		t.Fatalf("invalid child pid %d", proc.PID)
	}
	waitForLog(t, supervisor, "READY", time.Second)
	if err := supervisor.SendInput(context.Background(), "pz_01", "hello console"); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, supervisor, "INPUT:hello console", time.Second)
	if err := supervisor.Stop(context.Background(), "pz_01"); err != nil {
		t.Fatal(err)
	}
	waitForStopped(t, supervisor, time.Second)
	if err := supervisor.Kill(context.Background(), "pz_01"); err != nil {
		t.Fatalf("kill should be idempotent after stop: %v", err)
	}
	if status, pid, running := supervisor.Status(); status != "STOPPED" || pid != 0 || running {
		t.Fatalf("post-stop state=(%s,%d,%v)", status, pid, running)
	}
}

func TestLogBufferIsBoundedToOneThousandNewestLines(t *testing.T) {
	supervisor := NewForInstanceWithOptions("pz_01", Options{StartupGrace: 100 * time.Millisecond, StopTimeout: time.Second})
	if _, err := supervisor.Start(context.Background(), helperSpec("lines")); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, supervisor, "line-1104", 2*time.Second)
	logs := supervisor.Logs(5000)
	if len(logs) != maxLogLines {
		t.Fatalf("log buffer length=%d want %d", len(logs), maxLogLines)
	}
	if strings.Contains(strings.Join(logs, "\n"), "line-0000") || !strings.Contains(strings.Join(logs, "\n"), "line-1104") {
		t.Fatal("log ring did not retain newest output")
	}
	if err := supervisor.Stop(context.Background(), "pz_01"); err != nil {
		t.Fatal(err)
	}
}

func TestStopTimeoutKillsAndReapsIgnoringHelper(t *testing.T) {
	supervisor := NewForInstanceWithOptions("pz_01", Options{StartupGrace: 40 * time.Millisecond, StopTimeout: 60 * time.Millisecond})
	proc, err := supervisor.Start(context.Background(), helperSpec("ignore-quit"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := supervisor.Stop(ctx, "pz_01"); err != nil {
		t.Fatalf("timeout stop should kill and reap: %v", err)
	}
	waitForStopped(t, supervisor, time.Second)
	if err := supervisor.Kill(context.Background(), "pz_01"); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.SendInput(context.Background(), "pz_01", "later"); err == nil {
		t.Fatal("input after reaped process should fail")
	}
	if proc.PID <= 0 {
		t.Fatal("missing helper pid")
	}
}

func TestAdversarialArgvReachesHelperWithoutSplittingAndIsRedacted(t *testing.T) {
	const secret = "s'\"`$; &|()<>^%!$(touch /tmp/SHOULD_NOT_EXIST) %VAR%"
	argFile := filepath.Join(t.TempDir(), "argv.json")
	spec := helperSpec("capture-args", "-servername=server with spaces", "-adminpassword="+secret, "literal {{ .Variables.ADMIN_PASSWORD }}")
	spec.Env["GAMESERVER_PROCESS_ARG_FILE"] = argFile
	supervisor := NewForInstanceWithOptions("pz_01", Options{StartupGrace: 40 * time.Millisecond, StopTimeout: 100 * time.Millisecond})
	if _, err := supervisor.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	var got []string
	for time.Now().Before(deadline) {
		encoded, err := os.ReadFile(argFile)
		if err == nil {
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got == nil {
		t.Fatal("helper did not persist received argv")
	}
	wantTail := []string{"-servername=server with spaces", "-adminpassword=" + secret, "literal {{ .Variables.ADMIN_PASSWORD }}"}
	if len(got) < len(wantTail) || !equalStrings(got[len(got)-len(wantTail):], wantTail) {
		t.Fatalf("child argv tail=%#v, want %#v", got, wantTail)
	}
	if err := supervisor.Stop(context.Background(), "pz_01"); err != nil {
		t.Fatal(err)
	}
	logs := strings.Join(supervisor.Logs(maxLogLines), "\n")
	if strings.Contains(logs, secret) || !strings.Contains(logs, "-adminpassword=[REDACTED]") {
		t.Fatalf("secret not safely redacted from logs: %q", logs)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestArgumentAndChildOutputLogsRedactAdminPassword(t *testing.T) {
	const secret = "sentinel-secret-123"
	supervisor := NewForInstanceWithOptions("pz_01", Options{StartupGrace: 40 * time.Millisecond, StopTimeout: 100 * time.Millisecond})
	_, err := supervisor.Start(context.Background(), helperSpec("print-args", "-adminpassword="+secret))
	if err != nil {
		t.Fatal(err)
	}
	waitForLog(t, supervisor, "visible secret", time.Second)
	if err := supervisor.Stop(context.Background(), "pz_01"); err != nil {
		t.Fatal(err)
	}
	allLogs := strings.Join(supervisor.Logs(maxLogLines), "\n")
	if strings.Contains(allLogs, secret) {
		t.Fatalf("secret leaked in process log: %s", allLogs)
	}
	if !strings.Contains(allLogs, "-adminpassword=[REDACTED]") || !strings.Contains(allLogs, "[REDACTED]") {
		t.Fatalf("expected redacted process fingerprint/output, got %q", allLogs)
	}
}

func TestWrongInstanceAndCancelledStartAreRejected(t *testing.T) {
	supervisor := NewForInstanceWithOptions("pz_01", Options{StartupGrace: 100 * time.Millisecond, StopTimeout: 100 * time.Millisecond})
	_, err := supervisor.Start(context.Background(), helperSpec("echo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := supervisor.SendInput(context.Background(), "other", "hello"); err == nil {
		t.Fatal("wrong instance input accepted")
	}
	if err := supervisor.Kill(context.Background(), "pz_01"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = supervisor.Start(ctx, helperSpec("echo"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled start should fail with context error, got %v", err)
	}
}

func TestLogLimitIsClampedAndStatusDoesNotExposeSecrets(t *testing.T) {
	supervisor := NewForInstance("pz_01")
	for i := 0; i < 1100; i++ {
		supervisor.appendLog(strconv.Itoa(i))
	}
	if len(supervisor.Logs(999999)) != 1000 {
		t.Fatal("logs limit is not capped at 1000")
	}
	if len(supervisor.Logs(-1)) != 0 {
		t.Fatal("non-positive limit should return empty log list")
	}
}
