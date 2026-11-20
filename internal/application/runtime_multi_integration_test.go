package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestMultiInstanceComposeLifecycleInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_CI_RUNTIME_INTEGRATION") != "1" && os.Getenv("BASEHARBOR_RUNTIME_SECURITY_ACCEPTANCE") != "1" {
		t.Skip("real multi-instance runtime verification requires BASEHARBOR_CI_RUNTIME_INTEGRATION=1 or BASEHARBOR_RUNTIME_SECURITY_ACCEPTANCE=1")
	}
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	t.Setenv(ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))
	// Each instance must use its container credential, independent of host interpolation.
	t.Setenv("VALKEY_PASSWORD", "host-value-must-not-be-used-by-instance")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	compose, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatalf("detect compose: %v", err)
	}

	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New(fmt.Sprintf("multi-ci-%d", os.Getpid()), "dev", false, false, false)
	m = WithSQLInstances(m, "primary", "analytics")
	m = WithCacheInstances(m, "cache", "sessions")
	files, err := EnsureRuntime(ctx, serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	project := files.Project
	if err := compose.ConfigProject(ctx, project, files.Compose, files.Env); err != nil {
		t.Fatalf("validate generated multi-instance compose: %v", err)
	}
	if err := compose.UpProject(ctx, project, files.Compose, files.Env); err != nil {
		t.Fatalf("start multi-instance runtime: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		if err := compose.DestroyProject(cleanupCtx, project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy multi-instance runtime: %v", err)
		}
	}()

	network := bhruntime.ProjectResource{Kind: "network", Name: ApplicationBackendNetworkName(m)}
	exists, err := compose.InspectProjectResource(ctx, project, network)
	if err != nil {
		t.Fatalf("inspect application backend network: %v", err)
	}
	if !exists {
		t.Fatalf("application backend network %s was not created", network.Name)
	}

	deadline := time.Now().Add(45 * time.Second)
	for {
		err = VerifyPostgresRuntime(ctx, compose, m, files)
		if err == nil {
			err = VerifyValkeyRuntime(ctx, compose, m, files)
		}
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("multi-instance runtime never became ready: %v", err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("multi-instance runtime verification timed out: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}

	running, err := compose.RunningServicesProject(ctx, project, files.Compose, files.Env)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(running)
	want := []string{"postgres-analytics", "postgres-primary", "valkey-cache", "valkey-cache-access", "valkey-sessions", "valkey-sessions-access"}
	if len(running) != len(want) {
		t.Fatalf("unexpected running services: got %#v want %#v", running, want)
	}
	for i := range want {
		if running[i] != want[i] {
			t.Fatalf("unexpected running services: got %#v want %#v", running, want)
		}
		if want[i] == "postgres-analytics" || want[i] == "postgres-primary" {
			// The official postgres image starts its entrypoint as root to
			// initialize the data directory, then drops to the unprivileged
			// postgres user before launching the server. The running server is
			// verified to run non-root with zero effective capabilities below.
			allowedEntrypointCaps := []string{"CHOWN", "DAC_READ_SEARCH", "FOWNER", "SETGID", "SETUID"}
			requirements := containersecurity.Requirements{
				ReadOnlyRootfs:      true,
				DropAllCaps:         true,
				NoNewPrivs:          true,
				AllowRootEntrypoint: true,
				AllowedCapAdd:       allowedEntrypointCaps,
			}
			if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
				requirements.AllowedEffectiveCaps = []string{"CHOWN", "DAC_READ_SEARCH", "FOWNER", "SETGID", "SETUID"}
			}
			if err := containersecurity.VerifyComposeService(ctx, project, want[i], requirements); err != nil {
				t.Fatalf("%s runtime security: %v", want[i], err)
			}
			const processSecurity = `seen=0
for status in /proc/[0-9]*/status; do
  [ -r "$status" ] || continue
  name=$(awk '$1 == "Name:" { print $2; exit }' "$status" 2>/dev/null) || continue
  case "$name" in postgres*) ;; *) continue ;; esac
  uid=$(awk '$1 == "Uid:" { print $2; exit }' "$status" 2>/dev/null) || continue
  caps=$(awk '$1 == "CapEff:" { print $2; exit }' "$status" 2>/dev/null) || continue
  [ -n "$uid" ] && [ -n "$caps" ] || continue
  seen=1
  [ "$uid" != "0" ] || exit 1
  case "$caps" in *[!0]*) exit 1 ;; esac
done
			[ "$seen" = "1" ]`
			if _, err := compose.ExecProject(ctx, project, files.Compose, files.Env, want[i], "sh", "-ec", processSecurity); err != nil {
				t.Fatalf("%s postgres server must run non-root without effective capabilities: %v", want[i], err)
			}
			continue
		}
		if err := containersecurity.VerifyComposeService(ctx, project, want[i], containersecurity.Requirements{
			ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true,
		}); err != nil {
			t.Fatalf("%s runtime security: %v", want[i], err)
		}
	}
}
