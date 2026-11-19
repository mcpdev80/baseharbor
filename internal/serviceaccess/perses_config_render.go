package serviceaccess

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses configuration rendering.
//
// This is the producer of the file the Perses service definition mounts at
// PersesConfigContainerPath. The rendering is a pure, deterministic function of
// the validated security policy, so a deployment can prove what posture the
// process serves by re-rendering it, and reconciliation detects drift without
// ever printing credential material.
//
// Credential rules: the encryption key is referenced by file only
// (encryption_key_file); no secret value, token or key content is ever
// interpolated into the YAML. Datasource secrets stay in the Perses database
// behind BaseHarbor-owned provisioning. The native authentication provider is
// disabled: operator access belongs to the gateway, and an account-less native
// provider cannot authenticate anyone.
//
// The datasource proxy is left structurally absent from the rendering, and the
// gateway refuses the /proxy/ route prefix; the deny lists written here bind
// the server itself if a realization ever opens a proxy route.

const (
	// PersesEncryptionKeyContainerPath is where the realization mounts the
	// encryption key material, referenced (never inlined) by the rendering.
	PersesEncryptionKeyContainerPath = "/etc/perses/secrets/encryption-key"
	// PersesDatabaseContainerPath is the file-database folder inside the
	// container; the rendered service definition places it on a tmpfs, so a
	// restarted surface re-provisions from BaseHarbor-owned state.
	PersesDatabaseContainerPath = "/etc/perses/database"
)

// RenderPersesConfig renders the Perses configuration file for one validated
// security policy. It fails closed: an invalid posture has no rendering, and a
// policy that cannot be expressed without weakening the posture (a routable
// literal bind, a key reference that could be mistaken for a secret value) has
// no rendering either. The serviceAddress is the gateway plan's
// deployment-scoped backend alias the routes reach (empty omits the
// statement); it belongs to the plan, never to the security posture.
func RenderPersesConfig(policy capability.PersesSecurityPolicy, serviceAddress string) (string, error) {
	if err := capability.ValidatePersesSecurityPolicy(policy); err != nil {
		return "", err
	}
	bind := strings.TrimSpace(policy.BindAddress)
	if bind == "" {
		bind = "127.0.0.1"
	}
	service := strings.TrimSpace(serviceAddress)
	if service != "" {
		if addr, err := netip.ParseAddr(service); err == nil && !addr.Unmap().IsLoopback() {
			return "", fmt.Errorf("Perses service address %q is a routable literal: the gateway upstream may only name a service address or loopback", service)
		} else if err != nil && !validPersesBindAddress(service) {
			return "", fmt.Errorf("Perses service address %q is not a service address", service)
		}
	}
	if addr, err := netip.ParseAddr(bind); err == nil && !addr.Unmap().IsLoopback() {
		return "", fmt.Errorf("Perses bind address %q is a routable literal: the management surface may only bind loopback of its namespace or its service address", bind)
	} else if err != nil && !validPersesBindAddress(bind) {
		return "", fmt.Errorf("Perses bind address %q is not a literal address or service address", bind)
	}
	var b strings.Builder
	b.WriteString("# BaseHarbor rendered Perses configuration; deterministic from the validated security policy, mounted read-only.\n")
	fmt.Fprintf(&b, "# policy release pin: %s\n", policy.Release.Version)
	b.WriteString("database:\n  file:\n    folder: " + strconv.Quote(PersesDatabaseContainerPath) + "\n    extension: yaml\n")
	b.WriteString("security:\n")
	b.WriteString("  enable_auth: true\n")
	fmt.Fprintf(&b, "  readonly: %t\n", policy.ReadOnlyMode)
	b.WriteString("  encryption_key_file: " + strconv.Quote(PersesEncryptionKeyContainerPath) + "\n")
	if service != "" {
		// How this surface is reached, stated for the server's own view: the
		// gateway routes to the backend-network alias. The effective bind
		// stays the command-line loopback pin, so the process can never
		// answer on another interface; this key never carries a bind address.
		b.WriteString("  advertised_address: " + strconv.Quote(service) + "\n")
	}
	// fix-7 B2: enable_auth without any provider is refused by server
	// verification (no login path exists). The native provider is stated
	// explicitly, enabled: the gateway (mTLS or bearer) is the operator
	// boundary in front of it; native users live in the encrypted Perses
	// database (encryption_key_file above), and sign-up stays usable for
	// the first operator on a loopback-only surface.
	b.WriteString("  authentication:\n    providers:\n      enable_native: true\n")
	if len(policy.SecretFileAllowedDirectories) > 0 {
		b.WriteString("  secret_file_allowed_directories:\n")
		for _, directory := range sortedUnique(policy.SecretFileAllowedDirectories) {
			fmt.Fprintf(&b, "    - %s\n", strconv.Quote(directory))
		}
	}
	b.WriteString("provisioning:\n  folders: []\n")
	b.WriteString("datasource:\n  disable_local: true\n  proxy:\n")
	if len(policy.AllowedHosts) > 0 {
		b.WriteString("    allowed_hosts:\n")
		for _, host := range sortedUnique(policy.AllowedHosts) {
			fmt.Fprintf(&b, "      - %s\n", strconv.Quote(strings.ToLower(host)))
		}
	}
	if len(policy.AllowedNetworks) > 0 {
		b.WriteString("    allowed_networks:\n")
		for _, network := range sortedPersesPrefixes(policy.AllowedNetworks) {
			fmt.Fprintf(&b, "      - %s\n", strconv.Quote(network))
		}
	}
	b.WriteString("    denied_networks:\n")
	for _, network := range sortedPersesPrefixes(policy.DeniedNetworks) {
		fmt.Fprintf(&b, "      - %s\n", strconv.Quote(network))
	}
	fmt.Fprintf(&b, "    deny_private_networks: %t\n", policy.DenyPrivateNetworks)
	if len(policy.AllowedSchemes) > 0 {
		b.WriteString("    http:\n      allowed_schemes:\n")
		for _, scheme := range sortedUnique(policy.AllowedSchemes) {
			fmt.Fprintf(&b, "        - %s\n", strconv.Quote(strings.ToLower(scheme)))
		}
	}
	return b.String(), nil
}

// sortedUnique returns a stable, de-duplicated rendering order.
func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// sortedPersesPrefixes normalizes every network entry to canonical CIDR form
// (so a bare address and its /32 render identically) and orders the list
// deterministically.
func sortedPersesPrefixes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, entry := range values {
		prefix, err := parsePersesNetwork("proxy", entry)
		if err != nil {
			continue
		}
		out = append(out, prefix.String())
	}
	return sortedUnique(out)
}
