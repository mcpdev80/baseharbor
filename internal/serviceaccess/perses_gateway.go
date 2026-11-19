package serviceaccess

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
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
	// PersesConfigContainerPath is the file the rendered command names with
	// --config. It is a mount target, never a path BaseHarbor writes into with
	// a shell.
	PersesConfigContainerPath = "/etc/perses/config.yaml"
	// PersesConfigMountDir is the container directory the rendered host
	// configuration directory is bind-mounted to, read-only. Its parent holds
	// the mount target.
	PersesConfigMountDir = "/etc/perses"
	// PersesEncryptionKeyMountDir is the container directory the key
	// projection's directory is bind-mounted to, read-only. It holds
	// PersesEncryptionKeyContainerPath, the file the rendered configuration
	// references (never inlines) as security.encryption_key_file.
	PersesEncryptionKeyMountDir = "/etc/perses/secrets"
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
	// ServiceAddress is the deployment-scoped backend address the gateway
	// routes to for this surface (the compose project's Perses service alias
	// in the shared-namespace topology). Realization state, not posture:
	// BuildPersesGateway copies it onto the plan; the empty form (pure
	// posture projections) keeps the configuration statement out.
	ServiceAddress string
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
	// ServiceAddress is the deployment-scoped backend-network address this
	// gateway routes to (the compose project's Perses service alias in the
	// shared-namespace topology). It is realization state, NOT posture, so it
	// lives on the plan in this package — the capability security policy is
	// the accepted 0428-084 contract surface and stays deployment-free.
	ServiceAddress string
	// EncryptionKeyFile is the host path the realization materializes the
	// datasource encryption key at, container-readable. A reference only,
	// never a value; an empty path fails rendering closed.
	EncryptionKeyFile string
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
	bindAddress := strings.TrimSpace(requirements.PersesBindAddress)
	switch bindAddress {
	case "", "127.0.0.1", "localhost":
		bindAddress = "127.0.0.1"
	default:
		if addr, addrErr := netip.ParseAddr(bindAddress); addrErr == nil && addr.Unmap().IsLoopback() {
			if !strings.EqualFold(upstreamHost, bindAddress) {
				return PersesGatewayPlan{}, fmt.Errorf("Perses bind address %q is loopback of a namespace the gateway does not join and does not match the upstream host %q: the peer can never reach it", bindAddress, upstreamHost)
			}
		} else if !validPersesBindAddress(bindAddress) {
			return PersesGatewayPlan{}, fmt.Errorf("Perses bind address %q is not a literal address or service address", bindAddress)
		}
	}
	// The shared-namespace form is the loopback bind paired with an upstream
	// that is NOT a loopback literal: the gateway reaches the peer over a
	// routable name (the backend-network service alias) while Perses itself
	// stays bound to the namespace loopback. An upstream loopback literal is
	// the namespace-local form, which the rendering must not publish.
	sharedNetwork := true
	if addr, addrErr := netip.ParseAddr(upstreamHost); addrErr == nil && addr.Unmap().IsLoopback() {
		sharedNetwork = false
	}
	// The shared-namespace form always pins the namespace loopback on the
	// command line; the configuration statement names the service address the
	// gateway routes to instead. Keeping the two values distinct (and derived
	// from one plan) is what lets the compose rendering prove the mounted
	// file byte-for-byte against persisted state alone.

	plan := PersesGatewayPlan{
		ServiceAddress:        strings.TrimSpace(requirements.ServiceAddress),
		Policy:                requirements.Policy,
		DeniedPaths:           denied,
		RequireClientIdentity: authentication == AuthenticationMTLS,
		UpstreamBindAddress:   bindAddress,
		SharedGatewayNetwork:  sharedNetwork,
	}
	if sharedNetwork {
		// The shared-namespace form always pins the namespace loopback on the
		// command line; the configuration statement names the service address
		// the gateway routes to instead. Keeping the two values distinct (and
		// derived from one plan) is what lets the compose rendering prove the
		// mounted file byte-for-byte against persisted state alone.
		plan.ServiceAddress = upstreamHost
	}
	spec := HTTPGatewaySpec{
		ServiceName: PersesGatewayServiceName,
		Upstream:    upstream,
		// fix-7 B1 (variant A): the Perses service definition carries
		// network_mode (Docker Compose forbids network_mode AND networks on
		// one service), so it registers no backend alias of its own. The
		// alias the gateway routes to is registered on the gateway service
		// instead, in the first network entry it joins; the shared-namespace
		// topology stays decided by the non-loopback upstream form above —
		// the upstream is never rewritten to a loopback literal.
		// fix-7 B1 (variant A): the Perses service definition carries
		// network_mode (Docker Compose forbids network_mode AND networks on
		// one service), so it registers no backend alias of its own. The
		// alias the gateway routes to is registered on the gateway service
		// instead — HTTPGatewayComposeService emits it under the FIRST
		// network entry — and it exists only in the shared form, where the
		// upstream names a routable service address rather than a loopback
		// literal. The namespace-local form reaches the peer inside its own
		// namespace and needs no alias.
		NetworkAliases:         persesGatewayNetworkAliases(sharedNetwork, upstreamHost),
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

// persesGatewayNetworkAliases is the fix-7 B1 alias rule: the shared form
// registers the upstream's host (the deployment's Perses service alias) on the
// gateway service's first network; the namespace-local form registers none.
func persesGatewayNetworkAliases(sharedNetwork bool, upstreamHost string) []string {
	if !sharedNetwork {
		return nil
	}
	return []string{upstreamHost}
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
