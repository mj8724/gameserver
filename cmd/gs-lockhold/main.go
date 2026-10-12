// Command gs-lockhold is an acceptance helper: it acquires the instance lock and
// holds it, so a second process can be shown to be refused (HTTP 409
// "instance owned by another process") on any platform, including Windows where
// the rehearsal script's python/flock trick is unavailable.
//
// Usage:
//
//	gs-lockhold -data-root <root> -instance pz_01 -hold 20s
//
// It prints "LOCKED <pid> <path>" once the lock is held and releases it on exit.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mj8724/gameserver/internal/adapters/oslock"
	"github.com/mj8724/gameserver/internal/domain"
)

func main() {
	dataRoot := flag.String("data-root", "data", "data root containing servers/")
	instance := flag.String("instance", "pz_01", "instance id")
	hold := flag.Duration("hold", 20*time.Second, "how long to hold the lock")
	flag.Parse()

	serversRoot := filepath.Join(*dataRoot, "servers")
	manager, err := oslock.NewManager(serversRoot, "gs-lockhold")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gs-lockhold: %v\n", err)
		os.Exit(1)
	}
	handle, err := manager.AcquireInstance(domain.InstanceID(*instance), oslock.OwnerMeta{OperationID: "acceptance-lockhold"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "gs-lockhold: acquire failed: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("LOCKED %d %s\n", os.Getpid(), serversRoot)
	time.Sleep(*hold)
	if err := handle.Release(); err != nil {
		fmt.Fprintf(os.Stderr, "gs-lockhold: release failed: %v\n", err)
		os.Exit(3)
	}
	fmt.Println("RELEASED")
}
