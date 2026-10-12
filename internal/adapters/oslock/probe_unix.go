//go:build !windows

package oslock

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// OSProbe is the default process probe: liveness via signal 0 and a
// best-effort Linux /proc scan for processes that still reference the
// instance root.
type OSProbe struct{}

// Alive reports whether pid exists and is signalable.
func (OSProbe) Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// A permission error still proves the process exists.
	return strings.Contains(err.Error(), "operation not permitted") || strings.Contains(err.Error(), "permission denied")
}

// MatchingProcesses scans /proc on Linux. Other Unix platforms return an empty
// result: without that evidence the caller stays fail-closed whenever a record
// exists, and a clean start is only allowed when no record exists at all.
func (OSProbe) MatchingProcesses(root string) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, nil
	}
	var matches []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		if processReferencesRoot(pid, root) {
			matches = append(matches, pid)
		}
	}
	return matches, nil
}

func processReferencesRoot(pid int, root string) bool {
	commandLine, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err == nil && len(commandLine) > 0 && strings.Contains(string(commandLine), root) {
		return true
	}
	workingDir, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
	if err == nil && strings.HasPrefix(workingDir, root) {
		return true
	}
	return false
}
