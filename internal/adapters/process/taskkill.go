package process

import "strconv"

// taskkillArgs builds the argv used to terminate a Windows process tree.
//
// It lives in an untagged file on purpose: the command shape is the security
// relevant part (typed argv, no shell, no string concatenation) and is unit
// tested on every platform, while the Windows-only glue that executes it stays
// a thin wrapper.
func taskkillArgs(pid int) []string {
	return []string{"/PID", strconv.Itoa(pid), "/T", "/F"}
}
