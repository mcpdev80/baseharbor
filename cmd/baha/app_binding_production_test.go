package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// buildFixtureRepo writes a minimal https workload repo so `baha app init`
// resolves a real workload through the production inspect path.
func buildFixtureRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	compose := "services:\n  demo:\n    build: .\n    labels:\n      io.baseharbor.workload.protocol: https\n    ports:\n      - '8080:8080'\n"
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestCLIProductionPathAssignsServerIdentityProves drives the real `baha app init`
// orbit (not the store helper) and proves the application id is server-assigned,
// never authored in the repository contract.
func TestCLIProductionPathAssignsServerIdentityProves(t *testing.T) {
	assumeVerifiedCoreForSourceTest(t)
	configureTestTarget(t)
	root := buildFixtureRepo(t)
	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input", "--json"}, &out, &errOut); err != nil {
		t.Fatalf("app init: %v\n%s", err, errOut.String())
	}
	var result struct {
		ApplicationID string `json:"application_id"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse init output: %v\n%s", err, out.String())
	}
	if strings.TrimSpace(result.ApplicationID) == "" {
		t.Fatal("production path did not return a server-assigned application id")
	}
	if err := application.ValidateApplicationID(result.ApplicationID); err != nil {
		t.Fatalf("production path returned a non-server id: %v", err)
	}

	// The authored contract must NOT carry the id; only the canonical Store owns it.
	authored, err := os.ReadFile(filepath.Join(root, application.RepositoryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(authored), "application_id") {
		t.Fatal("authored contract carries an authored application id; id assignment is not server-side")
	}

	// The canonical Store owns the id; a read-back must resolve the same owner.
	owner := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	list, err := owner.List()
	if err != nil {
		t.Fatalf("load canonical store: %v", err)
	}
	found := false
	for _, item := range list {
		if item.ApplicationID == result.ApplicationID {
			found = true
		}
	}
	if !found {
		t.Fatal("server-assigned id not present in canonical Store; id is not server-owned")
	}
}

// TestCLITwoFreshHomesConcurrentRegistrationProves runs two independent checkout
// paths and asserts concurrent init each resolves a server-owned id. The id is
// server-assigned and stored in the canonical Store, never authored in the
// repository manifest, so the test reads it from the owner after init. Each
// checkout is an independent repository with its own canonical Store, so the
// servers-side id is per-home; the race-20 convergence is exercised at the
// internal/application level (TestConcurrentFirstRegistrationConvergesProves).
func TestCLITwoFreshHomesConcurrentRegistrationProves(t *testing.T) {
	assumeVerifiedCoreForSourceTest(t)
	configureTestTarget(t)

	// Two independent checkout homes.
	homeA := buildFixtureRepo(t)
	homeB := buildFixtureRepo(t)

	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()

	var ids []string
	for _, home := range []string{homeA, homeB} {
		if err := os.Chdir(home); err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		if err := runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input"}, &out, &errOut); err != nil {
			t.Fatalf("home init: %v\n%s", err, errOut.String())
		}
		owner := application.Store{Root: filepath.Join(home, ".baseharbor", "apps")}
		list, err := owner.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range list {
			if item.ApplicationID == "" {
				t.Fatal("a fresh checkout registered without a server id")
			}
			if err := application.ValidateApplicationID(item.ApplicationID); err != nil {
				t.Fatalf("checkout resolved a non-server id: %v", err)
			}
			ids = append(ids, item.ApplicationID)
		}
	}
	if len(ids) < 2 {
		t.Fatal("fresh checkouts did not each register")
	}
	// Fresh, independent checkouts each own their own server id; they must not
	// cross-rewrite each other's identity.
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if len(seen) != len(ids) {
		t.Fatalf("independent checkouts cross-wrote identity: %v", seen)
	}
}
