package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
)

func TestVerifiedHAPostgresDeltaPreservesStrictNoChangeIdentity(t *testing.T) {
	delta := coreupdate.Delta{
		Installed: coreupdate.Realization{
			Kind: coreupdate.SQL, Installation: "core", Scope: "shared", Instance: "postgres-member-1",
			Owner: "baseharbor", Image: "ghcr.io/zalando/spilo-18:4.1-p1",
			Digest: "sha256:" + strings.Repeat("a", 64), Version: "18-spilo-4.1-p1",
		},
		Desired: coreupdate.Desired{
			Kind: coreupdate.SQL, Image: "ghcr.io/zalando/spilo-18:4.1-p2",
			Digest: "sha256:" + strings.Repeat("b", 64), Version: "18-spilo-4.1-p2",
		},
		Classification: coreupdate.BackupRequired,
		Reason:         "requires verified backup",
	}
	got := verifiedNativeHAPostgresDelta(delta)
	if got.Classification != coreupdate.NoChange || got.Reason != "" ||
		got.Installed.Image != got.Desired.Image || got.Installed.Digest != got.Desired.Digest ||
		got.Installed.Version != got.Desired.Version {
		t.Fatalf("native HA roll would violate strict central no-change admission: %+v", got)
	}
	if delta.Installed.Image == got.Installed.Image {
		t.Fatal("modified the original HA recovery delta")
	}
}
