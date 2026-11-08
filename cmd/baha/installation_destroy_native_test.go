package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestInstallationDestroyCurrentChildrenInterruptedVolumeAndRetry(t *testing.T) {
	if os.Getenv("BASEHARBOR_INSTALLATION_DESTROY_ACCEPTANCE") != "1" {
		t.Skip("requires isolated native installation destruction")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(t.Context()), 12*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	kind := os.Getenv("BASEHARBOR_TEST_RUNTIME")
	if kind != "docker" && kind != "podman" {
		t.Fatal("explicit native runtime required")
	}
	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"target", "create", "installation-retry-native", "--provider", kind, "--access", "installation-retry-native", "--reference", "local", "--default"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	// A native installation destroy acceptance must first create the real
	// installation owner. A target registration and app init alone no longer
	// establish this authority; relying on them caused the gate to fail before
	// reaching the full destroy/retry scenario.
	core, err := installCore(ctx, strings.NewReader(""), &out, runtimeUpOptions{
		Yes: true, ControlPlaneOnly: true, MachineRole: coreinstallation.Development,
		RecoveryFile: filepath.Join(t.TempDir(), "recovery.json"),
	})
	if err != nil || !core.Ready {
		t.Fatalf("real Core bootstrap before installation destroy: ready=%t err=%v\n%s", core.Ready, err, out.String())
	}
	t.Chdir(t.TempDir())
	for name, data := range map[string]string{"compose.yaml": corelessHTTPWorkload, "baseharbor.yaml": "version: 1\napp:\n  name: installation-retry\n  environment: dev\nservices:\n  sql: true\n  identity: true\n"} {
		if err := os.WriteFile(name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	// A repository-owned candidate needs explicit, exact-submission consent.
	// First obtain its published digest without writing deployment state, then
	// acknowledge that same digest on the second invocation.
	applyCtx := coreBootstrapFixtureConsent(t, ctx)
	out.Reset()
	firstErr := runWithIO(applyCtx, []string{"app", "apply", "--skip-memory-preflight"}, &out, &out)
	if firstErr == nil || !strings.Contains(firstErr.Error(), "candidate submission requires") {
		t.Fatalf("expected pre-mutation candidate-consent challenge: %v\\n%s", firstErr, out.String())
	}
	match := regexp.MustCompile(`confirm-hash\s+([0-9a-f]{64})`).FindStringSubmatch(out.String())
	if len(match) != 2 {
		t.Fatalf("candidate challenge did not publish an exact confirmation digest: %s", out.String())
	}
	out.Reset()
	if err := runWithIO(applyCtx, []string{"app", "apply", "--skip-memory-preflight", "--confirm-candidate", match[1]}, &out, &out); err != nil {
		t.Fatalf("consented native fixture apply: %v\\n%s", err, out.String())
	}
	r, err := resolveApplication(ctx, application.Store{}, nil, "status")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := detectRuntimeForTarget(ctx, r.Target)
	if err != nil {
		t.Fatal(err)
	}
	// The real full-installation acceptance must prove the Keycloak side
	// of the Core as well as SQL, otherwise stale Identity containers may
	// survive an apparently successful destroy.
	identityFiles, identityExpected, err := identityprovider.ExistingKeycloakRuntimeInventoryAt(
		r.Manifest,
		capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		r.TargetStateRoot, r.Target.Name,
	)
	if err != nil {
		t.Fatalf("managed Keycloak realization absent before destroy: %v", err)
	}
	identityBefore, err := provider.InspectProjectResources(ctx, identityFiles.Project, identityExpected)
	if err != nil {
		t.Fatal(err)
	}
	keycloakRunning := false
	for _, resource := range identityBefore {
		if resource.Kind == "container" {
			keycloakRunning = true
		}
	}
	if !keycloakRunning {
		t.Fatal("native destroy acceptance did not start an actual managed Keycloak container")
	}
	shared, err := application.ExistingSharedBackendInventoryAt(r.TargetStateRoot, r.Target.Name, "core")
	if err != nil {
		t.Fatal(err)
	}
	// A real Core installation owns PostgreSQL directly. The application's
	// shared inventory then references CoreSQL without duplicating its service.
	// Probe the canonical Core owner rather than assuming a standalone SQL
	// container is present in the application's shared Compose project.
	if shared.CoreSQL != nil {
		coreSQL, inventoryErr := bhruntime.ExistingControlPlaneRuntimeInventory(filepath.Dir(shared.CoreSQL.Compose), shared.CoreSQL.Project)
		if inventoryErr != nil || len(coreSQL.SQLServices) == 0 {
			t.Fatalf("canonical Core-owned SQL service unavailable: %v", inventoryErr)
		}
		shared.Files.Project = coreSQL.Files.Project
		shared.Files.Compose = coreSQL.Files.Compose
		shared.Files.Env = coreSQL.Files.Env
		// Use the canonical Core SQL client service, not the database daemon
		// (which does not provide the application-authenticated test client).
		client := ""
		for _, service := range coreSQL.SQLServices {
			if service == "postgres-admin" {
				client = service
				break
			}
		}
		if client == "" {
			t.Fatal("canonical Core SQL client service is unavailable")
		}
		shared.Services[capability.ProviderPostgreSQL] = []string{client}
		shared.Resources = coreSQL.Resources
	}
	var credentials [32]byte
	qualifySelectedBackendData(t, ctx, provider, r, shared, "sql", 0, &credentials)
	plan := func() runtimeDestroyImpact {
		t.Helper()
		targets, discovery := discoverFullDestroyTargets()
		if countFullDestroyBlockers(discovery) > 0 {
			t.Fatal(discovery)
		}
		inventories, err := collectFullDestroyInventory(ctx, targets)
		if err != nil {
			t.Fatal(err)
		}
		p, err := buildRuntimeDestroyImpact(ctx, inventories, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := plan()
	if len(p.Applications) != 1 {
		t.Fatal("actual application reset absent from aggregate plan")
	}
	for _, ack := range []*application.ProviderChangeConsent{nil, {PlanID: p.Scope.PlanID, First: true, DeleteData: true}, {PlanID: p.Scope.PlanID, First: true, Second: true}} {
		if err := destroyInstallation(withProviderChangeConsent(ctx, ack), true, io.Discard, io.Discard); err == nil {
			t.Fatal("installation consent bypass")
		}
		qualifySelectedBackendData(t, ctx, provider, r, shared, "sql", 2, &credentials)
	}
	actual, err := provider.InspectProjectResources(ctx, shared.Files.Project, shared.Resources)
	if err != nil {
		t.Fatal(err)
	}
	volume := ""
	for _, resource := range actual {
		if resource.Kind == "volume" {
			volume = resource.Name
			break
		}
	}
	if volume == "" {
		t.Fatal("actual owned SQL volume absent")
	}
	foreign := "baha-foreign-installation-holder-" + filepath.Base(t.TempDir())
	native := func(args ...string) (string, error) {
		command := exec.CommandContext(ctx, kind, args...)
		if kind == "podman" {
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "XDG_CONFIG_HOME=") && !strings.HasPrefix(entry, "XDG_DATA_HOME=") {
					command.Env = append(command.Env, entry)
				}
			}
		}
		data, err := command.CombinedOutput()
		return string(data), err
	}
	if _, err := native("run", "-d", "--name", foreign, "--label", "com.docker.compose.project="+foreign, "--user", "65000:65000", "--read-only", "--cap-drop", "ALL", "--network", "none", "--volume", volume+":/held:ro", "docker.io/library/alpine:3.24", "sh", "-ec", "exec sleep 600"); err != nil {
		t.Fatal("isolated foreign holder creation failed")
	}
	defer func() { _, _ = native("container", "rm", "--force", foreign) }()
	p = plan()
	ack := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true}
	if err := destroyInstallation(withProviderChangeConsent(ctx, ack), true, io.Discard, io.Discard); err == nil {
		t.Fatal("in-use owned volume deletion falsely succeeded")
	}
	if _, err := os.Stat(r.TargetStateRoot); err != nil {
		t.Fatal("partial installation destroy removed its retry owner")
	}
	if _, err := os.Stat(shared.Files.Env); err != nil {
		t.Fatal("failed native deletion lost protected shared owner")
	}
	if _, err := native("container", "inspect", foreign); err != nil {
		t.Fatal("aggregate deletion removed foreign holder")
	}
	t.Log("explicit full-installation reset reached native in-use volume failure; exact protected Target/shared owner and foreign holder retained; no data continuity claimed after separately consented destructive steps")
	if _, err := native("container", "rm", "--force", foreign); err != nil {
		t.Fatal("remove only isolated foreign fixture holder")
	}
	p = plan()
	ack = &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true}
	if err := destroyInstallation(withProviderChangeConsent(ctx, ack), true, io.Discard, io.Discard); err != nil {
		t.Fatal("freshly acknowledged installation retry failed", err)
	}
	if _, err := native("volume", "inspect", volume); err == nil {
		t.Fatal("selected owned data volume survived successful deletion")
	}
	identityAfter, err := provider.ListOwnedProjectResources(ctx, identityFiles.Project)
	if err != nil {
		t.Fatal(err)
	}
	if len(identityAfter) != 0 {
		t.Fatalf("full installation destroy left owned Keycloak resources: %+v", identityAfter)
	}
	config, err := deployment.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("successful installation destroy retained configuration")
	}
	if _, err := os.Stat(r.TargetStateRoot); !os.IsNotExist(err) {
		t.Fatal("successful installation destroy retained selected owner")
	}
	t.Log("actual full-installation child reset, partial native deletion, preserved foreign authority and fresh current-plan retry completed; selected native data/configuration/owners absent")
}
