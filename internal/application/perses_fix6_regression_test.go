package application

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"go.yaml.in/yaml/v3"
)

// fix-6 regression tests (0428-089): these run through the PRODUCTION compose
// path (EnsureRuntime -> RuntimeComposeYAMLForProject ->
// writePersesGatewayComposeServices), not just the definition functions, and
// pin the two blockers the review found at ed7f448f. CI gate: they live in the
// internal/application package and run in the required "Go unit · packages"
// shard (go test ./internal/... selection of the PR gate plan), like every
// other test of this package.

// TestPersesProductionComposeIsGatewayOnlyExposure is regression (a): on the
// production composition the Perses service carries NO published port and NO
// routable bind; only the management gateway owns a host port, on loopback.
func TestPersesProductionComposeIsGatewayOnlyExposure(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	files := persesRuntimeFiles(t, m)
	rendered, err := RuntimeComposeYAMLForProject(m, files)
	if err != nil {
		t.Fatalf("render the production Perses compose: %v", err)
	}
	var compose struct {
		Services map[string]struct {
			Image       string   `yaml:"image"`
			Ports       []string `yaml:"ports"`
			NetworkMode string   `yaml:"network_mode"`
			Command     []string `yaml:"command"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &compose); err != nil {
		t.Fatalf("decode the production Perses compose: %v", err)
	}
	perses, ok := compose.Services[serviceaccess.PersesServiceName]
	if !ok {
		t.Fatalf("the composition must render the perses service:\n%s", rendered)
	}
	if len(perses.Ports) != 0 {
		t.Fatalf("regression (a): the Perses service must publish no port, got %v", perses.Ports)
	}
	for _, flag := range perses.Command {
		if strings.Contains(flag, "0.0.0.0") {
			t.Fatalf("regression (a): the Perses command must never bind 0.0.0.0, got %q", flag)
		}
	}
	if perses.NetworkMode != "service:"+serviceaccess.PersesGatewayServiceName {
		t.Fatalf("regression (a): the Perses service must join the gateway namespace, got %q", perses.NetworkMode)
	}
	gateway, ok := compose.Services[serviceaccess.PersesGatewayServiceName]
	if !ok {
		t.Fatalf("regression (a): the gateway service must be the only host-visible address:\n%s", rendered)
	}
	if len(gateway.Ports) == 0 {
		t.Fatal("regression (a): the gateway must publish the operator loopback port")
	}
	for _, published := range gateway.Ports {
		if !strings.HasPrefix(published, "127.0.0.1:") {
			t.Fatalf("regression (a): the gateway may only publish on loopback, got %q", published)
		}
	}
}

// TestPersesProductionComposeFailsClosedWithoutKeyMaterial is regression (b):
// a deployment whose encryption-key material was never produced (or was
// removed afterwards) must fail the composition closed instead of rendering a
// service whose configuration references a missing secret.
func TestPersesProductionComposeFailsClosedWithoutKeyMaterial(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	files := persesRuntimeFiles(t, m)
	// Simulate a projection that lost the key material (tampering, partial
	// restore): the authoritative 0600 file and its bind-directory copy.
	if err := os.Remove(PersesEncryptionKeyProjection(files.Dir)); err != nil {
		t.Fatal(err)
	}
	if _, err := RuntimeComposeYAMLForProject(m, files); err == nil {
		t.Fatal("regression (b): composition without materialized key material must fail closed, never render a silently keyless Perses")
	}
	// A missing gateway Caddyfile (half-projected deployment) fails closed too.
	files2 := persesRuntimeFiles(t, m)
	caddy := filepath.Join(PersesGatewayStateDir(files2), "service-access", "config", "Caddyfile")
	if err := os.Remove(caddy); err != nil {
		t.Fatal(err)
	}
	if _, err := RuntimeComposeYAMLForProject(m, files2); err == nil {
		t.Fatal("regression (b): composition without the gateway Caddyfile must fail closed")
	}
}

// TestPersesProductionComposeMountsKeyAndConfig is regression (c): with the
// key present, the rendering carries auth and mounts the key and the
// configuration read-only at the contract paths, and the EnsurePerses-Gate
// proves the configuration's enable_auth statement matches the validated
// policy.
func TestPersesProductionComposeMountsKeyAndConfig(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	files := persesRuntimeFiles(t, m)
	rendered, err := RuntimeComposeYAMLForProject(m, files)
	if err != nil {
		t.Fatalf("render the production Perses compose: %v", err)
	}
	var compose struct {
		Services map[string]struct {
			Volumes []string `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &compose); err != nil {
		t.Fatalf("decode the production Perses compose: %v", err)
	}
	mounts := map[string]bool{}
	for _, mount := range compose.Services[serviceaccess.PersesServiceName].Volumes {
		mounts[mount] = true
	}
	for _, want := range serviceaccess.PersesServiceMounts() {
		if !mounts[want] {
			t.Fatalf("regression (c): the rendering must carry the contract mount %q, got %v", want, mounts)
		}
		if !strings.HasSuffix(want, ":ro") && !strings.Contains(want, ":/persistence") {
			t.Fatalf("regression (c): every Perses bind mount must be read-only: %q", want)
		}
	}
	// The configuration in the container states enable_auth and references the
	// key file at the mounted path (never inline).
	configBytes, err := os.ReadFile(filepath.Join(files.Dir, "providers", "management-ui", "perses", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PersesGatewayPlanFor(m, capability.DefaultPersesSecurityPolicy())
	if err != nil {
		t.Fatal(err)
	}
	expectation, err := serviceaccess.RenderPersesConfig(plan.Policy, plan.ServiceAddress)
	if err != nil {
		t.Fatal(err)
	}
	if string(configBytes) != expectation {
		t.Fatalf("regression (c): the mounted configuration must be the deterministic rendering of the validated policy:\n%s", configBytes)
	}
	// The key bind source exists with the projection's readable mode.
	info, err := os.Stat(filepath.Join(files.Dir, "providers", "perses-management", "secrets", "encryption-key"))
	if err != nil {
		t.Fatalf("regression (c): the key bind source must exist: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("regression (c): the container-readable projection must be 0644, got %v", info.Mode().Perm())
	}
	authInfo, err := os.Stat(PersesEncryptionKeyHostPath(files.Dir))
	if err != nil {
		t.Fatal(err)
	}
	if authInfo.Mode().Perm() != 0o600 {
		t.Fatalf("regression (c): the authoritative key must stay 0600, got %v", authInfo.Mode().Perm())
	}
}

// TestPersesProductionComposeCarriesTheTempoAccessGatewayMount pins the P1
// access boundary in the production composition: the tempo-access gateway's
// container-readable runtime material (the trust bundle the datasource proxy
// verifies with, and the enrolled client identity it presents) rides into the
// Perses container as a read-only directory bind, exactly like every other
// managed gateway mount. Removing the bind from the shared contract
// (PersesServiceMounts) or rendering it without the contract turns this cell
// red; the datasource can only reach the gateway alias from the backend
// network when its trust material is actually mounted.
func TestPersesProductionComposeCarriesTheTempoAccessGatewayMount(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	files := persesRuntimeFiles(t, m)
	rendered, err := RuntimeComposeYAMLForProject(m, files)
	if err != nil {
		t.Fatalf("render the production Perses compose: %v", err)
	}
	want := serviceaccess.TempoAccessRuntimeComposeBind + ":" + serviceaccess.PersesTempoAccessMountDir + ":ro"
	if !strings.Contains(rendered, strconv.Quote(want)) {
		t.Fatalf("the production compose must carry the tempo-access gateway mount %q", want)
	}
	found := false
	for _, mount := range serviceaccess.PersesServiceMounts() {
		if mount == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("the shared Perses mount contract must name the tempo-access gateway bind, got %v", serviceaccess.PersesServiceMounts())
	}
}
