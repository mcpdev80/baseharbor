package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRetiredSharedMetricsWithdrawsOnlyItsAcknowledgedCanonicalGatewayRoute(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	t.Setenv(application.MetricsEnabledEnv, "true")
	prior := resolved.Manifest
	resolved.Manifest = application.WithMetricsSource(prior, "app", "demo-app", 8080, "/metrics")
	resolved.Manifest.Services.ObservabilityManagementUI = true
	if err := recordAppliedDeployment(t.Context(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		t.Fatal(err)
	}
	files, err := metrics.EnsureProviderFilesWithRuntimeCAAt(t.Context(), serviceissuer.New(t), resolved.TargetStateRoot, resolved.Target.Name, resolved.Manifest, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := application.ReleaseApplicationProviderReferencesAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		t.Fatal(err)
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatal(err)
	}
	var instance capability.ProviderInstance
	for _, candidate := range registry.Instances {
		if candidate.Provider.Kind == capability.ProviderPrometheus {
			instance = candidate
		}
	}
	if instance.ID == "" {
		t.Fatal("fixture lacks retained Prometheus offer")
	}
	runtime := &standaloneRetirementFixture{}
	inventory, err := existingStandaloneProviderInventory(resolved, runtime, instance, registry)
	if err != nil {
		t.Fatal("fixture lacks canonical Prometheus authority", err)
	}
	host, err := devaccess.SharedHost(resolved.Target.Name, "prometheus")
	if err != nil {
		t.Fatal(err)
	}
	route := devgateway.Route{Owner: "shared/prometheus", Key: "shared/prometheus", Host: host, Upstream: "https://prometheus-access:9090", Network: metrics.PublishNetworkName(inventory.project), TrustFile: filepath.Join(files.Dir, "service-access", "pki", "ca.pem"), ServerName: "prometheus"}
	unrelated := devgateway.Route{Owner: "app/sibling/dev", Key: "app/sibling/dev/workload-api", Host: "sibling.baha.localhost"}
	before, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.MetricsEnabledEnv, "false")
	resolved.Manifest = prior
	plan := application.ProviderPlan{Version: application.ProviderPlanVersion, Target: resolved.Target.Name, Complete: true, Changes: []application.ProviderChange{{InstanceID: instance.ID, Action: "remove-runtime", Ownership: capability.OwnershipBaseHarbor}}}
	groups, err := retiredSharedMetricsGatewayRoutes(resolved, runtime, plan, []devgateway.Route{route, unrelated})
	if err != nil || len(groups) != 1 || groups[0].Owner != route.Owner || len(groups[0].Routes) != 0 {
		t.Fatalf("retired metrics leaves a canonical gateway pointing at its absent upstream: %#v %v", groups, err)
	}
	for _, kind := range []string{"consumer", "required", "incomplete", "foreign Target", "foreign owner", "wrong protocol", "foreign upstream", "foreign network", "foreign trust", "foreign host", "foreign path"} {
		t.Run(kind, func(t *testing.T) {
			bad := plan
			bad.Changes = append([]application.ProviderChange(nil), plan.Changes...)
			changed := route
			preserve := false
			switch kind {
			case "consumer":
				bad.Changes[0].Consumers = []string{"deployment/sibling"}
				preserve = true
			case "required":
				bad.Required = []string{instance.ID}
				preserve = true
			case "incomplete":
				bad.Complete = false
			case "foreign Target":
				bad.Target = "foreign"
			case "foreign owner":
				bad.Changes[0].Ownership = capability.OwnershipExternal
			case "wrong protocol":
				bad.Version = "unknown"
			case "foreign upstream":
				changed.Upstream = "https://foreign:9090"
			case "foreign network":
				changed.Network = "foreign"
			case "foreign trust":
				changed.TrustFile = filepath.Join(t.TempDir(), "ca.pem")
			case "foreign host":
				changed.Host = "foreign.example"
			case "foreign path":
				changed.PathPrefix = "/foreign"
			}
			groups, err := retiredSharedMetricsGatewayRoutes(resolved, runtime, bad, []devgateway.Route{changed, unrelated})
			if len(groups) != 0 || (preserve && err != nil) || (!preserve && err == nil) {
				t.Fatalf("must preserve active or unverified route: %#v %v", groups, err)
			}
		})
	}
	canonical, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, append(append([]byte(nil), canonical...), []byte("\n# changed owner\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if groups, err := retiredSharedMetricsGatewayRoutes(resolved, runtime, plan, []devgateway.Route{route}); err == nil || len(groups) != 0 {
		t.Fatal("changed canonical definition authorized route removal")
	}
	if err := os.WriteFile(files.Compose, canonical, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.MetricsEnabledEnv, "true")
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, application.WithMetricsSource(prior, "app", "demo-app", 8080, "/metrics")); err != nil {
		t.Fatal(err)
	}
	current, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil || len(current.BindingsForProviderInstance(instance.ID)) == 0 {
		t.Fatal("fixture lacks actual new metrics binding", err)
	}
	t.Setenv(application.MetricsEnabledEnv, "false")
	if groups, err := retiredSharedMetricsGatewayRoutes(resolved, runtime, plan, []devgateway.Route{route}); err == nil || len(groups) != 0 {
		t.Fatal("new unreleased metrics binding authorized route removal")
	}
	after, err := os.ReadFile(files.Env)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("route selection rewrote protected provider material")
	}
}
