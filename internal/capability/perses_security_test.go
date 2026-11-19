package capability

import (
	"errors"
	"strings"
	"testing"
)

// authenticatedRelease is the release shape a provisioned BaseHarbor surface
// carries: a pinned release with an authentication class actually configured.
func authenticatedRelease(class string) PersesRelease {
	return PersesRelease{
		Version:                 "v0.55.1",
		Image:                   "persesdev/perses:v0.55.1",
		PrometheusPluginVersion: "v0.6.0",
		AuthenticationClass:     class,
	}
}

func protectedPolicy() PersesSecurityPolicy {
	return DefaultPersesSecurityPolicy()
}

func TestPersesSecurityPolicyRejectsAnonymousExposure(t *testing.T) {
	err := ValidatePersesSecurityPolicy(AnonymousPersesSecurityPolicy())
	if err == nil {
		t.Fatal("an anonymous Perses surface must fail closed: the upstream default exposes every API endpoint and the datasource proxy without credentials")
	}
	var anonymous *PersesAnonymousExposureError
	if !errors.As(err, &anonymous) {
		t.Fatalf("anonymous Perses error must be the typed exposure verdict, got %v", err)
	}
	policy := protectedPolicy()
	policy.AuthenticationEnabled = false
	if err := ValidatePersesSecurityPolicy(policy); err == nil {
		t.Fatal("disabling authentication on an otherwise protected surface must fail closed")
	}
}

func TestPersesSecurityPolicyRequiresEncryptionKeyWhenAuthEnabled(t *testing.T) {
	policy := protectedPolicy()
	policy.EncryptionKeyDeclared = false
	err := ValidatePersesSecurityPolicy(policy)
	if err == nil {
		t.Fatal("authentication without an encryption key must fail closed: the pinned release refuses to start and datasource secrets would use the upstream default key")
	}
	if !strings.Contains(err.Error(), "encryption key") {
		t.Fatalf("error must name the encryption key, got %v", err)
	}
	if !strings.Contains(err.Error(), "persesdev") && !strings.Contains(err.Error(), "v0.55.1") {
		t.Fatalf("error must name the pinned release the requirement comes from, got %v", err)
	}
}

func TestPersesSecurityPolicyRejectsVulnerablePins(t *testing.T) {
	cases := []struct {
		name    string
		version string
		denied  bool
	}{
		{name: "pre-fix release is unsupported", version: "v0.54.0", denied: true},
		{name: "older patch of the fix line is unsupported", version: "v0.53.1", denied: true},
		{name: "the fix release itself is allowed", version: "v0.55.0", denied: false},
		{name: "the pinned release is allowed", version: "v0.55.1", denied: false},
		{name: "a newer line is allowed", version: "v0.56.0", denied: false},
		{name: "pre-release of a newer line is allowed", version: "v0.56.0-rc.1", denied: false},
		{name: "a pre-release of the floor line is denied", version: "v0.54.0-rc.1", denied: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := protectedPolicy()
			policy.Release = authenticatedRelease("native-oidc")
			policy.Release.Version = tc.version
			err := ValidatePersesSecurityPolicy(policy)
			if tc.denied && err == nil {
				t.Fatalf("Perses %s must be rejected: it predates the datasource-proxy SSRF and credential-forwarding fixes", tc.version)
			}
			if !tc.denied && err != nil {
				t.Fatalf("Perses %s must validate: %v", tc.version, err)
			}
			if tc.denied && !strings.Contains(err.Error(), "SSRF") {
				t.Fatalf("rejection must name the missing fixes, got %v", err)
			}
		})
	}
}

func TestPersesSecurityPolicyRequiresProxyRestriction(t *testing.T) {
	policy := protectedPolicy()
	policy.DenyPrivateNetworks = false
	policy.ProvisioningEnforced = false
	err := ValidatePersesSecurityPolicy(policy)
	if err == nil {
		t.Fatal("an unrestricted datasource proxy must fail closed: without an allowlist, a private-network deny or provisioning control, anyone who can create a datasource can read any service the Target reaches")
	}
	if !strings.Contains(err.Error(), "allowed_hosts") || !strings.Contains(err.Error(), "provisioning") {
		t.Fatalf("error must name the three remedies, got %v", err)
	}
	// Each single remedy is sufficient.
	policy.ProvisioningEnforced = true
	if err := ValidatePersesSecurityPolicy(policy); err != nil {
		t.Fatalf("provisioning ownership alone must satisfy the proxy restriction: %v", err)
	}
	policy.ProvisioningEnforced = false
	policy.DenyPrivateNetworks = true
	if err := ValidatePersesSecurityPolicy(policy); err != nil {
		t.Fatalf("denying private networks alone must satisfy the proxy restriction: %v", err)
	}
	policy.DenyPrivateNetworks = false
	policy.AllowedHosts = []string{"prometheus.example.com", "*.monitoring.svc"}
	if err := ValidatePersesSecurityPolicy(policy); err != nil {
		t.Fatalf("an exhaustive host allowlist alone must satisfy the proxy restriction: %v", err)
	}
}

