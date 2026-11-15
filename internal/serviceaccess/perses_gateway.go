package serviceaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses management gateway adapter.
//
// Perses is the optional observability management surface. This adapter is the
// only place where a Perses deployment receives its transport and
// authorization requirements, and it enforces three rules taken from the pinned
// release's own security contract:
//
//   - the Perses process is never reachable anonymously: it binds a loopback
//     address that is not published, so an operator faces verified TLS plus the
//     gateway authentication, or nothing;
//   - the gateway refuses the datasource-proxy prefix, because an exposed
//     Perses datasource proxy is an SSRF oracle for everything the Target can
//     reach, and it can leak the plaintext of every secret that datasource
//     references;
//   - credential material is referenced by path and never inlined, so tokens,
//     the encryption key and datasource passwords cannot end up in a rendered
//     manifest, a log line or an audit record.
//
// Every failure is fail-closed and names the remedy.

const (
	// PersesUpstreamPort is the port the Perses server listens on. BaseHarbor
	// pins it to loopback and never publishes it.
	PersesUpstreamPort = 8080
	// PersesGatewayServiceName is the gateway service that owns operator access.
	PersesGatewayServiceName = "perses-ui-access"
	// PersesGatewayProvider is the serviceaccess provider label used for policy
	// resolution, matching the provider-native naming of the other management
	// surfaces.
	PersesGatewayProvider = "perses-management"
	// PersesServiceName is the unexposed Perses container.
	PersesServiceName = "perses"
	// PersesHostPortEnv is the loopback host port the gateway publishes on.
	PersesHostPortEnv = "BASEHARBOR_PERSES_UI_HOST_PORT"
	// PersesHealthAuthorizationEnv carries the gateway health probe credential.
	PersesHealthAuthorizationEnv = "BASEHARBOR_PERSES_HEALTH_AUTHORIZATION"
)

// PersesGatewayRequirements is what the gateway needs to protect one Perses
// surface. It is derived from the contract, never guessed.
type PersesGatewayRequirements struct {
	// Upstream is the URL of the Perses server this gateway fronts. It must be a
	// loopback or Target-internal service address, without credentials.
	Upstream string
	// Policy is the effective Perses security posture. It must validate.
	Policy capability.PersesSecurityPolicy
	// Authentication is the gateway authentication mode offered to operators.
	// Only mTLS and a bearer token are supported by the HTTP gateway.
	Authentication AuthenticationMode
	// AuthTokenFile is the path holding the operator bearer token when
	// Authentication is token. The token itself never appears in this struct.
	AuthTokenFile string
	// ServerNames are additional certificate subject names to mint.
	ServerNames []string
	// OperatorNetworks are Target-internal networks the deployment can reach.
	// The proxy must not be able to reach them, so they are denied.
	OperatorNetworks []string
	// KubernetesServiceAddress is the in-cluster API service address when the
	// Target is a cluster. Upstream denies it only when it is named, so
	// BaseHarbor names it.
	KubernetesServiceAddress string
}

// PersesGatewayPlan is the validated outcome: the hardened HTTP gateway spec,
// the refused route prefixes, and the effective policy that rendering depends
// on.
type PersesGatewayPlan struct {
	Spec HTTPGatewaySpec
	// Policy is the validated posture this plan was derived from.
	Policy capability.PersesSecurityPolicy
	// DeniedPaths lists the route prefixes the gateway refuses. The datasource
	// proxy prefix is always present.
	DeniedPaths []string
	// RequireClientIdentity reports that the gateway must verify a client
	// certificate, i.e. mTLS.
	RequireClientIdentity bool
	// ProxyDenyReason explains why the datasource proxy is refused. Empty means
	// the policy validates; the route stays denied by default.
	ProxyDenyReason string
	// UpstreamBindAddress is the loopback bind address rendered for the Perses
	// process so it can never be reached without the gateway.
	UpstreamBindAddress string
}

