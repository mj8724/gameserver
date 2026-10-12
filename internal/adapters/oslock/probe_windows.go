//go:build windows

package oslock

import "syscall"

// OSProbe on Windows cannot scan process command lines safely with the
// standard library. It reports liveness only; residual-process detection is
// therefore unproven and owner records keep the caller fail-closed.
type OSProbe struct{}

// stillActive is STILL_ACTIVE (259): the exit code a live process reports.
const stillActive = 259

// Alive reports whether pid is a running process.
//
// os.FindProcess only proves that a process object can be opened: Windows keeps
// that object alive while any handle (for example the parent shell's job
// handle) still references a terminated process, so a process killed moments
// ago can still be opened. GetExitCodeProcess distinguishes the two cases,
// otherwise `recover` would refuse to clear a stale record right after a crash
// until the handle is released.
func (OSProbe) Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	var code uint32
	if err := syscall.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}

// MatchingProcesses is not implemented on Windows.
func (OSProbe) MatchingProcesses(string) ([]int, error) { return nil, nil }