func TestPersesSecurityPolicyRejectsCredentialForwarding(t *testing.T) {
	policy := protectedPolicy()
	policy.ForwardUserCredentials = true
	err := ValidatePersesSecurityPolicy(policy)
	if err == nil {
		t.Fatal("forwarding caller credentials to datasources must fail closed: the pinned release denies it by default and doing so leaks operator credentials to every datasource")
	}
	if !strings.Contains(err.Error(), "forward user credentials") {
		t.Fatalf("error must name credential forwarding, got %v", err)
	}
}

func TestPersesSecurityPolicyValidatesNetworkAndSchemeSyntax(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*PersesSecurityPolicy)
		want   string
	}{
		{name: "routable literal host is invalid", mutate: func(p *PersesSecurityPolicy) { p.AllowedHosts = []string{"127.0.0.1"} }, want: "allowed host"},
		{name: "host with port is invalid", mutate: func(p *PersesSecurityPolicy) { p.AllowedHosts = []string{"prom.example.com:9090"} }, want: "allowed host"},
		{name: "wildcard with port is invalid", mutate: func(p *PersesSecurityPolicy) { p.AllowedHosts = []string{"*.svc:80"} }, want: "allowed host"},
		{name: "malformed allowed network", mutate: func(p *PersesSecurityPolicy) { p.AllowedNetworks = []string{"10.0.0.0/33"} }, want: "allowed network"},
		{name: "malformed denied network", mutate: func(p *PersesSecurityPolicy) { p.DeniedNetworks = []string{"10.1"} }, want: "denied network"},
		{name: "unsupported proxy scheme", mutate: func(p *PersesSecurityPolicy) { p.AllowedSchemes = []string{"gopher"} }, want: "scheme"},
		{name: "relative secret directory", mutate: func(p *PersesSecurityPolicy) { p.SecretFileAllowedDirectories = []string{"secrets"} }, want: "absolute path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := protectedPolicy()
			tc.mutate(&policy)
			err := ValidatePersesSecurityPolicy(policy)
			if err == nil {
				t.Fatalf("policy %v must fail validation", policy)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q must mention %q", err, tc.want)
			}
		})
	}
}

func TestPersesSecurityPolicyAcceptsAbsoluteSecretDirectories(t *testing.T) {
	policy := protectedPolicy()
	policy.SecretFileAllowedDirectories = []string{"/etc/baseharbor/secrets"}
	if err := ValidatePersesSecurityPolicy(policy); err != nil {
		t.Fatalf("an absolute secret directory must validate: %v", err)
	}
}

func TestPersesProxyDestinationDeniesUpstreamDefaultSet(t *testing.T) {
	policy := protectedPolicy()
	policy.DenyPrivateNetworks = false
	policy.AllowedSchemes = []string{"http", "https"}
	cases := []struct {
		endpoint string
		reason   string
	}{
		{"http://127.0.0.1:9090", "loopback"},
		{"http://127.5.5.5:9090", "loopback"},
		{"http://[::1]:9090", "loopback"},
		{"http://169.254.169.254/latest/meta-data/", "cloud metadata"},
		{"http://169.254.170.2/creds", "ECS"},
		{"http://100.100.100.200/latest/meta-data", "Alibaba"},
		{"http://168.63.129.16/", "WireServer"},
		{"http://169.254.1.1/", "link-local"},
		{"http://[fe80::1]/", "link-local"},
		{"http://[::ffff:127.0.0.1]:9090/", "loopback"},
		{"http://[fd00:ec2::254]/", "AWS IPv6 metadata"},
		{"http://0.0.0.0:1/", "this network"},
	}
	for _, tc := range cases {
		t.Run(tc.endpoint, func(t *testing.T) {
			verdict := PersesProxyDestinationAllowed(policy, tc.endpoint)
			if verdict.Allowed {
				t.Fatalf("%s must be denied by the upstream default set", tc.endpoint)
			}
			if len(verdict.Denied) == 0 {
				t.Fatalf("%s must report a denial reason", tc.endpoint)
			}
			if !strings.Contains(verdict.Denied[0].Reason, tc.reason) {
				t.Fatalf("%s denial reason %q must mention %q", tc.endpoint, verdict.Denied[0].Reason, tc.reason)
			}
		})
	}
}

