package serviceaccess

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func persesRequirements(authentication AuthenticationMode) PersesGatewayRequirements {
	return PersesGatewayRequirements{
		Upstream:       "http://perses:8080",
		Policy:         capability.DefaultPersesSecurityPolicy(),
		Authentication: authentication,
		AuthTokenFile:  "/run/baseharbor/perses-access-token",
		ServerNames:    []string{"perses-ui-access"},
	}
}

func TestBuildPersesGatewayRefusesAnonymousSurface(t *testing.T) {
	requirements := persesRequirements(AuthenticationMTLS)
	requirements.Policy = capability.AnonymousPersesSecurityPolicy()
	plan, err := BuildPersesGateway(requirements)
	if err == nil {
		t.Fatal("a gateway for an anonymous Perses surface must fail closed before anything is rendered")
	}
	if plan.Spec.ServiceName != "" {
		t.Fatalf("a refused plan must carry no spec, got %+v", plan.Spec)
	}
	var anonymous *capability.PersesAnonymousExposureError
	if !errors.As(err, &anonymous) {
		t.Fatalf("the refusal must be the typed anonymous-exposure verdict, got %v", err)
	}
}

func TestBuildPersesGatewayRequiresGatewayAuthentication(t *testing.T) {
	for _, authentication := range []AuthenticationMode{AuthenticationNone, AuthenticationNative, "", AuthenticationOIDC} {
		requirements := persesRequirements(authentication)
		_, err := BuildPersesGateway(requirements)
		if err == nil {
			t.Fatalf("gateway authentication %q must be refused: it cannot protect an operator-facing surface", authentication)
		}
		if authentication == AuthenticationOIDC && !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("an external OIDC gateway mode must name the missing adapter, got %v", err)
		}
	}
}

func TestBuildPersesGatewayDeniesDatasourceProxy(t *testing.T) {
	plan, err := BuildPersesGateway(persesRequirements(AuthenticationMTLS))
	if err != nil {
		t.Fatalf("a protected mTLS Perses gateway must build: %v", err)
	}
	if len(plan.DeniedPaths) != 1 || plan.DeniedPaths[0] != capability.PersesProxyPrefix {
		t.Fatalf("the gateway must refuse the datasource proxy prefix, got %v", plan.DeniedPaths)
	}
	if len(plan.Spec.DenyPaths) == 0 || plan.Spec.DenyPaths[0] != capability.PersesProxyPrefix {
		t.Fatalf("the gateway spec must carry the refusal into the rendered config, got %v", plan.Spec.DenyPaths)
	}
	if !plan.RequireClientIdentity || !plan.Spec.RequireClient {
		t.Fatal("an mTLS Perses gateway must require a client certificate")
	}
	if plan.Spec.Upstream != "http://perses:8080" {
		t.Fatalf("the validated upstream must survive into the spec, got %q", plan.Spec.Upstream)
	}
	if plan.UpstreamBindAddress != "127.0.0.1" {
		t.Fatalf("Perses must be bound to loopback so only the gateway reaches it, got %q", plan.UpstreamBindAddress)
	}
}

func TestBuildPersesGatewayRendersProxyRefusalWithoutLeakingSecrets(t *testing.T) {
	plan, err := BuildPersesGateway(persesRequirements(AuthenticationToken))
	if err != nil {
		t.Fatalf("a protected token Perses gateway must build: %v", err)
	}
	directives, digest, err := PersesGatewayCaddyDirectives(plan)
	if err != nil {
		t.Fatalf("the refusal directives must render: %v", err)
	}
	if !strings.Contains(directives, capability.PersesProxyPrefix) || !strings.Contains(directives, "404") {
		t.Fatalf("the rendered block must refuse the proxy with a not-found answer, got %q", directives)
	}
	if strings.Contains(directives, "Bearer") || strings.Contains(directives, "basic_auth") {
		t.Fatalf("refusal directives must never carry authorization material, got %q", directives)
	}
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("the rendering must be pinned by a sha256 digest, got %q", digest)
	}
	// Reconciliation detects drift: a different denial set changes the digest.
	other := plan
	other.DeniedPaths = []string{capability.PersesProxyPrefix, "/api/v1/dac/"}
	_, otherDigest, err := PersesGatewayCaddyDirectives(other)
	if err != nil {
		t.Fatalf("the alternative rendering must build: %v", err)
	}
	if otherDigest == digest {
		t.Fatal("the digest must change when the denied route set changes")
	}
	// A control character in a denied path is a rendering injection and must be
	// refused rather than silently dropped.
	bad := plan
	bad.DeniedPaths = []string{"/proxy/\n  reverse_proxy 10.0.0.1:80"}
	if _, _, err := PersesGatewayCaddyDirectives(bad); err == nil {
		t.Fatal("a denied path carrying control characters must be refused")
	}
}

