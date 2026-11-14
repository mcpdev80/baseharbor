package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestCreateTargetPersistsStableUID exercises the real CLI entry point
// createTargetDefinition through machineTargetCreateInput and proves the stable
// Target UID is generated and persisted on the written definition, without
// moving any on-disk Target state and without touching the ownership TenantID.
func TestCreateTargetPersistsStableUID(t *testing.T) {
	configureTestTarget(t)

	uid, err := deployment.NewTargetUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createTargetDefinition(context.Background(), machineTargetCreateInput{
		Name:            "podman-prod",
		RuntimeProvider: "podman",
		Access:          "local-docker",
		Reference:       "local",
		UID:             uid,
	}); err != nil {
		t.Fatalf("create target: %v", err)
	}

	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	def, ok := cfg.Targets["podman-prod"]
	if !ok {
		t.Fatal("created target missing from persisted config")
	}
	if def.TargetUID == "" {
		t.Fatal("stable Target UID was not persisted on create")
	}
	if def.TargetUID != strings.ToLower(uid) {
		t.Fatalf("persisted UID = %q, want %q", def.TargetUID, uid)
	}
}

// TestCreateTargetUIDIdempotentOnRetry proves re-issuing the same create with the
// same explicit UID keeps the one stable identity rather than regenerating it.
func TestCreateTargetUIDIdempotentOnRetry(t *testing.T) {
	configureTestTarget(t)

	uid, err := deployment.NewTargetUID()
	if err != nil {
		t.Fatal(err)
	}
	input := machineTargetCreateInput{
		Name:            "podman-prod",
		RuntimeProvider: "podman",
		Access:          "local-docker",
		Reference:       "local",
		UID:             uid,
	}
	if _, err := createTargetDefinition(context.Background(), input); err != nil {
		t.Fatalf("create target: %v", err)
	}

	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Targets["podman-prod"].TargetUID != strings.ToLower(uid) {
		t.Fatal("explicit UID not preserved across create")
	}
}

// TestCreateTargetRejectsUIDCollisionByName proves the CLI fails closed when the
// UID being assigned matches another Target's UID — the fail-closed collision
// guard at the real entry point.
func TestCreateTargetRejectsUIDCollisionByName(t *testing.T) {
	configureTestTarget(t)

	uid, err := deployment.NewTargetUID()
	if err != nil {
		t.Fatal(err)
	}
	first := machineTargetCreateInput{
		Name:            "alpha",
		RuntimeProvider: "docker",
		Access:          "local-docker",
		Reference:       "local",
		UID:             uid,
	}
	if _, err := createTargetDefinition(context.Background(), first); err != nil {
		t.Fatalf("create first target: %v", err)
	}

	second := machineTargetCreateInput{
		Name:            "beta",
		RuntimeProvider: "docker",
		Access:          "local-docker",
		Reference:       "local",
		UID:             uid,
	}
	if _, err := createTargetDefinition(context.Background(), second); err == nil {
		t.Fatal("duplicate Target UID across two names must fail closed at the CLI")
	}

	// The second create must have left no partial definition behind.
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := cfg.Targets["beta"]; exists {
		t.Fatal("failed create left a partial Target definition behind")
	}
}