func TestPersesProxyDestinationDeniesPrivateNetworksWhenConfigured(t *testing.T) {
	policy := protectedPolicy()
	for _, endpoint := range []string{"http://10.20.30.40:9090", "http://192.168.1.10:9090", "http://172.20.0.5:9090", "http://[fc00::5]:9090"} {
		if verdict := PersesProxyDestinationAllowed(policy, endpoint); verdict.Allowed {
			t.Fatalf("%s must be denied while deny_private_networks is enabled", endpoint)
		}
	}
	// A more specific allow wins, exactly as upstream documents.
	policy.AllowedNetworks = []string{"10.20.0.0/16"}
	if verdict := PersesProxyDestinationAllowed(policy, "http://10.20.30.40:9090"); !verdict.Allowed {
		t.Fatalf("the more specific allowed network must win over the private deny range: %+v", verdict)
	}
	if verdict := PersesProxyDestinationAllowed(policy, "http://10.30.0.1:9090"); verdict.Allowed {
		t.Fatal("an allowed /16 must not open the rest of 10.0.0.0/8")
	}
}

func TestPersesProxyDestinationMostSpecificWinsAgainstDefaultDeny(t *testing.T) {
	policy := protectedPolicy()
	policy.AllowedNetworks = []string{"127.0.0.0/8"}
	if verdict := PersesProxyDestinationAllowed(policy, "http://127.0.0.1:9090"); !verdict.Allowed {
		t.Fatalf("allowing 127.0.0.0/8 must allow loopback (same prefix length, allow wins): %+v", verdict)
	}
	// A broad private allow must not re-open the metadata endpoints, because they
	// are denied as single hosts and therefore more specific.
	policy2 := protectedPolicy()
	policy2.DenyPrivateNetworks = false
	policy2.AllowedNetworks = []string{"169.254.0.0/16"}
	if verdict := PersesProxyDestinationAllowed(policy2, "http://169.254.169.254/"); verdict.Allowed {
		t.Fatalf("allowing 169.254.0.0/16 must not allow the metadata endpoint (single host is more specific): %+v", verdict)
	}
}

func TestPersesProxyDestinationRejectsCredentialsAndSchemes(t *testing.T) {
	policy := protectedPolicy()
	verdict := PersesProxyDestinationAllowed(policy, "https://user:password@prom.example.com")
	if verdict.Allowed || verdict.VerifiedAtConnection {
		t.Fatalf("a datasource URL with embedded credentials must be refused: %+v", verdict)
	}
	if len(verdict.Denied) == 0 || !strings.Contains(verdict.Denied[0].Reason, "credentials") {
		t.Fatalf("denial must name credentials, got %+v", verdict.Denied)
	}
	if verdict := PersesProxyDestinationAllowed(policy, "ftp://prom.example.com"); verdict.Allowed || verdict.VerifiedAtConnection {
		t.Fatalf("a scheme outside the allowed list must be refused: %+v", verdict)
	}
	// https is the default allow here; a plain http name is refused by policy.
	if verdict := PersesProxyDestinationAllowed(policy, "http://prom.example.com"); verdict.Allowed || verdict.VerifiedAtConnection {
		t.Fatalf("plain http must be refused when only https is allowed: %+v", verdict)
	}
	relaxed := protectedPolicy()
	relaxed.AllowedSchemes = []string{"http", "https"}
	if verdict := PersesProxyDestinationAllowed(relaxed, "http://prom.example.com"); !verdict.VerifiedAtConnection {
		t.Fatalf("a hostname must be reported as needing connection-time verification, not statically cleared: %+v", verdict)
	}
}

