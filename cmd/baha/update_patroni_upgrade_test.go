package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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

func TestOwnedCoreHAPostgresRollRejectsForeignOrWrongInstallationBeforeRuntime(t *testing.T) {
	delta := coreupdate.Delta{
		Installed: coreupdate.Realization{Kind: coreupdate.SQL, Installation: "core-1", Scope: "shared", Instance: "postgres-member-1", Owner: "foreign"},
		Desired: coreupdate.Desired{Kind: coreupdate.SQL},
		Classification: coreupdate.BackupRequired,
	}
	files := bhruntime.Files{HA: true, Project: "core", Compose: "/tmp/core.yaml"}
	if err := rollOwnedCoreHAPostgres(context.Background(), nil, files, delta, "core-1", "target", "0.4.24", "/tmp"); err == nil {
		t.Fatal("nil runtime admitted")
	}
	if err := rollOwnedCoreHAPostgres(context.Background(), &patroniRollingTestRuntime{}, files, delta, "core-1", "target", "0.4.24", "/tmp"); err == nil {
		t.Fatal("foreign HA member admitted")
	}
	delta.Installed.Owner = "baseharbor"
	if err := rollOwnedCoreHAPostgres(context.Background(), &patroniRollingTestRuntime{}, files, delta, "other-installation", "target", "0.4.24", "/tmp"); err == nil {
		t.Fatal("cross-installation HA roll admitted")
	}
}