// BuildPersesGateway derives the protected access configuration for one Perses
// surface. It validates the release pin and the security policy before emitting
// anything, so a caller cannot accidentally deploy an anonymous Perses.
func BuildPersesGateway(requirements PersesGatewayRequirements) (PersesGatewayPlan, error) {
	upstream, err := normalizedPersesUpstream(requirements.Upstream)
	if err != nil {
		return PersesGatewayPlan{}, err
	}
	policyError := capability.ValidatePersesSecurityPolicy(requirements.Policy)
	if policyError != nil {
		return PersesGatewayPlan{}, policyError
	}
	var authentication AuthenticationMode
	switch requirements.Authentication {
	case AuthenticationMTLS, AuthenticationToken:
		authentication = requirements.Authentication
	case "":
		return PersesGatewayPlan{}, fmt.Errorf("Perses management gateway authentication is required: select mTLS or a bearer token, otherwise operator access is anonymous")
	default:
		return PersesGatewayPlan{}, fmt.Errorf("Perses management gateway authentication %q is not supported: use mTLS or a bearer token", requirements.Authentication)
	}
	if authentication == AuthenticationToken && strings.TrimSpace(requirements.AuthTokenFile) == "" {
		return PersesGatewayPlan{}, fmt.Errorf("Perses management gateway token authentication requires %s", EnvAuthTokenFile)
	}
	denied := []string{capability.PersesProxyPrefix}
	plan := PersesGatewayPlan{
		Policy:                requirements.Policy,
		DeniedPaths:           denied,
		RequireClientIdentity: authentication == AuthenticationMTLS,
		UpstreamBindAddress:   "127.0.0.1",
	}
	spec := HTTPGatewaySpec{
		ServiceName:            PersesGatewayServiceName,
		Upstream:               upstream,
		ContainerPort:          8443,
		PublishedPortEnv:       PersesHostPortEnv,
		CertificateNames:       requirements.ServerNames,
		RequireClient:          plan.RequireClientIdentity,
		DenyPaths:              denied,
		HealthURI:              "/api/v1/health",
		HealthStatus:           200,
		HealthAuthorizationEnv: PersesHealthAuthorizationEnv,
	}
	plan.Spec = spec
	return plan, nil
}

func normalizedPersesUpstream(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("Perses management gateway upstream is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse Perses management gateway upstream: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("Perses management gateway upstream %q must use http:// or https://", raw)
	}
	if parsed.User != nil {
		return "", errors.New("Perses management gateway upstream must not carry credentials: operator authentication belongs to the gateway and datasource secrets stay with the provider")
	}
	host := strings.TrimSuffix(strings.TrimPrefix(parsed.Hostname(), "["), "]")
	if host == "" {
		return "", fmt.Errorf("Perses management gateway upstream %q has no host", raw)
	}
	port := parsed.Port()
	if port == "" {
		port = strconv.Itoa(PersesUpstreamPort)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("Perses management gateway upstream %q has an invalid port", raw)
	}
	if addr, err := netip.ParseAddr(host); err == nil && !addr.Unmap().IsLoopback() {
		return "", fmt.Errorf("Perses management gateway upstream %q is a routable literal address: bind Perses to loopback or a Target-internal service name so only the gateway can reach it", raw)
	}
	return scheme + "://" + host + ":" + port, nil
}

// PersesDeniedProxyPaths reports the route prefixes a gateway must refuse for a
// policy. It never publishes the datasource proxy without a valid pin and
// posture, and it denies the proxy even when the policy validates: operators
// reach datasources through the panels, not by browsing a raw proxy.
func PersesDeniedProxyPaths(policy capability.PersesSecurityPolicy) ([]string, string) {
	if err := capability.ValidatePersesSecurityPolicy(policy); err != nil {
		return []string{capability.PersesProxyPrefix}, err.Error()
	}
	return []string{capability.PersesProxyPrefix}, ""
}

// PersesProxySSRFDenyNetworks returns the networks a Perses instance on this
// Target must never proxy to. The pinned release already denies loopback,
// link-local, cloud-metadata, multicast and reserved ranges, so this adds only
// what Target knowledge contributes: the operator networks and the in-cluster
// Kubernetes API service address.
func PersesProxySSRFDenyNetworks(requirements PersesGatewayRequirements) ([]string, error) {
	denied := make([]string, 0, len(requirements.OperatorNetworks)+1)
	for _, entry := range requirements.OperatorNetworks {
		prefix, err := parsePersesNetwork("operator", entry)
		if err != nil {
			return nil, err
		}
		denied = append(denied, prefix.String())
	}
	if raw := strings.TrimSpace(requirements.KubernetesServiceAddress); raw != "" {
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			return nil, fmt.Errorf("Perses Kubernetes service address %q is not an IP address: upstream denies it as a single host, so it must be exact", raw)
		}
		addr = addr.Unmap()
		denied = append(denied, netip.PrefixFrom(addr, addr.BitLen()).String())
	}
	return denied, nil
}

