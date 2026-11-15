package capability

import (
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Perses security contract for the selected upstream release.
//
// Perses v0.55.0 fixed a set of vulnerabilities that are reachable with the
// shipped default configuration, among them JWT forgery, SSRF and credential
// leaks through the datasource proxy, and arbitrary file reads. The same release
// made two conditions mandatory that BaseHarbor must therefore never weaken:
//
//   - when authentication is enabled, an encryption key must be provided,
//     otherwise Perses refuses to start;
//   - the datasource proxy denies loopback, link-local, cloud-metadata and
//     private destinations by default, and it no longer forwards the caller's
//     own credentials to the datasource.
//
// Perses also ships with authentication DISABLED by default, which makes every
// API endpoint - including the datasource proxy - reachable without
// credentials. BaseHarbor MUST NEVER deploy that default: an anonymous Perses
// instance is not a management surface, it is an unauthenticated proxy into the
// Target network. Everything below is fail-closed for exactly that reason.
//
// These are pure contract functions. They keep the security vocabulary in the
// same package as the version axes it belongs to, so a provider realization can
// never drift from the pinned release it claims to support.

// persesMinSecureVersion is the earliest Perses product version carrying the
// datasource-proxy SSRF and credential-forwarding fixes. A pin below it must
// never be reported as a supported surface.
const persesMinSecureVersion = "v0.55.0"

// PersesProxyPrefix is the path prefix under which the Perses server forwards
// requests to datasource URLs. It is the SSRF-relevant surface of the product
// and the reason the gateway must never publish an anonymous Perses instance.
const PersesProxyPrefix = "/proxy/"

// PersesSecurityPolicy is the effective security posture of one selected Perses
// management surface. It is the input BaseHarbor passes to the provider
// realization and the object the contract validates before anything is rendered
// or deployed.
type PersesSecurityPolicy struct {
	// Release is the pinned product version and image of the surface.
	Release PersesRelease
	// AuthenticationEnabled corresponds to Perses `security.enable_auth`.
	// BaseHarbor requires it: the upstream default (false) exposes the whole
	// API, including the datasource proxy, without credentials.
	AuthenticationEnabled bool
	// EncryptionKeyDeclared reports whether `security.encryption_key` (or its
	// *_file form) is configured. Perses requires it once authentication is on.
	// The key material itself never belongs in this struct, a manifest or a log.
	EncryptionKeyDeclared bool
	// DenyPrivateNetworks corresponds to `datasource.proxy.deny_private_networks`.
	// When false the proxy may still reach RFC1918 destinations, which on a
	// BaseHarbor Target are another application's network.
	DenyPrivateNetworks bool
	// AllowedNetworks and DeniedNetworks are the explicit proxy network lists
	// (`datasource.proxy.allowed_networks` / `.denied_networks`).
	AllowedNetworks []string
	DeniedNetworks  []string
	// AllowedHosts is the exhaustive host allowlist
	// (`datasource.proxy.allowed_hosts`). When non-empty it replaces any
	// implicit "everything the server can reach" allowance.
	AllowedHosts []string
	// AllowedSchemes restricts `datasource.proxy.http.allowed_schemes`. An empty
	// list means the upstream default, which still includes plain http.
	AllowedSchemes []string
	// ForwardUserCredentials would re-enable forwarding the caller's own
	// Authorization material to datasources. BaseHarbor has no supported use for
	// it; any value other than false fails closed.
	ForwardUserCredentials bool
	// SecretFileAllowedDirectories enables file-referenced secrets
	// (`security.secret_file_allowed_directories`). When empty, Perses rejects
	// every file reference in a secret, which is the safe default.
	SecretFileAllowedDirectories []string
	// ProvisioningEnforced reports that datasource creation happens only through
	// BaseHarbor-owned provisioning, so an authenticated operator cannot add a
	// datasource pointing at an arbitrary host.
	ProvisioningEnforced bool
	// ReadOnlyMode corresponds to `security.readonly`, disabling the mutating
	// API. Optional: it narrows the blast radius but is never required.
	ReadOnlyMode bool
}

// DefaultPersesSecurityPolicy is the strict posture BaseHarbor requires for the
// optional Perses surface: authentication on, encryption key supplied, private
// and reserved destinations denied for the proxy, only https datasource schemes,
// caller credentials never forwarded, and datasources owned by provisioning.
func DefaultPersesSecurityPolicy() PersesSecurityPolicy {
	return PersesSecurityPolicy{
		Release:                PersesDefaultRelease,
		AuthenticationEnabled:  true,
		EncryptionKeyDeclared:  true,
		DenyPrivateNetworks:    true,
		AllowedSchemes:         []string{"https"},
		ForwardUserCredentials: false,
		ProvisioningEnforced:   true,
	}
}

// AnonymousPersesSecurityPolicy is the upstream default configuration: no
// authentication, no encryption key, unrestricted proxy destinations. It exists
// so the contract can prove that this posture is rejected rather than merely
// described. It is never a valid BaseHarbor deployment input.
func AnonymousPersesSecurityPolicy() PersesSecurityPolicy {
	return PersesSecurityPolicy{Release: PersesDefaultRelease}
}

// ValidatePersesSecurityPolicy checks the pinned release and the effective
// security posture together. Every failure is a fail-closed statement about the
// surface: it is unsupported, never "configured".
func ValidatePersesSecurityPolicy(policy PersesSecurityPolicy) error {
	if err := ValidatePersesRelease(policy.Release); err != nil {
		return err
	}
	if err := ValidatePersesVersionFloor(policy.Release.Version); err != nil {
		return err
	}
	if !policy.AuthenticationEnabled {
		return &PersesAnonymousExposureError{}
	}
	if !policy.EncryptionKeyDeclared {
		return fmt.Errorf("Perses management surface has authentication enabled without an encryption key: the pinned %s release refuses to start, and datasource secrets would fall back to the upstream default key", policy.Release.Version)
	}
	if policy.ForwardUserCredentials {
		return fmt.Errorf("Perses management surface must not forward user credentials to datasources: %s and later deny this by default", persesMinSecureVersion)
	}
	if err := validatePersesProxyRestrictions(policy); err != nil {
		return err
	}
	for _, directory := range policy.SecretFileAllowedDirectories {
		if !strings.HasPrefix(strings.TrimSpace(directory), "/") {
			return fmt.Errorf("Perses secret file allowed directory %q must be an absolute path", directory)
		}
	}
	return nil
}

// ValidatePersesVersionFloor rejects a pin older than the release that carries
// the datasource-proxy SSRF and credential-forwarding fixes.
func ValidatePersesVersionFloor(version string) error {
	parsed, err := parsePersesSemVer(version)
	if err != nil {
		return err
	}
	minimum, err := parsePersesSemVer(persesMinSecureVersion)
	if err != nil {
		return err
	}
	if comparePersesSemVer(parsed, minimum) < 0 {
		return fmt.Errorf("Perses release %s predates %s and lacks the datasource-proxy SSRF and credential-forwarding fixes; the management surface is unsupported", version, persesMinSecureVersion)
	}
	return nil
}

// PersesAnonymousExposureError is the fail-closed verdict for a Perses surface
// that would expose its API, and through it the datasource proxy, without
// authentication.
type PersesAnonymousExposureError struct{}

func (*PersesAnonymousExposureError) Error() string {
	return "Perses management surface would be exposed without authentication: enable it only behind a verified authentication provider, otherwise every caller can create datasources and use the proxy to reach services the Target can see"
}

func validatePersesProxyRestrictions(policy PersesSecurityPolicy) error {
	for _, host := range policy.AllowedHosts {
		if err := validatePersesAllowedHost(host); err != nil {
			return err
		}
	}
	for label, networks := range map[string][]string{"allowed": policy.AllowedNetworks, "denied": policy.DeniedNetworks} {
		for _, entry := range networks {
			if _, err := parsePersesNetwork(label, entry); err != nil {
				return err
			}
		}
	}
	for _, scheme := range policy.AllowedSchemes {
		scheme = strings.ToLower(strings.TrimSpace(scheme))
		if scheme != "http" && scheme != "https" {
			return fmt.Errorf("Perses datasource proxy scheme %q is not supported: only http and https are valid", scheme)
		}
	}
	if len(policy.AllowedHosts) == 0 && !policy.DenyPrivateNetworks && !policy.ProvisioningEnforced {
		return fmt.Errorf("Perses datasource proxy is unrestricted: set an allowed_hosts list, deny private networks, or keep datasource creation in BaseHarbor-owned provisioning")
	}
	return nil
}

var persesAllowedHostPattern = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

func validatePersesAllowedHost(host string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || strings.ContainsAny(host, ":/") {
		return fmt.Errorf("Perses datasource proxy allowed host %q is invalid: allowed_hosts takes hostnames only, without a port or path; put IP addresses in allowed_networks", host)
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return fmt.Errorf("Perses datasource proxy allowed host %q is an IP address: allowed_hosts takes hostnames only, put the address in allowed_networks", host)
	}
	if !persesAllowedHostPattern.MatchString(host) {
		return fmt.Errorf("Perses datasource proxy allowed host %q is invalid: use an exact hostname or a *.subdomain wildcard", host)
	}
	return nil
}

// parsePersesNetwork accepts a CIDR prefix or a bare IP address and returns the
// prefix that covers it, so a single-host entry and a range compare correctly.
func parsePersesNetwork(label, raw string) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Prefix{}, fmt.Errorf("Perses datasource proxy %s network is empty", label)
	}
	if addr, err := netip.ParseAddr(raw); err == nil {
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("Perses datasource proxy %s network %q is not a CIDR prefix or IP address", label, raw)
	}
	return prefix, nil
}

