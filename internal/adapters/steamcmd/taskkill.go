package steamcmd

import "strconv"

// taskkillArgs builds the argv used to terminate a Windows process tree.
// Untagged so the typed-argv shape is unit tested on every platform; the
// Windows-only glue that executes it stays a thin wrapper.
func taskkillArgs(pid int) []string {
	return []string{"/PID", strconv.Itoa(pid), "/T", "/F"}
}