func parsePersesNetwork(label, raw string) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Prefix{}, fmt.Errorf("Perses %s network is empty", label)
	}
	if addr, err := netip.ParseAddr(raw); err == nil {
		addr = addr.Unmap()
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("Perses %s network %q is not a CIDR prefix or IP address", label, raw)
	}
	return prefix, nil
}

// PersesComposeServices renders the Perses service and its gateway. Perses binds
// loopback inside its own network namespace and is never published; the gateway
// is the only host-visible address and carries the TLS and authentication
// requirements from the plan.
func PersesComposeServices(plan PersesGatewayPlan, files HTTPGatewayFiles, release capability.PersesRelease) (string, error) {
	if err := capability.ValidatePersesSecurityPolicy(plan.Policy); err != nil {
		return "", err
	}
	if strings.TrimSpace(release.Image) == "" {
		return "", &capability.PersesNotConfiguredError{}
	}
	if strings.TrimSpace(files.Caddyfile) == "" {
		return "", errors.New("Perses management gateway requires its Caddyfile before rendering")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s:\n", PersesServiceName)
	fmt.Fprintf(&b, "    image: %s\n", release.Image)
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - /tmp:rw,noexec,nosuid,nodev\n")
	// Deliberately no `ports:` block: the gateway is the only reachable address.
	b.WriteString("    expose:\n")
	fmt.Fprintf(&b, "      - \"%d\"\n", PersesUpstreamPort)
	// The configuration is rendered by the realization and mounted read-only;
	// BaseHarbor never rewrites it with a shell so that a rendering stays
	// deterministic and free of credential interpolation. Binding loopback is
	// the flag the product documents for this purpose.
	b.WriteString("    command:\n")
	fmt.Fprintf(&b, "      - --config=/etc/perses/config.yaml\n")
	fmt.Fprintf(&b, "      - --web.listen-address=%s:%d\n", plan.UpstreamBindAddress, PersesUpstreamPort)
	b.WriteString(HTTPGatewayComposeService(files, plan.Spec))
	return b.String(), nil
}

// PersesGatewayCaddyDirectives renders the refused datasource-proxy routes and
// reports the SHA-256 of the rendering so reconciliation can detect drift
// without printing credential material.
func PersesGatewayCaddyDirectives(plan PersesGatewayPlan) (string, string, error) {
	if plan.RequireClientIdentity && !plan.Spec.RequireClient {
		return "", "", errors.New("Perses management gateway plan requires a client certificate but the spec does not enforce it")
	}
	var b strings.Builder
	for _, path := range plan.DeniedPaths {
		path = strings.TrimSpace(path)
		if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n{}") {
			return "", "", fmt.Errorf("Perses management gateway denied path %q is invalid", path)
		}
		fmt.Fprintf(&b, "  respond %s* 404\n", path)
	}
	rendered := b.String()
	sum := sha256.Sum256([]byte(rendered))
	return rendered, "sha256:" + hex.EncodeToString(sum[:]), nil
}

// EnsurePersesGatewayMaterial projects the TLS material the Perses gateway
// needs, reusing the managed-local PKI of the service-access authority. Private
// key material stays owner-only under pki/; only the read-only runtime
// projection is readable by the unprivileged gateway UID, and reconciliation
// regenerates it from the protected source.
func EnsurePersesGatewayMaterial(ctx context.Context, issuer Issuer, policy Policy, providerDir string, serverNames ...string) (TLSMaterial, error) {
	if issuer == nil {
		return TLSMaterial{}, errors.New("Perses management gateway requires a service issuer")
	}
	if !policy.TLSRequired {
		return TLSMaterial{}, errors.New("BaseHarbor managed service access cannot disable TLS")
	}
	names := append([]string{"localhost", "127.0.0.1"}, serverNames...)
	dir := filepath.Join(providerDir, "service-access")
	if err := ensureDir(dir); err != nil {
		return TLSMaterial{}, err
	}
	material, err := EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), names...)
	if err != nil {
		return TLSMaterial{}, err
	}
	return projectGatewayMaterial(dir, material)
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create Perses service access state: %w", err)
	}
	return nil
}
