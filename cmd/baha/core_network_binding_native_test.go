package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Qualify the actual shared network before/after independent management setup.
// Reading the same once-seeded application database proves more than READY.
func TestCoreBootstrapRetainsApplicationSharedNetworkAndSQL(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORE_SHARED_NETWORK_ACCEPTANCE") != "1" {
		t.Skip("requires isolated native Docker/rootless Podman acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(t.Context()), 15*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	kind := os.Getenv("BASEHARBOR_TEST_RUNTIME")
	if kind != "docker" && kind != "podman" {
		t.Fatal("explicit native runtime required")
	}
	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"target", "create", "shared-network-native", "--provider", kind, "--access", "shared-network-native", "--reference", "local", "--default"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	for name, content := range map[string]string{
		"compose.yaml":    corelessHTTPWorkload,
		"baseharbor.yaml": "version: 1\napp:\n  name: retained-network-sql\n  environment: dev\nservices:\n  sql: true\n",
	} {
		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	app := applySelectiveIdentityIntent(t, ctx)
	provider, err := detectRuntimeForTarget(ctx, app.Target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 3*time.Minute)
		defer cancel()
		if err := approvedNativeFixtureCleanup(t, cleanup, io.Discard); err != nil {
			t.Errorf("owned fixture cleanup failed; preserve retry owners: %v", err)
		}
	})
	shared, err := application.ExistingSharedBackendInventoryAt(app.TargetStateRoot, app.Target.Name, "core")
	if err != nil || shared.CoreSQL != nil {
		t.Fatal("initial application did not create its independent shared SQL owner", err)
	}
	env, err := os.ReadFile(shared.Files.Env)
	if err != nil {
		t.Fatal(err)
	}
	var credentials [32]byte
	qualifySelectedBackendData(t, ctx, provider, app, shared, "sql", 0, &credentials)
	// Capture only native network ID/labels, never container environment secrets.
	network := func() string {
		t.Helper()
		output, err := exec.CommandContext(ctx, kind, "network", "inspect", shared.Files.Network).Output()
		if err != nil {
			t.Fatal("native shared network inspection failed")
		}
		var records []struct {
			ID     string            `json:"Id"`
			Labels map[string]string `json:"Labels"`
		}
		if json.Unmarshal(output, &records) != nil || len(records) != 1 || records[0].ID == "" || records[0].Labels["com.docker.compose.project"] != shared.Files.Project {
			t.Fatal("native network has no exact original shared ownership")
		}
		encoded, _ := json.Marshal(records[0])
		return string(encoded)
	}
	originalNetwork := network()
	ports := bhruntime.Ports{}
	ports.Postgres, err = selectControlPlanePort(&out, "PostgreSQL", "--postgres-port", 0, bhruntime.DefaultPostgresPort, 15432)
	if err != nil {
		t.Fatal(err)
	}
	ports.OpenBao, err = selectControlPlanePort(&out, "OpenBao", "--openbao-port", 0, bhruntime.DefaultOpenBaoPort, 18200)
	if err != nil {
		t.Fatal(err)
	}
	opts := runtimeUpOptions{Yes: true, ControlPlaneOnly: true, MachineRole: coreinstallation.Development, RecoveryFile: filepath.Join(t.TempDir(), "recovery.json"), PostgresPort: ports.Postgres, OpenBaoPort: ports.OpenBao}
	var installationID string
	for repeat := 0; repeat < 2; repeat++ {
		state, err := installCore(ctx, nil, &out, opts)
		if err != nil || !state.Ready {
			t.Fatalf("independent management bootstrap repeat=%d failed: %v", repeat, err)
		}
		if repeat == 0 {
			installationID = state.ID
		} else if state.ID != installationID {
			t.Fatal("bootstrap repeat changed installation identity")
		}
		files, err := existingTargetRuntimeFiles(ctx)
		if err != nil || !files.DefaultNetworkExternal {
			t.Fatal("Core did not retain an external shared network reference", err)
		}
		owner, err := bhruntime.ExistingControlPlaneRuntimeInventory(filepath.Dir(files.Compose), files.Project)
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range owner.Resources {
			if resource.Kind == "network" && resource.Name == shared.Files.Network {
				t.Fatal("Core claimed ownership of the application network")
			}
		}
		if network() != originalNetwork {
			t.Fatal("Core replaced or relabelled the original network")
		}
		after, err := os.ReadFile(shared.Files.Env)
		if err != nil || sha256.Sum256(after) != sha256.Sum256(env) {
			t.Fatal("Core bootstrap changed original shared SQL credentials")
		}
		qualifySelectedBackendData(t, ctx, provider, app, shared, "sql", 2, &credentials)
	}
	// Simulate an interrupted reference write: the protected env already says
	// external, while the exact secured Compose owner still has its old shape.
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	needle := "  default:\n    name: " + shared.Files.Network + "\n    external: true\n"
	if !strings.Contains(string(compose), needle) {
		t.Fatal("canonical external reference absent")
	}
	pending := strings.Replace(string(compose), needle, strings.TrimSuffix(needle, "    external: true\n"), 1)
	if err := os.WriteFile(files.Compose, []byte(pending), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := installCore(ctx, nil, &out, opts); err != nil {
		t.Fatal("canonical pending network reference did not recover", err)
	}
	if repaired, err := os.ReadFile(files.Compose); err != nil || !bytes.Equal(repaired, compose) {
		t.Fatal("pending recovery did not restore the exact secured owner")
	}
	plan, err := buildSelectedCoreDestroyPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ack := &application.ProviderChangeConsent{PlanID: plan.Impact.Scope.PlanID, First: true, Second: true}
	if err := runtimeDestroy(withProviderChangeConsent(ctx, ack), []string{"--yes"}, io.Discard); err != nil {
		t.Fatal("selected management retirement failed", err)
	}
	if network() != originalNetwork {
		t.Fatal("management retirement changed the retained application network")
	}
	qualifySelectedBackendData(t, ctx, provider, app, shared, "sql", 2, &credentials)
	if _, err := installCore(ctx, nil, &out, opts); err != nil {
		t.Fatal("management recovery failed", err)
	}
	if network() != originalNetwork {
		t.Fatal("management recovery changed the retained application network")
	}
	qualifySelectedBackendData(t, ctx, provider, app, shared, "sql", 2, &credentials)
	t.Log("independent management bootstrap and repeat retained original shared network ID/labels, excluded Core ownership, preserved original application SQL credentials and once-seeded verify-full TLS data")
	t.Log("canonical pending reference recovered; selected Core retirement/reactivation preserved original shared-network ownership and actual application TLS data")
}
