//go:build !linux && !darwin

package process

import (
	"os"
	"os/exec"
)

func configureProcessGroup(*exec.Cmd) error { return nil }
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
