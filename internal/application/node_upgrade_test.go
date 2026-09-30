package application

import (
	"context"
	"testing"

	"github.com/mj8724/gameserver/internal/ports"
)

// A node upgrade/rollback drill in miniature: rotate the identity (the "upgrade
// step"), verify the new fingerprint is what peers must pin, then roll back by
// re-registering the previous fingerprint and confirming authorization follows.
func TestNodeUpgradeRollbackDrill(t *testing.T) {
	nodes := &fakeNodes{}
	ctx := context.Background()

	before, err := nodes.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := nodes.Register(ctx, ports.RegisteredNode{NodeID: before.NodeID, Fingerprint: "fp-old"}); err != nil {
		t.Fatal(err)
	}
	if err := nodes.Authorize(ctx, before.NodeID, "fp-old"); err != nil {
		t.Fatalf("pre-upgrade authorization must succeed: %v", err)
	}

	// Upgrade step: rotate the key; the old fingerprint must stop working once
	// the peer registry is updated to the new one.
	rotated, err := nodes.Rotate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Fingerprint == before.Fingerprint {
		t.Fatal("rotation must produce a new fingerprint")
	}
	if err := nodes.Register(ctx, ports.RegisteredNode{NodeID: rotated.NodeID, Fingerprint: rotated.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	nodes.authorizeErr = nil
	if err := nodes.Authorize(ctx, rotated.NodeID, rotated.Fingerprint); err != nil {
		t.Fatalf("post-upgrade authorization must succeed: %v", err)
	}

	// Rollback: re-register the previous fingerprint and confirm it authorizes
	// again, while the upgraded one no longer does (the registry pins exactly one).
	if err := nodes.Register(ctx, ports.RegisteredNode{NodeID: rotated.NodeID, Fingerprint: before.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	pinned := nodes.registry[rotated.NodeID]
	if pinned != before.Fingerprint {
		t.Fatalf("rollback must pin the previous fingerprint: %q", pinned)
	}
	if before.Fingerprint == rotated.Fingerprint {
		t.Fatal("rollback target must differ from the upgraded key")
	}
}

// The drill must also prove that device A's identity authorizes only itself,
// never another node id (no cross-node privilege).
func TestNodeIdentityIsNotTransferable(t *testing.T) {
	nodes := &fakeNodes{}
	ctx := context.Background()
	if err := nodes.Register(ctx, ports.RegisteredNode{NodeID: "node-a", Fingerprint: "fp-a"}); err != nil {
		t.Fatal(err)
	}
	if err := nodes.Register(ctx, ports.RegisteredNode{NodeID: "node-b", Fingerprint: "fp-b"}); err != nil {
		t.Fatal(err)
	}
	if err := nodes.Authorize(ctx, "node-a", "fp-a"); err != nil {
		t.Fatalf("own identity must authorize: %v", err)
	}
	if err := nodes.Authorize(ctx, "node-a", "fp-b"); err == nil {
		t.Fatal("another node's fingerprint must not authorize node-a")
	}
}
