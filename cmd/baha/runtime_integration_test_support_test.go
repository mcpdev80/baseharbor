package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	identityprovider "github.com/mcpdev80/baseharbor/internal/identityprovider"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var (
	runtimeIntegrationTrustPlaneOnce sync.Once
	runtimeIntegrationTrustPlaneErr  error
)

// Single deterministic teardown of the shared bootstrap trust-plane. Registered
// once per package by ensureRuntimeIntegrationTrustPlane; runs on BOTH the
// success and failure paths so the bootstrap Core never becomes a residual
// bh-w169-ci-bootstrap-* container/volume/network after the package. The owning
// target + runtime provider are captured while the target selection env is
// still valid; teardown reuses them and never depends on the ambient
// selection. This helper owns no product destroy path — it only removes the
// resources the test suite itself materialized.
var (
	runtimeIntegrationTrustPlaneCleanupOnce sync.Once
	runtimeIntegrationTrustPlaneBootstrap   string
	runtimeIntegrationTrustPlaneTarget      deployment.ResolvedTarget
	runtimeIntegrationTrustPlaneProvider    bhruntime.RuntimeProvider
	runtimeIntegrationTrustPlaneRegistered  bool
)

func ensureRuntimeIntegrationTrustPlane(t *testing.T, ctx context.Context) {
	t.Helper()
	runtimeIntegrationTrustPlaneOnce.Do(func() {
		configHome, err := os.MkdirTemp("", "baseharbor-ci-config-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		dataHome, err := os.MkdirTemp("", "baseharbor-ci-data-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if err := os.Setenv("XDG_CONFIG_HOME", configHome); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if err := os.Setenv("XDG_DATA_HOME", dataHome); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		// Isolation is a CODE-GARANTIE, not an environmental precondition: the
		// bootstrap target below is a *fresh* configured target with its own
		// `<name>-default` native network and its own management Core, so this
		// single shared `up --control-plane-only` never gates on the ambient
		// `local` Core that a polluted shared daemon may carry. The ambient
		// `local` Core (if any) is never touched, reset or deleted. The four
		// lifecycle tests below each add their own distinct BASEHARBOR_TARGET
		// after this helper runs (one isolated Core per test), so a mutation
		// that drops a single test's target fails only that one test at the
		// gate — a per-test discriminating proof, not a systemic failure.
		//
		// The bootstrap target name is generated fresh per process so a
		// contaminated or pre-existing Core on a fixed name can never be the
		// isolation boundary: each run owns its own `<name>-default` network and
		// management Core. A per-process suffix keeps the cell reproducible only
		// by re-running, never by reusing another lane's fixed-name Core.
		bootstrapTarget := "w169-ci-bootstrap-" + bootstrapTargetSuffix()
		if err := os.Setenv("BASEHARBOR_TARGET", bootstrapTarget); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		// Always restore the selection, including when OpenBao was already
		// initialized or when bootstrap fails. Otherwise a later lifecycle
		// test silently inherits this bootstrap target.
		defer func() {
			if err := os.Unsetenv("BASEHARBOR_TARGET"); err != nil && runtimeIntegrationTrustPlaneErr == nil {
				runtimeIntegrationTrustPlaneErr = err
			}
			if runtimeIntegrationTrustPlaneErr == nil {
				runtimeIntegrationTrustPlaneErr = mutatePersistedTarget("", true)
			}
		}()
		// The shared rootless daemon is the integration endpoint. An active
		// CLI context (the prepare step's setuptool registers "rootless")
		// takes precedence over DOCKER_HOST inside the engine resolver, so
		// make the endpoint authoritative and neutralize the context.
		_ = os.Unsetenv("DOCKER_CONTEXT")

		var out bytes.Buffer
		if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
			if err := runWithIO(ctx, []string{"target", "create", bootstrapTarget, "--runtime-provider", "podman", "--access", "local", "--access-provider", "local", "--reference", "local", "--default"}, &out, &out); err != nil {
				runtimeIntegrationTrustPlaneErr = err
				return
			}
			out.Reset()
		} else {
			// Docker (the default test runtime) needs its own bootstrap target
			// configured before the single shared `up` below; this target has
			// its own `<name>-default` native network and management Core, so
			// the gate never sees the ambient `local` Core.
			if err := runWithIO(ctx, []string{"target", "create", bootstrapTarget, "--runtime-provider", "docker", "--access", "local", "--access-provider", "local", "--reference", "local", "--default"}, &out, &out); err != nil {
				runtimeIntegrationTrustPlaneErr = err
				return
			}
			out.Reset()
		}
		if err := runWithIO(ctx, []string{"up", "--control-plane-only", "--yes"}, &out, &out); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		// Capture the bootstrap Core runtime provider + owning target NOW, while
		// the bootstrap target selection and its config are still valid. The
		// deferred unset below clears BASEHARBOR_TARGET, so teardown cannot
		// re-resolve them later. This stored pair is what the single package
		// teardown uses to destroy the bootstrap Core.
		if tgt, terr := effectiveTarget(ctx); terr == nil {
			if provider, perr := detectRuntimeForTarget(ctx, tgt); perr == nil {
				runtimeIntegrationTrustPlaneTarget = tgt
				runtimeIntegrationTrustPlaneProvider = provider
			}
		}

		compose, files, err := openBaoRuntime(ctx)
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		state, err := platformopenbao.Inspect(ctx, compose, files)
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if state.Initialized {
			if state.Sealed {
				runtimeIntegrationTrustPlaneErr = os.ErrPermission
				return
			}
			runtimeIntegrationTrustPlaneErr = platformopenbao.CheckManager(ctx, compose, files)
			return
		}

		dir, err := os.MkdirTemp("", "baseharbor-ci-openbao-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		recoveryPath := filepath.Join(dir, "recovery.json")
		out.Reset()
		runtimeIntegrationTrustPlaneErr = runWithIO(ctx, []string{"openbao", "bootstrap", "--recovery-file", recoveryPath}, &out, &out)
	})
	if runtimeIntegrationTrustPlaneErr != nil {
		t.Fatalf("prepare BaseHarbor integration trust plane: %v", runtimeIntegrationTrustPlaneErr)
	}
	if _, err := existingTargetRuntimeFiles(ctx); err != nil {
		t.Fatalf("BaseHarbor integration trust plane state missing: %v", err)
	}
	// A missing per-test isolation call must never silently fall back to
	// the bootstrap Core or a configured default target.
	sentinel := "w169-isolation-required-" + bootstrapTargetSuffix()
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load targets for isolation guard: %v", err)
	}
	if _, exists := cfg.Targets[sentinel]; exists {
		t.Fatalf("isolation guard target %q unexpectedly exists", sentinel)
	}
	t.Setenv("BASEHARBOR_TARGET", sentinel)

	// Register the deterministic bootstrap trust-plane teardown exactly once,
	// after the trust plane is proven ready. It must run on BOTH the success
	// and the failure paths, so it survives a test that t.Fatalfs mid-run. The
	// bootstrap target name + owning target + runtime were captured inside the
	// sync.Once closure above (before the selection env was cleared).
	runtimeIntegrationTrustPlaneBootstrap = "w169-ci-bootstrap-" + bootstrapTargetSuffix()
	registerRuntimeIntegrationTrustPlaneCleanup(t, ctx)
}

