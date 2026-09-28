//go:build !linux && !darwin && !windows

package steamcmd

import (
	"os"
	"os/exec"
)

func configureProcessGroup(*exec.Cmd) error { return nil }

// terminateProcessTree is the fallback for platforms without a
// dedicated implementation: plain signal, no tree semantics.
func terminateProcessTree(process *os.Process) error {
	if process == nil {
		return nil
	}
	return process.Signal(os.Interrupt)
}
func killProcessTree(process *os.Process) error {
	if process == nil {
		return nil
	}
	return process.Kill()
}