// persesDenyByDefault lists the destinations the pinned release denies without
// any operator configuration. It mirrors the upstream default deny set so a
// BaseHarbor gateway and the Perses server can never disagree about it; the
// authoritative list is the v0.55 configuration documentation.
//
// Kubernetes service and in-cluster mesh addresses are deliberately absent:
// their ranges are site-specific, so a realization that knows the Target must
// deny the Kubernetes API service address through DeniedNetworks itself.
var persesDenyByDefault = []struct {
	Prefix string
	Reason string
}{
	{"127.0.0.0/8", "loopback"},
	{"::1/128", "loopback"},
	{"0.0.0.0/8", "this network"},
	{"::/128", "unspecified"},
	{"169.254.0.0/16", "link-local"},
	{"fe80::/10", "link-local"},
	{"169.254.169.254/32", "cloud metadata and credentials endpoint"},
	{"169.254.170.2/32", "AWS ECS container credentials"},
	{"169.254.170.23/32", "AWS EKS Pod Identity"},
	{"fd00:ec2::23/128", "AWS EKS Pod Identity (IPv6)"},
	{"fd00:ec2::254/128", "AWS IPv6 metadata"},
	{"100.100.100.200/32", "Alibaba Cloud metadata"},
	{"168.63.129.16/32", "Azure WireServer"},
	{"192.0.0.192/32", "Oracle Cloud legacy metadata"},
	{"224.0.0.0/4", "multicast"},
	{"240.0.0.0/4", "reserved"},
	{"ff00::/8", "multicast"},
	{"::/96", "deprecated IPv4-compatible IPv6"},
	{"2001::/32", "Teredo"},
	{"64:ff9b:1::/48", "NAT64 local use"},
}