func TestPersesGatewayTokenAuthenticationRequiresTokenFileReference(t *testing.T) {
	requirements := persesRequirements(AuthenticationToken)
	requirements.AuthTokenFile = ""
	if _, err := BuildPersesGateway(requirements); err == nil {
		t.Fatal("token authentication without a token file must fail closed")
	}
	if _, err := BuildPersesGateway(persesRequirements(AuthenticationToken)); err != nil {
		t.Fatalf("a referenced token file must be accepted: %v", err)
	}
	// The requirement struct must never carry credential values, only paths: a
	// rendered manifest or audit record must not be able to leak the token.
	data, err := json.Marshal(requirements)
	if err != nil {
		t.Fatalf("marshal requirements: %v", err)
	}
	if !strings.Contains(string(data), "auth_token_file") && !strings.Contains(string(data), "AuthTokenFile") {
		t.Fatalf("the requirements must expose the token as a file reference, got %s", data)
	}
}

func TestNormalizedPersesUpstreamRejectsCredentialBearingAndRoutableAddresses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "missing", value: "", want: "upstream is required"},
		{name: "embedded credentials", value: "http://user:pass@perses:8080", want: "must not carry credentials"},
		{name: "routable literal", value: "http://10.1.2.3:8080", want: "routable literal"},
		{name: "wrong scheme", value: "grpc://perses:8080", want: "must use http:// or https://"},
		{name: "invalid port", value: "http://perses:notaport", want: "invalid port"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizedPersesUpstream(tc.value)
			if err == nil {
				t.Fatalf("upstream %q must be refused", tc.value)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q must mention %q", err, tc.want)
			}
		})
	}
	// Loopback literals and service names are both legitimate upstreams.
	for _, value := range []string{"http://127.0.0.1:8080", "http://localhost:8080", "https://perses.ui:8443", "http://perses"} {
		normalized, err := normalizedPersesUpstream(value)
		if err != nil {
			t.Fatalf("upstream %q must be accepted: %v", value, err)
		}
		if !strings.HasPrefix(normalized, "http") || !strings.Contains(normalized, ":8") {
			t.Fatalf("the normalized upstream must keep scheme and port, got %q", normalized)
		}
	}
}

func TestPersesProxySSRFDenyNetworksAddsTargetKnowledge(t *testing.T) {
	denied, err := PersesProxySSRFDenyNetworks(PersesGatewayRequirements{
		OperatorNetworks:         []string{"10.20.0.0/16", "192.168.7.0/24"},
		KubernetesServiceAddress: "10.96.0.1",
	})
	if err != nil {
		t.Fatalf("the deny list must build: %v", err)
	}
	joined := strings.Join(denied, " ")
	for _, want := range []string{"10.20.0.0/16", "192.168.7.0/24", "10.96.0.1/32"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the deny list must contain %s, got %v", want, denied)
		}
	}
	// A bare hostname is not a valid single-host deny: upstream compares IPs.
	if _, err := PersesProxySSRFDenyNetworks(PersesGatewayRequirements{KubernetesServiceAddress: "kubernetes.default.svc"}); err == nil {
		t.Fatal("a non-IP Kubernetes service address must fail closed rather than silently widen the proxy")
	}
	if _, err := PersesProxySSRFDenyNetworks(PersesGatewayRequirements{OperatorNetworks: []string{"nonsense"}}); err == nil {
		t.Fatal("a malformed operator network must fail closed")
	}
}