func registerRuntimeIntegrationTrustPlaneCleanup(t *testing.T, ctx context.Context) {
	runtimeIntegrationTrustPlaneCleanupOnce.Do(func() {
		runtimeIntegrationTrustPlaneRegistered = true
		// The teardown uses its OWN fresh context: the test ctx may already be
		// canceled by the time t.Cleanup runs (e.g. the test hit its own
		// timeout). A canceled parent would make the fail-closed teardown report
		// false "context canceled" residuals instead of actually destroying the
		// bootstrap Core.
		t.Cleanup(func() {
			if !runtimeIntegrationTrustPlaneRegistered {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			destroyRuntimeIntegrationTrustPlane(t, cleanupCtx)
		})
	})
}

// destroyRuntimeIntegrationTrustPlane tears down exactly the bootstrap Core the
// suite materialized: the consolidated control-plane project plus its shared
// keycloak Core and shared application backends. It is fail-closed — any
// residual bh-w169-ci-bootstrap-* container/volume/network after teardown fails
// the run, which is the CI-visible wächter signal the runtime_integration job
// asserts. No product destroy path is altered; only resources the test suite
// materialized are removed. The owning target + runtime are captured at
// registration time (see registerRuntimeIntegrationTrustPlaneCleanup) because
// the selection env is cleared before cleanup runs.
func destroyRuntimeIntegrationTrustPlane(t *testing.T, ctx context.Context) {
	t.Helper()
	if !runtimeIntegrationTrustPlaneRegistered || runtimeIntegrationTrustPlaneProvider == nil {
		return
	}
	target := runtimeIntegrationTrustPlaneTarget
	dataDir, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Errorf("bootstrap trust-plane teardown target root: %v", err)
		return
	}
	runCleanup := func(resource string, fn func() error) {
		if err := fn(); err != nil {
			t.Errorf("bootstrap trust-plane teardown %s: %v", resource, err)
		}
	}
	runCleanup("identity-keycloak", func() error {
		return identityprovider.DestroyAllSharedKeycloakAt(ctx, runtimeIntegrationTrustPlaneProvider, dataDir, target.Name)
	})
	runCleanup("data-providers", func() error {
		return application.DestroyAllSharedBackendsAt(ctx, runtimeIntegrationTrustPlaneProvider, dataDir, target.Name)
	})
	runtimeRoot := filepath.Join(dataDir, "runtime")
	if files, filesErr := bhruntime.ExistingFilesForProject(runtimeRoot, bhruntime.SharedProjectName(target.Name)); filesErr == nil {
		runCleanup("control-plane", func() error {
			return runtimeIntegrationTrustPlaneProvider.DestroyProject(ctx, files.Project, files.Compose, files.Env)
		})
	}
	if err := assertBootstrapTrustPlaneLeavesNoLeftover(t, ctx, runtimeIntegrationTrustPlaneProvider, target); err != nil {
		t.Errorf("bootstrap trust-plane teardown left residual resources: %v", err)
	}
}

