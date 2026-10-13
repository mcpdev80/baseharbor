package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestApplicationFullResetRequiresNativeCurrentScopeAndDeletesSelectedData(t *testing.T) {
	if os.Getenv("BASEHARBOR_APP_FULL_RESET_ACCEPTANCE") != "1" {
		t.Skip("requires isolated native application reset acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 8*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	target := configureTestTarget(t)
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		entry := cfg.Targets[target.Name]
		entry.Runtime.Provider = "podman"
		cfg.Targets[target.Name] = entry
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, provider, target)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	source := []byte("version: 1\napp:\n  name: reset-owned\n  environment: dev\nservices:\n  sql: true\n")
	if err := os.WriteFile("baseharbor.yaml", source, 0600); err != nil {
		t.Fatal(err)
	}
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	applySelectiveIdentityIntent(t, ctx)
	r, err := resolveApplication(ctx, application.Store{}, nil, "destroy")
	if err != nil {
		t.Fatal(err)
	}
	files, _, expected, err := nativeBackendInventory(r)
	if err != nil {
		t.Fatal(err)
	}
	var credentialDigest [32]byte
	inventory := application.SharedBackendInventory{Files: application.SharedBackendFiles{Project: files.Project, Compose: files.Compose, Env: files.Env}, Services: map[capability.ProviderKind][]string{capability.ProviderPostgreSQL: {"postgres"}}}
	qualifySelectedBackendData(t, ctx, provider, r, inventory, "sql", 0, &credentialDigest)
	complete := false
	t.Cleanup(func() {
		if complete {
			return
		}
		cleanup, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 2*time.Minute)
		defer cancel()
		if err := destroySelectiveApplicationWithConsent(cleanup); err != nil {
			t.Errorf("isolated reset fixture cleanup: %v", err)
		}
	})
	p, err := buildApplicationDestructionPlan(ctx, r, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, ack := range []*application.ProviderChangeConsent{nil, {PlanID: p.Scope.PlanID, First: true}, {PlanID: p.Scope.PlanID, First: true, Second: true}} {
		err := runWithIO(withProviderChangeConsent(ctx, ack), []string{"app", "destroy", "--yes", "--full-reset", "--json"}, io.Discard, io.Discard)
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.Details == nil {
			t.Fatal("native reset bypassed required scope or separate data consent", err)
		}
		qualifySelectedBackendData(t, ctx, provider, r, inventory, "sql", 2, &credentialDigest)
	}
	// A failed consent attempt cannot mutate the owner or invalidate its plan.
	fresh, err := buildApplicationDestructionPlan(ctx, r, true)
	if err != nil || fresh.Scope.PlanID != p.Scope.PlanID {
		if err == nil {
			t.Logf("unchanged reset scope: owner revision changed=%t provider impact changed=%t", fresh.Scope.Owner.Revision != p.Scope.Owner.Revision, fresh.Providers.Impact.PlanID != p.Providers.Impact.PlanID)
		}
		t.Fatal("refused consent mutated reset scope", err)
	}
	ack := &application.ProviderChangeConsent{PlanID: fresh.Scope.PlanID, First: true, Second: true, DeleteData: true}
	var output bytes.Buffer
	if err := runWithIO(withProviderChangeConsent(ctx, ack), []string{"app", "destroy", "--yes", "--full-reset", "--json"}, &output, io.Discard); err != nil {
		t.Fatal("explicit native reset failed", err)
	}
	actual, err := provider.InspectProjectResources(ctx, files.Project, expected)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range actual {
		if resource.Kind == "container" || resource.Kind == "volume" {
			t.Fatal("selected native reset retained owned runtime/data")
		}
	}
	if _, err := deployment.LoadDeploymentRecord(r.DeploymentIdentity); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("verified reset retained selected deployment owner", err)
	}
	data, err := os.ReadFile("baseharbor.yaml")
	if err != nil || sha256.Sum256(data) != sha256.Sum256(source) {
		t.Fatal("reset changed authored source")
	}
	if !strings.Contains(output.String(), `"data_deleted": true`) {
		t.Fatal("native reset lacks structured data deletion result")
	}
	complete = true
	t.Log("actual native application full reset: missing/one/two acknowledgements without separate data consent preserved original TLS-authenticated SQL; current outer scope plus separate consent removed selected native data and deployment owner while preserving authored source")
}
