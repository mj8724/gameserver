package version

import "testing"

func TestStringReturnsVersion(t *testing.T) {
	if got := String(); got != Version {
		t.Fatalf("String() = %q, want %q", got, Version)
	}
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}
