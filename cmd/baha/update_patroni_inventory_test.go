package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestMixedPatroniInventoryOnlyResumesJournaledPatch(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	state := coreinstallation.State{ID: "core", Spec: coreinstallation.Spec{HA: true}}
	old := bhruntime.ImageIdentity{Reference: "ghcr.io/zalando/spilo-18:4.1-p1", Digest: "sha256:" + strings.Repeat("a", 64)}
	next := bhruntime.ImageIdentity{Reference: "ghcr.io/zalando/spilo-18:4.1-p2", Digest: "sha256:" + strings.Repeat("b", 64)}
	pin := coreupdate.BackingPin{Role: "core-ha-postgresql", Version: "18-spilo-4.1-p2", Image: next.Reference, Digest: next.Digest}
	path := filepath.Join(root, "patroni-members.json")
	journal := coreupdate.PatroniMemberJournal{
		Path: path, Release: "0.4.24", Installation: state.ID, Scope: "shared",
		Desired: coreupdate.Desired{Kind: coreupdate.SQL, Version: pin.Version, Image: pin.Image, Digest: pin.Digest},
	}
	images := []bhruntime.ImageIdentity{old, next, old}
	if _, err := classifyMixedHAPostgresWithJournal(ctx, state, "0.4.24", images, pin, path); err == nil {
		t.Fatal("unreceipted partial upgrade admitted")
	}
	if err := journal.Record(ctx, "postgres-member-2", "verified"); err != nil {
		t.Fatal(err)
	}
	got, err := classifyMixedHAPostgresWithJournal(ctx, state, "0.4.24", images, pin, path)
	if err != nil || got.Classification != coreupdate.BackupRequired || got.Installed.Image != old.Reference {
		t.Fatalf("receipted partial patch not admitted safely: %+v %v", got, err)
	}
	images[2] = bhruntime.ImageIdentity{Reference: "ghcr.io/zalando/spilo-18:4.1-p0", Digest: "sha256:" + strings.Repeat("c", 64)}
	if _, err := classifyMixedHAPostgresWithJournal(ctx, state, "0.4.24", images, pin, path); err == nil {
		t.Fatal("third foreign Spilo image identity admitted")
	}
	images[2] = old
	if err := journal.Record(ctx, "postgres-member-1", "verified"); err != nil {
		t.Fatal(err)
	}
	if _, err := classifyMixedHAPostgresWithJournal(ctx, state, "0.4.24", images, pin, path); err == nil {
		t.Fatal("verified member with old image admitted")
	}
}