// persesPrivateNetworks mirrors the ranges upstream documents as "private" for
// the datasource proxy. They are the operator's own application networks, so
// they need an explicit allow or deny rather than a silent pass-through.
var persesPrivateNetworks = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"100.64.0.0/10",
	"fc00::/7",
	"fec0::/10",
}

// PersesDeniedDestination is one destination the datasource proxy must refuse,
// with the reason it is refused.
type PersesDeniedDestination struct {
	Host   string
	Reason string
}

// PersesProxyDecision is the verdict for one candidate datasource endpoint.
type PersesProxyDecision struct {
	// Allowed reports whether static inspection can let the request proceed. It
	// is never true for an undecidable destination.
	Allowed bool
	// VerifiedAtConnection reports that the endpoint carries a hostname whose
	// addresses must be re-checked at connection time, as the pinned release
	// does. Static inspection alone cannot clear a name.
	VerifiedAtConnection bool
	// Denied lists every reason the destination must be refused. Empty together
	// with Allowed means "refused, no matching destination".
	Denied []PersesDeniedDestination
}

// PersesProxyDestinationAllowed resolves one datasource endpoint against the
// default deny set, the policy's own allow/deny lists and the private-network
// rule.
//
// It applies the precedence the pinned release documents: the most specific
// network wins, and on equal prefix length an allowed network wins over a
// denied one. A hostname is never cleared by name alone - the check reports
// VerifiedAtConnection so the realization knows it must resolve and re-verify
// every returned address, including the ones reached through redirects.
func PersesProxyDestinationAllowed(policy PersesSecurityPolicy, endpoint string) PersesProxyDecision {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return PersesProxyDecision{Denied: []PersesDeniedDestination{{Host: endpoint, Reason: "empty datasource endpoint"}}}
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return PersesProxyDecision{Denied: []PersesDeniedDestination{{Host: endpoint, Reason: "unparseable datasource endpoint"}}}
	}
	host := strings.TrimSuffix(strings.TrimPrefix(parsed.Hostname(), "["), "]")
	var denied []PersesDeniedDestination
	if parsed.User != nil {
		denied = append(denied, PersesDeniedDestination{Host: host, Reason: "datasource URLs must not carry credentials; reference a BaseHarbor secret instead"})
	}
	scheme := strings.ToLower(parsed.Scheme)
	allowedSchemes := policy.AllowedSchemes
	if len(allowedSchemes) == 0 {
		allowedSchemes = []string{"http", "https"}
	}
	if !containsFolded(allowedSchemes, scheme) {
		denied = append(denied, PersesDeniedDestination{Host: host, Reason: "scheme " + scheme + " is not in the allowed datasource proxy schemes"})
	}
	if len(policy.AllowedHosts) > 0 && !persesHostMatchesAllowed(policy.AllowedHosts, host) {
		denied = append(denied, PersesDeniedDestination{Host: host, Reason: "host is not in the datasource proxy allowed_hosts list"})
	}
	addr, addrErr := netip.ParseAddr(host)
	if addrErr != nil {
		// A name: only the scheme and allowlist questions are decidable here.
		if isLoopbackName(host) {
			denied = append(denied, PersesDeniedDestination{Host: host, Reason: "loopback hostname; the upstream default deny set refuses loopback unless it is allowed explicitly"})
		}
		if len(denied) > 0 {
			return PersesProxyDecision{Denied: denied}
		}
		return PersesProxyDecision{VerifiedAtConnection: true, Denied: nil}
	}
	addr = addr.Unmap()

	if match := mostSpecific(policy.DeniedNetworks, addr); match.found {
		denied = append(denied, PersesDeniedDestination{Host: addr.String(), Reason: "destination is in the policy denied_networks list (" + match.entry + ")"})
		return PersesProxyDecision{Denied: denied}
	}
	allow := mostSpecific(policy.AllowedNetworks, addr)
	private := mostSpecific(persesPrivateNetworks, addr)
	defaultDeny := mostSpecificDenyByDefault(addr)

	switch {
	case defaultDeny.found:
		if allow.found && allow.bits >= defaultDeny.bits {
			return PersesProxyDecision{Allowed: true}
		}
		denied = append(denied, PersesDeniedDestination{Host: addr.String(), Reason: "destination is in the upstream default deny set (" + defaultDeny.reason + ")"})
	case policy.DenyPrivateNetworks && private.found:
		if allow.found && allow.bits >= private.bits {
			return PersesProxyDecision{Allowed: true}
		}
		denied = append(denied, PersesDeniedDestination{Host: addr.String(), Reason: "destination is in the private network " + private.entry + " and deny_private_networks is enabled"})
	case allow.found:
		return PersesProxyDecision{Allowed: true}
	}
	return PersesProxyDecision{Denied: denied}
}

