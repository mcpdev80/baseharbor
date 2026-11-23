package application

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"go.yaml.in/yaml/v3"
)

// fix-7 regression tests (0428-089): the two blockers the independent review
// measured at head 97d88ffef, pinned against the PRODUCTION composition path
// (EnsureRuntime -> RuntimeComposeYAMLForProject -> RenderPersesCompose).
// CI gate: they live in internal/application and run in the required
// "Go unit · packages" shard of the PR gate plan like every other test of
// this package.

// persesComposeServiceDoc parses one rendered compose file into the service
// fields the gateway topology and the Perses server posture are decided by.
type persesComposeServiceDoc struct {
	Ports       []string `yaml:"ports"`
	NetworkMode string   `yaml:"network_mode"`
	Networks    any      `yaml:"networks"`
	Command     []string `yaml:"command"`
	Image       string   `yaml:"image"`
}

func persesComposeServicesDoc(t *testing.T, rendered string) map[string]persesComposeServiceDoc {
	t.Helper()
	var doc struct {
		Services map[string]persesComposeServiceDoc `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &doc); err != nil {
		t.Fatalf("decode the rendered compose: %v\n%s", err, rendered)
	}
	return doc.Services
}

// TestPersesComposeValidatesUnderRealToolchain is fix-7 B1 requirement 1: the
// rendered compose must pass the REAL validator, not a string comparison —
// the string-only checks are what let the network_mode+networks defect
// through. The YAML-level network_mode XOR networks rule is enforced on every
// host; where the docker compose CLI exists (the hosted CI runners), its own
// `config --quiet` validation must accept the stack too.
func TestPersesComposeValidatesUnderRealToolchain(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	files := persesRuntimeFiles(t, m)
	rendered, err := RuntimeComposeYAMLForProject(m, files)
	if err != nil {
		t.Fatalf("render the production Perses compose: %v", err)
	}
	// Rule 1a (everywhere): a service with network_mode must not also
	// declare networks, and vice versa (compose-spec forbids the pair).
	for name, service := range persesComposeServicesDoc(t, rendered) {
		hasNetworks := service.Networks != nil && service.Networks != "[]" && !(isList(service.Networks) && listLen(service.Networks) == 0)
		if service.NetworkMode != "" && hasNetworks {
			t.Fatalf("regression (B1): service %q declares network_mode %q AND networks %v: docker compose rejects that combination", name, service.NetworkMode, service.Networks)
		}
	}
	perses := persesComposeServicesDoc(t, rendered)[serviceaccess.PersesServiceName]
	if perses.NetworkMode != "service:"+serviceaccess.PersesGatewayServiceName {
		t.Fatalf("regression (B1): the Perses service must keep the shared-namespace topology, got network_mode %q", perses.NetworkMode)
	}
	// Rule 1d: the shared topology must still be the one persisted — if the
	// plan silently switched to the namespace-local form this witness fails.
	plan, err := serviceaccess.ReadPersesGatewayPlan(PersesGatewayStateDir(files))
	if err != nil {
		t.Fatalf("read the persisted gateway plan: %v", err)
	}
	if !plan.SharedGatewayNetwork {
		t.Fatal("regression (B1): the persisted plan is no longer the shared-namespace topology")
	}
	// Rule 1b: docker's own validator, where the toolchain exists.
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Log("docker CLI unavailable: the hosted exact-head run covers the compose-spec validation")
		return
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(rendered), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(docker, "compose", "-f", filepath.Join(dir, "compose.yaml"), "config", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("regression (B1): `docker compose config -q` rejects the rendered stack: %v\n%s\nrendered:\n%s", err, out, rendered)
	}
}

func isList(v any) bool {
	_, ok := v.([]any)
	return ok
}

func listLen(v any) int {
	list, ok := v.([]any)
	if !ok {
		return -1
	}
	return len(list)
}

// TestPersesComposeAliasLivesOnGatewayNotPerses is fix-7 B1 requirements 2+3:
// the backend-network alias of the surface must be registered on the GATEWAY
// service (the only backend member that can forward), never on the Perses
// service (whose network_mode forbids it); and the Perses command must keep
// the loopback pin, so a peer that resolves the alias and dials :8080 reaches
// the gateway's namespace where the proxy refuses it — Perses never answers
// on a routable interface.
func TestPersesComposeAliasLivesOnGatewayNotPerses(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	files := persesRuntimeFiles(t, m)
	rendered, err := RuntimeComposeYAMLForProject(m, files)
	if err != nil {
		t.Fatalf("render the production Perses compose: %v", err)
	}
	alias := mPersesServiceAlias(m)
	if alias == "" {
		t.Fatal("the deployment alias must not be empty")
	}
	services := persesComposeServicesDoc(t, rendered)
	gateway, ok := services[serviceaccess.PersesGatewayServiceName]
	if !ok {
		t.Fatalf("the gateway service must render:\n%s", rendered)
	}
	if !yamlCarriesAlias(gateway.Networks, alias) {
		t.Fatalf("regression (B1): the gateway service must register the backend alias %q in its networks, got %v:\n%s", alias, gateway.Networks, rendered)
	}
	perses := services[serviceaccess.PersesServiceName]
	if perses.Networks != nil {
		t.Fatalf("regression (B1): the Perses service declares networks %v alongside network_mode:\n%s", perses.Networks, rendered)
	}
	if len(perses.Command) == 0 {
		t.Fatal("the Perses service must carry its pinned command")
	}
	bound := false
	for _, flag := range perses.Command {
		if strings.Contains(flag, "--web.listen-address=127.0.0.1:") {
			bound = true
		}
		if strings.Contains(flag, "0.0.0.0") {
			t.Fatalf("regression (B1): the Perses command must never bind 0.0.0.0, got %q", flag)
		}
	}
	if !bound {
		t.Fatalf("regression (B1): the Perses command must keep the namespace loopback pin, got %v", perses.Command)
	}
	// In the shared-namespace topology the gateway dials the namespace
	// loopback (Perses binds loopback inside the joined namespace and the
	// deployment alias resolves to the gateway itself), so the Caddyfile
	// upstream is the loopback, never the alias. The alias still rides the
	// gateway service network (verified above) and is a certificate identity
	// of the gateway; it is simply not the dial target in this topology.
	caddy := filepath.Join(PersesGatewayStateDir(files), "service-access", "config", "Caddyfile")
	contents, err := os.ReadFile(caddy)
	if err != nil {
		t.Fatalf("read the gateway Caddyfile: %v", err)
	}
	if !strings.Contains(string(contents), "reverse_proxy http://127.0.0.1:"+strconv.Itoa(serviceaccess.PersesUpstreamPort)+" {") {
		t.Fatalf("regression (B1): the shared-namespace gateway must dial the namespace loopback, got:\n%s", contents)
	}
}

// yamlCarriesAlias reports whether a parsed compose networks value (map form)
// names the alias in any network's aliases list.
func yamlCarriesAlias(networks any, alias string) bool {
	byNetwork, ok := networks.(map[string]any)
	if !ok {
		return false
	}
	for _, entry := range byNetwork {
		switch typed := entry.(type) {
		case nil:
			continue
		case map[string]any:
			list, ok := typed["aliases"].([]any)
			if !ok {
				continue
			}
			for _, candidate := range list {
				if name, ok := candidate.(string); ok && name == alias {
					return true
				}
			}
		}
	}
	return false
}

// TestPersesRenderedConfigAuthenticationProviderIsValid is fix-7 B2: with
// enable_auth true the configuration must activate a provider (the native
// one), and the parsed YAML must match the shape the Perses server itself
// validates (security.enable_auth, security.authentication.providers
// .enable_native, security.encryption_key_file, all under `security`). The
// rejected 97d88ffef shape (authentication: at document level, providers
// disabled) must never render again.
func TestPersesRenderedConfigAuthenticationProviderIsValid(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	files := persesRuntimeFiles(t, m)
	contents, err := os.ReadFile(filepath.Join(files.Dir, "providers", "management-ui", "perses", "config.yaml"))
	if err != nil {
		t.Fatalf("read the mounted configuration: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(contents, &doc); err != nil {
		t.Fatalf("the mounted configuration must parse: %v\n%s", err, contents)
	}
	security, ok := doc["security"].(map[string]any)
	if !ok {
		t.Fatalf("the configuration must carry a security block:\n%s", contents)
	}
	if enabled, ok := security["enable_auth"].(bool); !ok || !enabled {
		t.Fatal("regression (B2): the management surface must never render with authentication disabled")
	}
	if _, inline := security["encryption_key"]; inline {
		t.Fatal("the encryption key must never be inlined")
	}
	keyFile, ok := security["encryption_key_file"].(string)
	if !ok || keyFile != serviceaccess.PersesEncryptionKeyContainerPath {
		t.Fatalf("regression (B2): security.encryption_key_file must name the mounted key file, got %v", security["encryption_key_file"])
	}
	auth, ok := security["authentication"].(map[string]any)
	if !ok {
		t.Fatalf("regression (B2): enable_auth requires the security.authentication block:\n%s", contents)
	}
	providers, ok := auth["providers"].(map[string]any)
	if !ok {
		t.Fatalf("regression (B2): security.authentication must carry a providers block:\n%s", contents)
	}
	native, ok := providers["enable_native"].(bool)
	if !ok || !native {
		t.Fatalf("regression (B2): enable_auth with no active provider has no login path (the Perses server refuses that shape): enable_native must be true, got %v", providers["enable_native"])
	}
	if doc["authentication"] != nil {
		t.Fatalf("regression (B2): the providers block belongs under security.authentication, not at document level:\n%s", contents)
	}
}
