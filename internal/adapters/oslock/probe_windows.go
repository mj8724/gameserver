//go:build windows

package oslock

import "os"

// OSProbe on Windows cannot scan process command lines safely with the
// standard library. It reports liveness only; residual-process detection is
// therefore unproven and owner records keep the caller fail-closed.
type OSProbe struct{}

// Alive reports whether pid exists.
func (OSProbe) Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process != nil
}

// MatchingProcesses is not implemented on Windows.
func (OSProbe) MatchingProcesses(string) ([]int, error) { return nil, nil }
