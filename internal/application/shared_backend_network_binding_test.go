package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type sharedNetworkRuntime struct {
	bhruntime.RuntimeProvider
	actual []bhruntime.ProjectResource
	err    error
}

func (r sharedNetworkRuntime) InspectProjectResources(context.Context, string, []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	return r.actual, r.err
}

func TestSharedBackendJoinsVerifiedCoreNetworkWithoutSQLMigration(t *testing.T) {
	root := t.TempDir()
	files := SharedBackendFilesAt(root, "local", "dev")
	core, err := bhruntime.EnsureFilesForProjectAndResources(filepath.Join(root, "runtime"), files.Project, files.ResourceProject, bhruntime.Ports{Postgres: 5432, OpenBao: 8200}, false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(core.Compose)
	if err != nil {
		t.Fatal(err)
	}
	state := sharedBackendState{Version: sharedBackendStateVersion, Environment: "core", PostgresMembers: 1, PostgresAdminCredential: "credentials/standalone", Applications: map[string]sharedBackendAppState{}}
	network := bhruntime.ProjectResource{Kind: "network", Name: files.Network}
	if err := bindSharedBackendNetwork(t.Context(), sharedNetworkRuntime{actual: []bhruntime.ProjectResource{network}}, root, files, &state); err != nil {
		t.Fatal(err)
	}
	if !state.NetworkExternal || state.CoreSQL != nil || state.PostgresAdminCredential != "credentials/standalone" {
		t.Fatal("transport binding changed SQL owner or credentials")
	}
	compose := sharedBackendCompose(files, state)
	if !strings.Contains(compose, "  shared-backend:\n    name: "+files.Network+"\n    external: true\n") || !strings.Contains(compose, "shared-postgres-core:") {
		t.Fatal("independent SQL did not join the existing network")
	}
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeSharedBackendState(files.State, state); err != nil {
		t.Fatal(err)
	}
	for p, v := range map[string]string{files.Compose: compose, files.Env: "SHARED_POSTGRES_ADMIN_PASSWORD=test-only\n"} {
		if err := os.WriteFile(p, []byte(v), 0600); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := ExistingSharedBackendInventoryAt(root, "local", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if !inventory.NetworkExternal {
		t.Fatal("inventory lost the protected external network binding")
	}
	for _, resource := range inventory.Resources {
		if resource == network {
			t.Fatal("shared retirement claims the Core-owned network")
		}
	}
	loaded, err := loadSharedBackendState(files.State, "core")
	if err != nil || !loaded.NetworkExternal {
		t.Fatal("network binding lost on restart", err)
	}
	after, err := os.ReadFile(core.Compose)
	if err != nil || string(after) != string(before) {
		t.Fatal("binding rewrote Core owner")
	}
	for _, runtime := range []sharedNetworkRuntime{{}, {actual: []bhruntime.ProjectResource{{Kind: "network", Name: "foreign"}}}, {err: errors.New("foreign native owner")}} {
		if err := bindSharedBackendNetwork(t.Context(), runtime, root, files, &loaded); err == nil {
			t.Fatal("unverified native network accepted")
		}
	}
	if err := os.WriteFile(core.Compose, append(before, []byte("# foreign alteration\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := bindSharedBackendNetwork(t.Context(), sharedNetworkRuntime{actual: []bhruntime.ProjectResource{network}}, root, files, &loaded); err == nil {
		t.Fatal("modified Core owner accepted")
	}
}

func TestSharedBackendStandaloneNetworkKeepsExistingOwner(t *testing.T) {
	root := t.TempDir()
	files := SharedBackendFilesAt(root, "local", "dev")
	state := sharedBackendState{Version: sharedBackendStateVersion, Environment: "core", Applications: map[string]sharedBackendAppState{}}
	before := sharedBackendCompose(files, state)
	if err := bindSharedBackendNetwork(t.Context(), nil, root, files, &state); err != nil {
		t.Fatal(err)
	}
	if state.NetworkExternal || sharedBackendCompose(files, state) != before {
		t.Fatal("standalone owner implicitly migrated")
	}
	state.NetworkExternal = true
	if err := bindSharedBackendNetwork(t.Context(), nil, root, files, &state); err == nil {
		t.Fatal("lost external owner accepted")
	}
	state.NetworkExternal = false
	core, err := bhruntime.EnsureFilesForProjectAndResources(filepath.Join(root, "runtime"), files.Project, files.ResourceProject, bhruntime.Ports{Postgres: 5432, OpenBao: 8200}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bhruntime.BindControlPlaneToExistingDefaultNetwork(core); err != nil {
		t.Fatal(err)
	}
	if err := bindSharedBackendNetwork(t.Context(), nil, root, files, &state); err != nil {
		t.Fatal(err)
	}
	if state.NetworkExternal || sharedBackendCompose(files, state) != before {
		t.Fatal("Core joining shared owner reversed network authority")
	}
	state.NetworkExternal = true
	if err := bindSharedBackendNetwork(t.Context(), nil, root, files, &state); err == nil {
		t.Fatal("external-on-both-sides network has no owner")
	}
}

func TestSharedBackendNetworkRejectsPartialCoreOwner(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	state := sharedBackendState{}
	if err := bindSharedBackendNetwork(t.Context(), nil, root, SharedBackendFilesAt(root, "local", "dev"), &state); err == nil {
		t.Fatal("partial Core state treated as absent owner")
	}
}