// assertBootstrapTrustPlaneLeavesNoLeftover fails t when any BaseHarbor-owned
// container, volume or network for the bootstrap control-plane namespace
// survives teardown. An intentional leak here makes the CI run red.
func assertBootstrapTrustPlaneLeavesNoLeftover(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, target deployment.ResolvedTarget) error {
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return fmt.Errorf("inspect containers after bootstrap teardown: %w", err)
	}
	owned := []string{}
	for _, c := range containers {
		if strings.HasPrefix(c.Project, "bh-w169-ci-bootstrap") || strings.HasPrefix(c.Project, "bh-"+target.Name) {
			owned = append(owned, c.Project+"/"+c.Name)
		}
	}
	if len(owned) != 0 {
		sort.Strings(owned)
		return fmt.Errorf("bootstrap containers remain after teardown: %s", strings.Join(owned, ", "))
	}
	project := bhruntime.SharedProjectName(target.Name)
	leftover, err := provider.ListOwnedProjectResources(ctx, project)
	if err != nil {
		return fmt.Errorf("inspect bootstrap control-plane resources after teardown: %w", err)
	}
	if len(leftover) != 0 {
		names := []string{}
		for _, r := range leftover {
			names = append(names, r.Kind+" "+r.Name)
		}
		sort.Strings(names)
		return fmt.Errorf("bootstrap control-plane resources remain after teardown: %s", strings.Join(names, ", "))
	}
	return nil
}

