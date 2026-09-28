//go:build windows

package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// creationFlagsNewProcessGroup mirrors windows.CREATE_NEW_PROCESS_GROUP. It is
// spelled out to keep the dependency surface at syscall only (ADR §1.1).
const creationFlagsNewProcessGroup = 0x00000200

func configureProcessGroup(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= creationFlagsNewProcessGroup
	return nil
}

// terminateProcessTree and killProcessTree both terminate the whole tree:
// Windows has no SIGTERM equivalent, so the graceful path is the console
// command sent over stdin, and any timeout escalation must remove the children
// too (PZ and SteamCMD both spawn them). taskkill /T /F is the supported,
// dependency-free way to do that; process.Kill is the last resort.
func terminateProcessTree(process *os.Process) error {
	return stopWindowsTree(process)
}

func killProcessTree(process *os.Process) error {
	return stopWindowsTree(process)
}

func stopWindowsTree(process *os.Process) error {
	if process == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// argv is built by a pure, unit-tested helper: typed arguments, no shell.
	command := exec.CommandContext(ctx, "taskkill", taskkillArgs(process.Pid)...)
	err := command.Run()
	if err == nil {
		return nil
	}
	// A tree that is already gone is the desired end state.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
		return nil
	}
	if killErr := process.Kill(); killErr == nil {
		return nil
	}
	return err
}
