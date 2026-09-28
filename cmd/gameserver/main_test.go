package main

import (
	"testing"

	"github.com/mj8724/gameserver/internal/version"
)

func TestBuildVersionIsAvailable(t *testing.T) {
	if version.String() == "" {
		t.Fatal("build version must be set")
	}
}