func TestPersesProxyDestinationAllowedHostsListIsAuthoritative(t *testing.T) {
	policy := protectedPolicy()
	policy.AllowedSchemes = []string{"https"}
	policy.AllowedHosts = []string{"prometheus.example.com", "*.thanos.example.com"}
	if verdict := PersesProxyDestinationAllowed(policy, "https://prometheus.example.com"); !verdict.VerifiedAtConnection {
		t.Fatalf("an exact allowlisted host must pass static checks: %+v", verdict)
	}
	if verdict := PersesProxyDestinationAllowed(policy, "https://a.thanos.example.com"); !verdict.VerifiedAtConnection {
		t.Fatalf("a wildcard-subdomain host must pass static checks: %+v", verdict)
	}
	verdict := PersesProxyDestinationAllowed(policy, "https://evil.example.org")
	if verdict.Allowed || verdict.VerifiedAtConnection {
		t.Fatalf("a host outside allowed_hosts must be refused: %+v", verdict)
	}
	if len(verdict.Denied) == 0 || !strings.Contains(verdict.Denied[0].Reason, "allowed_hosts") {
		t.Fatalf("denial must name allowed_hosts, got %+v", verdict.Denied)
	}
}

func TestPersesProxyDestinationFailsClosedOnUnparseableEndpoints(t *testing.T) {
	policy := protectedPolicy()
	for _, endpoint := range []string{"", "   ", "://nope", "http://"} {
		verdict := PersesProxyDestinationAllowed(policy, endpoint)
		if verdict.Allowed || verdict.VerifiedAtConnection {
			t.Fatalf("endpoint %q must not be cleared: %+v", endpoint, verdict)
		}
		if len(verdict.Denied) == 0 {
			t.Fatalf("endpoint %q must report why it is refused", endpoint)
		}
	}
}

func TestPersesStatusCannotReportConfiguredForAnonymousSurface(t *testing.T) {
	anonymous := AnonymousPersesSecurityPolicy()
	if status := PersesConfiguredStatusNow(true, anonymous); status == nil {
		t.Fatal("the fail-closed status must still be a concrete state")
	} else if *status != PersesStatusNotConfigured {
		t.Fatalf("an anonymous Perses surface must report %q, got %q", PersesStatusNotConfigured, *status)
	}
	if status := PersesConfiguredStatusNow(true, protectedPolicy()); status == nil || *status != PersesStatusConfiguredUnverified {
		t.Fatalf("a protected selected surface must report %q, got %v", PersesStatusConfiguredUnverified, status)
	}
	// Neither variant can ever be "verified": no health probe exists in this Core.
	for _, policy := range []PersesSecurityPolicy{protectedPolicy(), AnonymousPersesSecurityPolicy()} {
		if status := PersesConfiguredStatusNow(true, policy); status != nil && *status == PersesStatusVerified {
			t.Fatalf("no policy may produce %q without a real health probe: %+v", PersesStatusVerified, policy)
		}
	}
	if status := PersesConfiguredStatusNow(false, protectedPolicy()); status == nil || *status != PersesStatusNotConfigured {
		t.Fatalf("an unselected surface must report %q regardless of policy", PersesStatusNotConfigured)
	}
}

func TestPersesSecurityPolicyDefaultValidates(t *testing.T) {
	if err := ValidatePersesSecurityPolicy(DefaultPersesSecurityPolicy()); err != nil {
		t.Fatalf("the documented BaseHarbor default posture must validate: %v", err)
	}
	if PersesDefaultRelease.Version != "v0.55.1" {
		t.Fatalf("the pinned default must be the current security release, got %q", PersesDefaultRelease.Version)
	}
	if err := ValidatePersesVersionFloor(PersesDefaultRelease.Version); err != nil {
		t.Fatalf("the pinned default must clear the version floor: %v", err)
	}
}

func TestPersesProxySafeDefaultsAreStatedAndSorted(t *testing.T) {
	defaults := PersesProxySafeDefaults()
	if len(defaults) < 10 {
		t.Fatalf("the default deny set must be stated explicitly, got %d entries", len(defaults))
	}
	joined := strings.Join(defaults, "\n")
	for _, required := range []string{"127.0.0.0/8", "169.254.169.254/32", "168.63.129.16/32", "fe80::/10"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("the default deny set must state %s, got:\n%s", required, joined)
		}
	}
	for i := 1; i < len(defaults); i++ {
		if defaults[i-1] > defaults[i] {
			t.Fatalf("the default deny set must be deterministically ordered: %v", defaults)
		}
	}
}
