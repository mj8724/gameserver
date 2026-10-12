package process

import (
	"strings"
	"testing"
)

// The tree-termination argv is the security-relevant part on Windows: it must
// stay a typed argument list (no shell, no concatenation), never a single
// command string.
func TestTaskkillArgsStayTypedAndExact(t *testing.T) {
	args := taskkillArgs(4242)
	want := []string{"/PID", "4242", "/T", "/F"}
	if len(args) != len(want) {
		t.Fatalf("argv = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, args[i], want[i])
		}
	}
	for _, arg := range args {
		if strings.ContainsAny(arg, "&|;><`$\"'\n") {
			t.Fatalf("argv must not carry shell metacharacters: %q", arg)
		}
	}
}
