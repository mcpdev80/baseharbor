package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
)

func TestSharedBackendGatewayRetirementPreservesActiveSiblingAndUnrelatedRoutes(t *testing.T) {
	files := application.SharedBackendFilesAt(t.TempDir(), "isolated", "dev")
	pg := devgateway.Route{Owner: "shared/postgresql", Key: "shared/postgresql", Upstream: "https://shared-pgadmin:8443", Network: files.Network, TrustFile: filepath.Join(files.Dir, "management-ui", "postgres", "pki", "ca.pem"), ServerName: "localhost"}
	cache := devgateway.Route{Owner: "shared/valkey", Key: "shared/valkey", Upstream: "https://shared-cache-ui-access:8443", Network: files.Network, TrustFile: filepath.Join(files.Dir, "management-ui", "cache", "pki", "ca.pem"), ServerName: "localhost"}
	unrelated := devgateway.Route{Owner: "app/sibling/dev", Key: "app/sibling/dev/workload-api"}
	inventory := application.SharedBackendInventory{Files: files, Services: map[capability.ProviderKind][]string{capability.ProviderPostgreSQL: {"shared-pgadmin"}}}
	groups, err := retiredSharedBackendGatewayOwnerRoutes([]devgateway.Route{pg, cache, unrelated}, inventory)
	if err != nil || len(groups) != 1 || groups[0].Owner != cache.Owner || len(groups[0].Routes) != 0 {
		t.Fatalf("must retire only released cache UI, preserving sibling SQL UI and workload: %#v %v", groups, err)
	}
	inventory.Services = map[capability.ProviderKind][]string{}
	groups, err = retiredSharedBackendGatewayOwnerRoutes([]devgateway.Route{pg, cache, unrelated}, inventory)
	if err != nil || len(groups) != 2 {
		t.Fatalf("last UI consumer release leaves stale gateway routes: %#v %v", groups, err)
	}
	cache.Network = "foreign-network"
	if _, err := retiredSharedBackendGatewayOwnerRoutes([]devgateway.Route{cache}, inventory); err == nil {
		t.Fatal("must reject route whose authority differs from the canonical provider owner")
	}
}

func TestSharedBackendGatewayRetirementRejectsMissingOrSymlinkedOwner(t *testing.T) {
	root := t.TempDir()
	resolved := resolvedApplication{TargetStateRoot: root, Target: deployment.ResolvedTarget{Name: "isolated"}, Manifest: application.New("demo", "dev", false, false, false)}
	routes := []devgateway.Route{{Owner: "shared/postgresql", Key: "shared/postgresql"}}
	if _, err := sharedBackendGatewayRetirements(resolved, routes); err == nil {
		t.Fatal("missing owner cannot authorize shared route removal")
	}
	files := application.SharedBackendFilesAt(root, "isolated", "dev")
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(foreign, []byte("foreign-owner"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, files.State); err != nil {
		t.Fatal(err)
	}
	if _, err := sharedBackendGatewayRetirements(resolved, routes); err == nil {
		t.Fatal("symlinked owner cannot authorize shared route removal")
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "foreign-owner" {
		t.Fatal("foreign owner material changed")
	}
	if groups, err := sharedBackendGatewayRetirements(resolved, []devgateway.Route{{Owner: "app/sibling/dev"}}); err != nil || len(groups) != 0 {
		t.Fatalf("unrelated routes must not require a shared backend owner: %#v %v", groups, err)
	}
}
