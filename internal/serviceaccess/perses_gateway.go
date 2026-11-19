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
	"regexp"
	"strconv"
	"strings"
	"sync"

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
	// PersesConfigContainerPath is the file the rendered command names with
	// --config. It is a mount target, never a path BaseHarbor writes into with
	// a shell.
	PersesConfigContainerPath = "/etc/perses/config.yaml"
	// PersesConfigMountDir is the container directory the rendered host
	// configuration directory is bind-mounted to, read-only. Its parent holds
	// the mount target.
	PersesConfigMountDir = "/etc/perses"
)

// PersesGatewayRequirements is what the gateway needs to protect one Perses
// surface. It is derived from the contract, never guessed.
type PersesGatewayRequirements struct {
	// Upstream is the URL of the Perses server this gateway fronts. It must be a
	// loopback or Target-internal service address, without credentials.
	Upstream string
	// PersesBindAddress is the address the Perses process binds. The empty
	// value is the shared-namespace form: Perses binds the loopback of the
	// network namespace its gateway container joins, so the port can never be
	// host-forwarded and the gateway still reaches it. A non-loopback value
	// must name the exact address the gateway upstream routes to.
	PersesBindAddress string
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
	// UpstreamBindAddress is the address the Perses process binds so it can
	// never be reached without the gateway. With SharedGatewayNetwork it is the
	// loopback of the namespace the gateway joins; otherwise it is the
	// Target-internal service address rendered into --web.listen-address.
	UpstreamBindAddress string
	// SharedGatewayNetwork reports that the Perses service definition renders
	// network_mode into the gateway container's namespace. It is how the
	// loopback upstream of BuildPersesGateway becomes reachable without ever
	// publishing the Perses port on the host.
	SharedGatewayNetwork bool
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
	// Two supported topologies, both fail-closed:
	//   - shared namespace (the gateway adapter default): Perses binds the
	//     loopback of the network namespace its gateway container joins
	//     (compose network_mode), the gateway upstream is the Perses service
	//     address, and the Perses port can never be host-forwarded;
	//   - namespace-local loopback: only when the caller names the exact
	//     upstream the bind matches (a deployment sharing one namespace end
	//     to end); the service definition then publishes no network_mode and
	//     no ports at all.
	// A mismatched loopback pair strands the peer and a routable literal bind
	// makes Perses itself reachable; both fail closed here.
	upstreamURL, err := url.Parse(upstream)
	if err != nil {
		return PersesGatewayPlan{}, fmt.Errorf("parse normalized Perses gateway upstream: %w", err)
	}
	upstreamHost := strings.TrimSuffix(strings.TrimPrefix(upstreamURL.Hostname(), "["), "]")
	sharedNetwork := true
	if addr, addrErr := netip.ParseAddr(upstreamHost); addrErr == nil && addr.Unmap().IsLoopback() {
		sharedNetwork = false
	}
	bindAddress := strings.TrimSpace(requirements.PersesBindAddress)
	switch bindAddress {
	case "", "127.0.0.1", "localhost":
		bindAddress = "127.0.0.1"
	default:
		if addr, addrErr := netip.ParseAddr(bindAddress); addrErr == nil && addr.Unmap().IsLoopback() {
			if !strings.EqualFold(upstreamHost, bindAddress) {
				return PersesGatewayPlan{}, fmt.Errorf("Perses bind address %q is loopback of a namespace the gateway does not join and does not match the upstream host %q: the peer can never reach it", bindAddress, upstreamHost)
			}
			sharedNetwork = false
		} else if !validPersesBindAddress(bindAddress) {
			return PersesGatewayPlan{}, fmt.Errorf("Perses bind address %q is not a literal address or service address", bindAddress)
		} else {
			sharedNetwork = false
		}
	}
	plan := PersesGatewayPlan{
		Policy:                requirements.Policy,
		DeniedPaths:           denied,
		RequireClientIdentity: authentication == AuthenticationMTLS,
		UpstreamBindAddress:   bindAddress,
		SharedGatewayNetwork:  sharedNetwork,
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
	normalized := scheme + "://" + host + ":" + port
	if addr, addrErr := netip.ParseAddr(host); addrErr == nil {
		if !addr.Unmap().IsLoopback() {
			return "", fmt.Errorf("Perses management gateway upstream %q is a routable literal address: bind Perses to loopback or a Target-internal service name so only the gateway can reach it", raw)
		}
		return normalized, nil
	}
	if !validPersesBindAddress(host) {
		return "", fmt.Errorf("Perses management gateway upstream %q has an invalid host", raw)
	}
	return normalized, nil
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

// validPersesBindAddress accepts a literal address or a DNS service name:
// exactly what --web.listen-address takes.
func validPersesBindAddress(raw string) bool {
	if raw == "" {
		return false
	}
	if _, err := netip.ParseAddr(raw); err == nil {
		return true
	}
	_, err := netip.ParsePrefix(raw)
	return err != nil && persesAllowedBindPattern.MatchString(raw)
}

var persesAllowedBindPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

// PersesRenderedConfig is one rendered Perses configuration file plus the host
// path the realization must materialize it at. Contents carry no credential
// material: they reference files only.
type PersesRenderedConfig struct {
	// HostPath is where the realization writes the rendering; the rendered
	// service definition bind-mounts its directory at PersesConfigContainerPath,
	// read-only.
	HostPath string
	// Contents is the deterministic YAML of the effective policy.
	Contents string
}

// PersesConfigRenderer materializes the Perses configuration file for a plan.
// It is the producer half of the --config=/etc/perses/config.yaml contract:
// without a renderer wired into the deployment there is no configuration to
// mount, and BuildPersesGateway refuses to render service definitions that
// would silently fall back to the upstream anonymous default posture.
type PersesConfigRenderer func(context.Context, PersesGatewayPlan) (PersesRenderedConfig, error)

// SetPersesConfigRenderer registers the realization's renderer. It must be set
// before any deployment renders the optional Perses surface; a nil renderer is
// an explicit statement that no realization can materialize the file, and the
// compose rendering then fails closed.
func SetPersesConfigRenderer(renderer PersesConfigRenderer) {
	persesConfigRendererMu.Lock()
	defer persesConfigRendererMu.Unlock()
	persesConfigRenderer = renderer
}

// PersesConfigRendererRegistered reports whether a realization provides the
// configuration producer the rendered service definition consumes.
func PersesConfigRendererRegistered() bool {
	persesConfigRendererMu.RLock()
	defer persesConfigRendererMu.RUnlock()
	return persesConfigRenderer != nil
}

var (
	persesConfigRendererMu sync.RWMutex
	persesConfigRenderer   PersesConfigRenderer
)

// PersesComposeServices renders the Perses service and its gateway. The Perses
// process binds the address from the plan and is never published; the gateway
// is the only host-visible address and carries the TLS and authentication
// requirements from the plan. The configuration the command line names is
// rendered from the validated policy by the registered realization renderer
// and mounted read-only, so the posture the contract verified is the posture
// the process actually serves.
func PersesComposeServices(ctx context.Context, plan PersesGatewayPlan, files HTTPGatewayFiles, release capability.PersesRelease) (string, error) {
	if err := capability.ValidatePersesSecurityPolicy(plan.Policy); err != nil {
		return "", err
	}
	if strings.TrimSpace(release.Image) == "" {
		return "", &capability.PersesNotConfiguredError{}
	}
	if strings.TrimSpace(files.Caddyfile) == "" {
		return "", errors.New("Perses management gateway requires its Caddyfile before rendering")
	}
	persesConfigRendererMu.RLock()
	renderer := persesConfigRenderer
	persesConfigRendererMu.RUnlock()
	if renderer == nil {
		return "", errors.New("Perses service rendering requires a registered configuration renderer: the --config file must have a proven producer, otherwise the deployment silently runs the upstream anonymous default posture")
	}
	configured, err := renderer(ctx, plan)
	if err != nil {
		return "", err
	}
	if expected, err := RenderPersesConfig(plan.Policy); err != nil {
		return "", err
	} else if configured.Contents != expected {
		return "", fmt.Errorf("the registered Perses configuration renderer did not produce the deterministic rendering of the validated policy for %s; refusing to mount it", configured.HostPath)
	}
	if err := validatePersesRenderedConfig(plan, configured); err != nil {
		return "", err
	}
	var b strings.Builder
	// The gateway container is rendered first because the Perses service
	// definition joins its network namespace.
	gateway := HTTPGatewayComposeService(files, plan.Spec)
	gatewayName := strings.TrimSpace(plan.Spec.ServiceName)
	if gatewayName == "" || !strings.HasPrefix(gateway, "  "+gatewayName+":") {
		return "", errors.New("Perses management gateway spec must name its service before rendering")
	}
	b.WriteString(gateway)
	fmt.Fprintf(&b, "  %s:\n", PersesServiceName)
	fmt.Fprintf(&b, "    image: %s\n", release.Image)
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	if !plan.SharedGatewayNetwork && (strings.TrimSpace(plan.UpstreamBindAddress) == "" || strings.TrimSpace(plan.UpstreamBindAddress) == "0.0.0.0") {
		return "", errors.New("Perses service rendering requires the bind address the plan verifies")
	}
	if plan.SharedGatewayNetwork {
		// The loopback bind is only meaningful inside the namespace the gateway
		// joins, and it is what keeps the Perses port unforwardable on the
		// host. Deliberately no `ports:` and no `networks:` block for Perses.
		fmt.Fprintf(&b, "    network_mode: %s\n", strconv.Quote("service:"+gatewayName))
	} else {
		// Namespace-local loopback: no published port, no expose, no network —
		// nothing can route to Perses from outside its own namespace.
		b.WriteString("    networks: []\n")
	}
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - /tmp:rw,noexec,nosuid,nodev\n")
	b.WriteString("      - /etc/perses/database:rw,noexec,nosuid,nodev\n")
	// The configuration is rendered by the realization and mounted read-only;
	// BaseHarbor never rewrites it with a shell so that a rendering stays
	// deterministic and free of credential interpolation. Binding loopback is
	// the flag the product documents for this purpose.
	b.WriteString("    command:\n")
	fmt.Fprintf(&b, "      - --config=%s\n", PersesConfigContainerPath)
	fmt.Fprintf(&b, "      - --web.listen-address=%s:%d\n", plan.UpstreamBindAddress, PersesUpstreamPort)
	fmt.Fprintf(&b, "    volumes:\n")
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(composeBindSource(filepath.Dir(configured.HostPath))+":"+PersesConfigMountDir+":ro"))
	return b.String(), nil
}

// validatePersesRenderedConfig proves the provenance chain of one rendering:
// the file exists at the path the renderer declared, it is the deterministic
// rendering of the plan's effective policy, and it does not carry credential
// material. A renderer that writes elsewhere, drifts from the policy or
// interpolates secrets fails the deployment closed.
func validatePersesRenderedConfig(plan PersesGatewayPlan, configured PersesRenderedConfig) error {
	if strings.TrimSpace(configured.HostPath) == "" || !filepath.IsAbs(configured.HostPath) {
		return errors.New("Perses configuration renderer must declare an absolute host path")
	}
	contents, err := os.ReadFile(configured.HostPath)
	if err != nil {
		return fmt.Errorf("Perses configuration renderer declared %s but the file is not materialized: %w", configured.HostPath, err)
	}
	if string(contents) != configured.Contents {
		return fmt.Errorf("Perses configuration at %s does not match the renderer output; the mounted file must be the deterministic rendering", configured.HostPath)
	}
	if expected, err := RenderPersesConfig(plan.Policy); err != nil {
		return err
	} else if string(contents) != expected {
		return fmt.Errorf("Perses configuration at %s is not the deterministic rendering of the validated policy", configured.HostPath)
	}
	return nil
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