// PersesProxySafeDefaults lists, in machine-readable form, the proxy
// restrictions the pinned release applies without configuration, so status
// output can state what is enforced rather than only what is hoped for.
func PersesProxySafeDefaults() []string {
	out := make([]string, 0, len(persesDenyByDefault))
	for _, entry := range persesDenyByDefault {
		out = append(out, entry.Prefix+" ("+entry.Reason+")")
	}
	sort.Strings(out)
	return out
}

type persesNetworkMatch struct {
	found  bool
	entry  string
	bits   int
	reason string
}

func mostSpecific(list []string, addr netip.Addr) persesNetworkMatch {
	best := persesNetworkMatch{bits: -1}
	for _, entry := range list {
		prefix, err := parsePersesNetwork("match", entry)
		if err != nil || !prefix.Contains(addr) {
			continue
		}
		if prefix.Bits() > best.bits {
			best = persesNetworkMatch{found: true, entry: entry, bits: prefix.Bits()}
		}
	}
	return best
}

func mostSpecificDenyByDefault(addr netip.Addr) persesNetworkMatch {
	best := persesNetworkMatch{bits: -1}
	for _, entry := range persesDenyByDefault {
		prefix, err := parsePersesNetwork("default", entry.Prefix)
		if err != nil || !prefix.Contains(addr) {
			continue
		}
		if prefix.Bits() > best.bits {
			best = persesNetworkMatch{found: true, entry: entry.Prefix, bits: prefix.Bits(), reason: entry.Reason}
		}
	}
	return best
}

func persesHostMatchesAllowed(hosts []string, host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, candidate := range hosts {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		if strings.HasPrefix(candidate, "*.") {
			if strings.HasSuffix(host, candidate[1:]) {
				return true
			}
			continue
		}
		if candidate == host {
			return true
		}
	}
	return false
}

func isLoopbackName(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "ip6-localhost"
}

func containsFolded(list []string, candidate string) bool {
	for _, entry := range list {
		if strings.EqualFold(strings.TrimSpace(entry), candidate) {
			return true
		}
	}
	return false
}

func parsePersesSemVer(version string) (persesSemVer, error) {
	raw := strings.TrimSpace(version)
	raw = strings.TrimPrefix(raw, "v")
	// A pre-release or build suffix is dropped: the floor applies to the release
	// line, and a release candidate of a newer line is still newer.
	if idx := strings.IndexAny(raw, "-+"); idx >= 0 {
		raw = raw[:idx]
	}
	parts := strings.Split(raw, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return persesSemVer{}, fmt.Errorf("Perses release version %q is not a valid version", version)
	}
	var parsed persesSemVer
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return persesSemVer{}, fmt.Errorf("Perses release version %q is not a valid version", version)
		}
		parsed[i] = n
	}
	return parsed, nil
}

type persesSemVer [3]int

func comparePersesSemVer(a, b persesSemVer) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