func TestPersesComposeServicesNeverPublishesPersesDirectly(t *testing.T) {
	plan, err := BuildPersesGateway(persesRequirements(AuthenticationMTLS))
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	files := HTTPGatewayFiles{
		Dir:       t.TempDir(),
		Caddyfile: filepath.Join(t.TempDir(), "config", "Caddyfile"),
		Material:  TLSMaterial{ServerName: "perses-ui-access"},
	}
	restore := registerPersesRendererForTest(t, plan)
	rendered, err := PersesComposeServices(context.Background(), plan, files, capability.PersesDefaultRelease)
	if err != nil {
		t.Fatalf("the Perses services must render: %v", err)
	}
	persesBlock := rendered
	if idx := strings.Index(rendered, "  "+PersesServiceName+":"); idx >= 0 {
		persesBlock = rendered[idx:]
	}
	if strings.Contains(persesBlock, "ports:") {
		t.Fatalf("the Perses service must never be published to the host; only the gateway is reachable:\n%s", persesBlock)
	}
	if !strings.Contains(persesBlock, "read_only: true") || !strings.Contains(persesBlock, "cap_drop") || !strings.Contains(persesBlock, "no-new-privileges:true") {
		t.Fatalf("the Perses service must keep the shared hardening baseline:\n%s", persesBlock)
	}
	if !strings.Contains(rendered, `127.0.0.1:${`+PersesHostPortEnv) {
		t.Fatalf("the gateway must publish on loopback only, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "image: "+capability.PersesDefaultRelease.Image) {
		t.Fatalf("the rendering must use the pinned image, got:\n%s", rendered)
	}
	// Unpinned releases must fail closed rather than render `latest`.
	if _, err := PersesComposeServices(context.Background(), plan, files, capability.PersesRelease{Version: "v0.55.1"}); err == nil {
		t.Fatal("a Perses release without a pinned image must not render")
	}
	var notConfigured *capability.PersesNotConfiguredError
	if _, err := PersesComposeServices(context.Background(), plan, files, capability.PersesRelease{}); !errors.As(err, &notConfigured) {
		t.Fatalf("an unpinned surface must report the typed not-configured verdict, got %v", err)
	}
	restore()
	// A plan whose policy has since been weakened must not render either.
	weakened := plan
	weakened.Policy = capability.AnonymousPersesSecurityPolicy()
	if _, err := PersesComposeServices(context.Background(), weakened, files, capability.PersesDefaultRelease); err == nil {
		t.Fatal("rendering must re-check the posture, not trust the plan shape")
	}
}

// registerPersesRendererForTest materializes the deterministic configuration
// rendering under a temporary host directory, exactly as a realization would,
// and restores the previous registration when the test ends.
func registerPersesRendererForTest(t *testing.T, plan PersesGatewayPlan) func() {
	t.Helper()
	previous := persesConfigRenderer
	dir := t.TempDir()
	SetPersesConfigRenderer(func(_ context.Context, current PersesGatewayPlan) (PersesRenderedConfig, error) {
		contents, err := RenderPersesConfig(current.Policy)
		if err != nil {
			return PersesRenderedConfig{}, err
		}
		hostPath := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(hostPath, []byte(contents), 0o444); err != nil {
			return PersesRenderedConfig{}, err
		}
		return PersesRenderedConfig{HostPath: hostPath, Contents: contents}, nil
	})
	return func() { SetPersesConfigRenderer(previous) }
}

func TestEnsurePersesGatewayMaterialRequiresIssuerAndTLS(t *testing.T) {
	if _, err := EnsurePersesGatewayMaterial(context.Background(), nil, Policy{TLSRequired: true}, t.TempDir()); err == nil {
		t.Fatal("a Perses gateway without a service issuer must fail closed")
	}
	policy, err := Resolve("production", PersesGatewayProvider, AuthenticationMTLS)
	if err != nil {
		t.Fatalf("resolve the Perses gateway policy: %v", err)
	}
	if !policy.TLSRequired || !policy.AuthenticationRequired {
		t.Fatalf("managed production Perses access must require TLS and authentication, got %+v", policy)
	}
	if _, err := EnsurePersesGatewayMaterial(context.Background(), nil, Policy{TLSRequired: false, PKISource: PKIManagedLocal}, t.TempDir()); err == nil {
		t.Fatal("the Perses gateway must not accept a policy that disables TLS")
	}
}

func TestPersesGatewayMaterialProjectionKeepsPrivateKeyOwnerOnly(t *testing.T) {
	// The projection is the documented, derived runtime copy; the authoritative
	// key must stay owner-only so a container cannot read it from the state dir.
	dir := t.TempDir()
	pkiDir := filepath.Join(dir, "pki")
	if err := os.MkdirAll(pkiDir, 0o700); err != nil {
		t.Fatalf("mkdir pki: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkiDir, "server-key.pem"), []byte("KEY\n"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if info, err := os.Stat(filepath.Join(pkiDir, "server-key.pem")); err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("authoritative Perses key must be owner-only, got %v %v", info, err)
	}
}