// isolateRuntimeForTest configures a fresh target for the requested runtime.
// Target and provider are verified through the real deployment resolver, not
// inferred from environment variables or constructed network names.
func isolateRuntimeForTest(t *testing.T, name string) {
	t.Helper()
	provider := integrationRuntimeProvider(t)
	isoTarget := "w169-" + name + "-" + bootstrapTargetSuffix() + "-iso"
	if err := os.Setenv("BASEHARBOR_TARGET", isoTarget); err != nil {
		t.Fatalf("set isolated target %q: %v", isoTarget, err)
	}
	var buf bytes.Buffer
	args := []string{"target", "create", isoTarget, "--runtime-provider", provider, "--access", "local", "--access-provider", "local", "--reference", "local"}
	if err := runWithIO(t.Context(), args, &buf, &buf); err != nil {
		t.Fatalf("configure isolated %s target %q: %v\n%s", provider, isoTarget, err, buf.String())
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load actual target configuration: %v", err)
	}
	selected, err := selectedTargetName("", os.Getenv("BASEHARBOR_TARGET"), cfg)
	if err != nil {
		t.Fatalf("resolve active target: %v", err)
	}
	resolved, err := cfg.ResolveTarget(selected, "")
	if err != nil {
		t.Fatalf("resolve configured runtime target: %v", err)
	}
	if resolved.Name != isoTarget || resolved.RuntimeProvider != provider || resolved.AccessProvider != "local" {
		t.Fatalf("runtime isolation mismatch: want target=%q provider=%q access=local, got %+v", isoTarget, provider, resolved)
	}
	// TEST-SCOPE owner loan marker (card t-20261106-r27-remote-up-podman-owner-missing,
	// lane w286): this isolated test Target has no installation of its own by
	// design — the boot Target's genuine Core installation is the single
	// durable owner. The marker file documents the isolated test Target; the
	// production owner loan resolves this namespace against the boot Core by
	// its genuine, READY installation, not by the marker alone.
	writeBahaTestIsolationMarker(t, isoTarget)
	t.Logf("isolated runtime resolved from deployment config: target=%q runtime=%q endpoint=%q context=%q", resolved.Name, resolved.RuntimeProvider, resolved.DockerEndpoint, resolved.DockerContext)
}

func integrationRuntimeProvider(t *testing.T) string {
	t.Helper()
	provider := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if provider == "" {
		return "docker"
	}
	if provider != "docker" && provider != "podman" {
		t.Fatalf("unsupported integration runtime provider %q", provider)
	}
	return provider
}

// bootstrapTargetSuffix returns a short random hex suffix so the shared trust
// plane helper configures a bootstrap target that cannot collide with a
// contaminated or pre-existing Core on a fixed name; each process gets its own
// `w169-ci-bootstrap-<suffix>` isolated Core.
func bootstrapTargetSuffix() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000"
	}
	return hex.EncodeToString(b[:])
}

func TestIntegrationRuntimeProviderSelection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		runtime string
		want    string
	}{
		{name: "default-docker", runtime: "", want: "docker"},
		{name: "explicit-docker", runtime: "docker", want: "docker"},
		{name: "explicit-podman", runtime: "podman", want: "podman"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_TEST_RUNTIME", tc.runtime)
			if got := integrationRuntimeProvider(t); got != tc.want {
				t.Fatalf("runtime provider selection: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMissingRuntimeIsolationCannotFallBackToBootstrap(t *testing.T) {
	const sentinel = "w169-isolation-required-regression"
	t.Setenv("BASEHARBOR_TARGET", sentinel)
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "w169-ci-bootstrap",
		Targets: map[string]deployment.TargetDefinition{
			"w169-ci-bootstrap": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}},
		},
	}
	selected, err := selectedTargetName("", os.Getenv("BASEHARBOR_TARGET"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if selected != sentinel {
		t.Fatalf("missing isolation selected %q instead of guard %q", selected, sentinel)
	}
	if _, err := cfg.ResolveTarget(selected, ""); err == nil {
		t.Fatal("unconfigured isolation guard resolved to the bootstrap target")
	}
}
