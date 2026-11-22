package metrics

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

// Round-2 CODEX-REVIEW cells (akte 1645-0428-091.md): the P1 endpoint
// routability, the P2 policy posture, and the P4 gate-order contract.

// ---- 0428-091 round-2 cells: the CODEX-REVIEW findings P1–P4 ----

// TestPersesTempoDatasourceEndpointIsGatewayAliasRoutable (P1) proves the
// datasource endpoint is routable from a consumer that runs OUTSIDE the host's
// network namespace — exactly where the pinned Perses instance sits. The
// endpoint derived from the managed materialization is the gateway's
// backend-network alias, never a loopback address and never the bare
// "localhost" name the upstream default deny set refuses; and the rendered URL
// is policy-accepted by the pinned Perses proxy destination contract instead
// of being denied. Reverting the alias rendering (the loopback or
// localhost-host form) turns this cell red: the deny-set decision fails.
func TestPersesTempoDatasourceEndpointIsGatewayAliasRoutable(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	files := materializeTempoProviderIn(t, state, m)
	gateway, _ := startTempoMaterialGatewayWithUpstream(t, state, m, tempoSearchAnswer)
	writeTempoEndpoint(t, files, tempoTestServerPort(t, gateway.URL))

	proxyURL, err := tempoProviderAccessURL(mustProviderEndpoint(t, files), m)
	if err != nil {
		t.Fatalf("the managed endpoint must render the gateway-alias URL: %v", err)
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		t.Fatal(err)
	}
	host := parsed.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		t.Fatalf("the datasource endpoint host %q is a literal address: a consumer outside the host namespace can never route it (P1)", host)
	}
	if _, ipErr := net.LookupIP("127.0.0.1"); ipErr != nil {
		t.Fatal(ipErr) // guard: the loopback probe below must be meaningful
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		t.Fatalf("the datasource endpoint host %q is the loopback NAME the upstream default deny set refuses (P1)", host)
	}
	if want := traces.ManagedAccessAliases(tempoAccessGatewayServiceName)[1]; host != want {
		t.Fatalf("datasource host = %q, want the managed gateway alias %q", host, want)
	}
	// The pinned Perses proxy policy must ACCEPT the endpoint it will be
	// configured with: a denied destination is a datasource that can never
	// query, no matter what the resource shape says.
	decision := capability.PersesProxyDestinationAllowed(capability.DefaultPersesSecurityPolicy(), proxyURL)
	if len(decision.Denied) > 0 {
		t.Fatalf("the default Perses proxy policy must accept the datasource endpoint %q, denied: %+v", proxyURL, decision.Denied)
	}
	// And the loopback form it replaced must STILL be denied by that policy:
	// this is what the rewrite must never regress into.
	if allowed := capability.PersesProxyDestinationAllowed(capability.DefaultPersesSecurityPolicy(), "https://localhost:8443"); allowed.Allowed || !allowed.VerifiedAtConnection {
		if len(allowed.Denied) == 0 {
			t.Fatal("the loopback datasource endpoint must stay refused by the upstream default deny set")
		}
	}
	res, err := BuildPersesTempoDatasourceWithProbe(m, httpClientTempoProbe{client: mustTempoProviderClient(t, m)})
	if err != nil {
		t.Fatalf("the alias datasource must build: %v", err)
	}
	if res.Resource.Spec.Plugin.Spec.Proxy == nil || res.Resource.Spec.Plugin.Spec.Proxy.Spec.URL != proxyURL {
		t.Fatalf("emitted datasource URL = %+v, want the gateway-alias URL %q", res.Resource.Spec.Plugin.Spec.Proxy, proxyURL)
	}
}

// TestPersesTempoDatasourceStrictPolicyFailClosedWithoutIdentity (P2) is the
// fail-closed cell the review demands: with a policy that REQUIRES
// authentication (a stage/production environment) and a provider whose PKI
// carries no verifiable client identity, the datasource build and its health
// probe must both fail — never fall back to a permissive client.
func TestPersesTempoDatasourceStrictPolicyFailClosedWithoutIdentity(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	m.Environment = "stage"
	// The provider materializes with an enrolled client pair (the managed
	// gateway reconciliation mints it under the raised posture); the pair is
	// then RETIRED from the provider state — the boundary's own projection
	// and source tree carry no client identity any more. The strict stage
	// policy this datasource resolves REQUIRES mTLS, so the datasource must
	// fail closed on the missing enrolled identity instead of presenting an
	// anonymous session, in the build path and the health path alike.
	files := materializeTempoProviderIn(t, state, m)
	gateway, _ := startTempoMaterialGatewayWithUpstream(t, state, m, tempoSearchAnswer)
	for _, path := range []string{
		"runtime/client-cert.pem", "runtime/client-key.pem",
		"pki/client-cert.pem", "pki/client-key.pem",
	} {
		if err := os.Remove(filepath.Join(files.Dir, "service-access", path)); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	writeTempoEndpoint(t, files, tempoTestServerPort(t, gateway.URL))
	var authz *TempoAuthorizationError
	if _, err := BuildPersesTempoDatasource(m); !errors.As(err, &authz) {
		t.Fatalf("a required-policy provider without an enrolled client identity must fail closed, got %v", err)
	}
	if err := VerifyPersesTempoDatasourceHealth(t.Context(), m); err == nil {
		t.Fatal("the health path must never verify without the required identity")
	}
}

// TestPersesTempoDatasourcePresentsClientIdentityUnderStrictPolicy (P2, green
// half): with the strict policy in force AND the enrolled managed identity the
// gateway reconciliation mints, the build succeeds and the real query presents
// the client certificate — proven against a listener that refuses any session
// without one (RequireAndVerifyClientCert, the tempo-access posture).
func TestPersesTempoDatasourcePresentsClientIdentityUnderStrictPolicy(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	m.Environment = "stage"
	files := materializeTempoProviderIn(t, state, m)
	gateway, seen := startTempoMaterialGatewayWithUpstream(t, state, m, tempoSearchAnswer)
	writeTempoEndpoint(t, files, tempoTestServerPort(t, gateway.URL))
	res, err := BuildPersesTempoDatasourceWithProbe(m, httpClientTempoProbe{client: mustTempoProviderClient(t, m)})
	if err != nil {
		t.Fatalf("the enrolled strict-policy identity must authorize the datasource: %v", err)
	}
	if res.Reachability != PersesTempoReachabilityVerified {
		t.Fatalf("reachability = %q, want verified through the mTLS query", res.Reachability)
	}
	if seen == nil || *seen == "" {
		t.Fatal("the strict gateway must have observed the real TraceQL query")
	}
	if err := VerifyPersesTempoDatasourceHealth(t.Context(), m); err != nil {
		t.Fatalf("health over the authorized mTLS path: %v", err)
	}
}

// TestPersesTempoDatasourceDevKeepsPermittedPlaintextDefault (P2, permitted
// case): the dev environment default marks authentication optional at the
// POLICY layer, and the service-access surface (a server-auth-only listener,
// not the mTLS tempo-access gateway) is allowed to build its client without
// downgrading any required posture. This is the sibling provider surface
// (management UI) where policy.AuthenticationRequired=false legitimately
// yields a non-client-certified client: the cell pins that the policy flag,
// not a forced rewrite, is what decides it.
func TestPersesTempoDatasourcePolicyFlagDecidesClientRequirement(t *testing.T) {
	materialDir := t.TempDir()
	issuer := serviceissuer.New(t)
	// Reconcile a gateway WITHOUT client requirement (policy flag off) and
	// one WITH it, through the same managed-local machinery.
	for _, requireClient := range []bool{false, true} {
		dir := filepath.Join(materialDir, strconv.FormatBool(requireClient))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		policy := serviceaccess.Policy{
			Environment:            "dev",
			Provider:               string(capability.ProviderTempo),
			TLSRequired:            true,
			Authentication:         serviceaccess.AuthenticationMTLS,
			AuthenticationRequired: requireClient,
			PKISource:              serviceaccess.PKIManagedLocal,
			ServerName:             "localhost",
		}
		if _, err := serviceaccess.EnsureTLSMaterial(t.Context(), issuer, policy, filepath.Join(dir, "pki"), "localhost"); err != nil {
			t.Fatalf("reconcile managed material (requireClient=%v): %v", requireClient, err)
		}
		material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(dir, "pki"))
		if err != nil {
			t.Fatalf("read managed material (requireClient=%v): %v", requireClient, err)
		}
		client, err := serviceaccess.NewHTTPClientForPolicy(material, policy)
		if err != nil {
			t.Fatalf("build client (requireClient=%v): %v", requireClient, err)
		}
		transport, ok := client.Transport.(*http.Transport)
		if !ok || transport.TLSClientConfig == nil {
			t.Fatalf("client transport = %#v, want a TLS transport", client.Transport)
		}
		if got := len(transport.TLSClientConfig.Certificates); requireClient && got != 1 {
			t.Fatalf("required policy must present the enrolled client identity, got %d certificates", got)
		}
		if !requireClient {
			if got := len(transport.TLSClientConfig.Certificates); got != 0 {
				t.Fatalf("a permitted plaintext default must not force client material, got %d certificates", got)
			}
			var absent *TempoAuthorizationError
			// The tempo-access datasource path raises the requirement to the
			// boundary's posture regardless (P2): a provider whose gateway
			// verifies clients must present the identity even under the dev
			// policy default. Prove that raising happens HERE, in the
			// datasource reader, by pointing it at this client-less PKI.
			datasourceFiles := traces.ProviderFiles{Dir: filepath.Join(dir)}
			if _, err := tempoProviderHTTPClient(persesTempoTraceManifest(), datasourceFiles); err == nil {
				t.Fatal("the datasource client must fail closed when the gateway-verified identity is absent from the provider material")
			} else if !errors.As(err, &absent) {
				t.Fatalf("expected TempoAuthorizationError, got %v", err)
			}
		}
	}
}

// TestPersesTempoDatasourceBuilderGatesPinAndSelection (P4.2) pins the builder
// selection gate order: an unpinned release, an unselected surface or signal,
// and a denied collection policy each fail closed BEFORE any endpoint or
// client resolution — a build that reached the source would prove the pin or
// the selection could be bypassed.
func TestPersesTempoDatasourceBuilderGatesPinAndSelection(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	// Unpinned release: rejected before the surface even matters.
	badRelease := capability.PersesRelease{Version: "", Image: "", AuthenticationClass: "unsupported"}
	var unpinned *capability.PersesNotConfiguredError
	if _, err := buildPersesTempoDatasourceWithProbe(persesTempoTraceManifest(), badRelease, unavailableTempoProbe{}); !errors.As(err, &unpinned) {
		t.Fatalf("the builder must fail closed on an unpinned release before anything else, got %v", err)
	}
	// A provider state that would SUCCEED if consulted: point the resolution
	// at materialized, authorized Tempo, then remove ONLY the selection and
	// prove the gate still refuses before the source path runs.
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	materializeTempoProviderIn(t, state, m)
	unselected := m
	unselected.Services.Perses = false
	var surface *TempoSurfaceNotSelectedError
	if _, err := BuildPersesTempoDatasource(unselected); !errors.As(err, &surface) {
		t.Fatalf("a deselected Perses surface must fail closed even over materialized Tempo, got %v", err)
	}
	notrace := persesTempoManifest()
	notrace.Services.ObservabilityManagementUI = true
	notrace.Services.Perses = true
	var notSelected *TempoNotSelectedError
	if _, err := BuildPersesTempoDatasource(notrace); !errors.As(err, &notSelected) {
		t.Fatalf("an unselected Traces signal must fail closed over materialized Tempo, got %v", err)
	}
	t.Setenv(application.TracesEnabledEnv, "false")
	var denied *TempoAuthorizationError
	if _, err := BuildPersesTempoDatasource(m); !errors.As(err, &denied) {
		t.Fatalf("a disabled traces collection policy must fail closed over materialized Tempo, got %v", err)
	}
}

// TestPersesTempoDatasourceMatchesMaterializedGatewayCompose is the compose
// witness of the datasource contract (P4.3): the URL the datasource emits for
// a materialized provider must address the listener that provider's compose
// file actually publishes — the managed tempo-access gateway mapping
// 127.0.0.1:${BASEHARBOR_TEMPO_PORT} onto container port 8443, the only
// published address of the access surface (the Tempo container's own HTTP
// port is never published). A datasource URL whose port moved off that
// mapping would name a listener the gateway does not own.
func TestPersesTempoDatasourceMatchesMaterializedGatewayCompose(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	files := materializeTempoProviderIn(t, state, m)
	composeData, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(composeData), "127.0.0.1:${BASEHARBOR_TEMPO_PORT}:8443") {
		t.Fatalf("the materialized provider must publish the gateway access port 8443:\n%s", composeData)
	}
	gateway, _ := liveTempo(t, m, tempoSearchAnswer)
	endpoint := mustProviderEndpoint(t, files)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Port() != strconv.Itoa(tempoTestServerPort(t, gateway.URL)) {
		t.Fatalf("the materialized endpoint must carry the published gateway port: %s", endpoint)
	}
	res, err := BuildPersesTempoDatasourceWithProbe(m, httpClientTempoProbe{client: mustTempoProviderClient(t, m)})
	if err != nil {
		t.Fatalf("the datasource over the materialized gateway must build: %v", err)
	}
	datasourceURL := res.Resource.Spec.Plugin.Spec.Proxy.Spec.URL
	parsedDatasource, err := url.Parse(datasourceURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsedDatasource.Port() != parsed.Port() {
		t.Fatalf("the datasource port %q must equal the published gateway port %q", parsedDatasource.Port(), parsed.Port())
	}
}

// TestPersesTempoDatasourceEndpointAgreesWithMaterializedGatewayMapping (P1,
// mutation probe): the datasource URL's port must be the port the materialized
// gateway actually serves. Where a deployment's compose mapping drifted — the
// env publishes a port onto some listener other than the gateway's access port
// — the alias URL would point Perses at a port nothing answers, so the build
// fails closed instead of emitting a datasource that can never query.
func TestPersesTempoDatasourceEndpointAgreesWithMaterializedGatewayMapping(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	files := materializeTempoProviderIn(t, state, m)
	composeData, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(composeData), "${BASEHARBOR_TEMPO_PORT}:8443") {
		t.Fatalf("the managed gateway compose must publish BASEHARBOR_TEMPO_PORT onto its access port 8443:\n%s", composeData)
	}
	// Stand the gateway in and pin the endpoint to its published port, then
	// drift the compose mapping to a port the gateway does not serve.
	gateway, _ := liveTempo(t, m, tempoSearchAnswer)
	published := tempoTestServerPort(t, gateway.URL)
	writeTempoEndpoint(t, files, published)
	writeTempoCompose(t, files, strings.Replace(string(composeData), "${BASEHARBOR_TEMPO_PORT}:8443", "${BASEHARBOR_TEMPO_PORT}:3200", 1))
	var absent *TempoEndpointAbsentError
	if _, err := BuildPersesTempoDatasource(m); !errors.As(err, &absent) {
		t.Fatalf("a gateway whose compose mapping drifted off its access port must fail the datasource build closed, got %v", err)
	}
	// Restoring the managed mapping must build again (same provider, no other
	// change): the check compares the endpoint against the gateway mapping,
	// not against a hard-coded assumption about this test's port.
	writeTempoCompose(t, files, string(composeData))
	res, err := BuildPersesTempoDatasourceWithProbe(m, httpClientTempoProbe{client: mustTempoProviderClient(t, m)})
	if err != nil {
		t.Fatalf("the managed gateway mapping must build the datasource: %v", err)
	}
	datasourceURL := res.Resource.Spec.Plugin.Spec.Proxy.Spec.URL
	if want := "https://" + tempoAccessAlias + ":" + strconv.Itoa(published); datasourceURL != want {
		t.Fatalf("datasource URL = %q, want the managed gateway alias with its published port %q", datasourceURL, want)
	}
}

// writeTempoCompose replaces the materialized provider's compose file (the
// drift cells mutate the gateway's published-port mapping and restore it).
func writeTempoCompose(t *testing.T, files traces.ProviderFiles, contents string) {
	t.Helper()
	if err := os.WriteFile(files.Compose, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestPersesTempoDatasourceAliasIsMaterializedGatewayIdentity (P1) pins the
// compose- and certificate-side truth the datasource URL leans on: the managed
// tempo-access rendering registers the access alias as a network alias of the
// gateway, the gateway spec mints the alias into its server certificate, and
// the gateway container-side listener is the access port the datasource
// names. Reverting any of the three (rendering the gateway without aliases,
// or dropping the alias from the certificate names) turns this cell red.
func TestPersesTempoDatasourceAliasIsMaterializedGatewayIdentity(t *testing.T) {
	spec := traces.TempoAccessSpecForTests()
	aliases := spec.NetworkAliases
	found := false
	for _, alias := range aliases {
		if alias == tempoAccessAlias {
			found = true
		}
	}
	if !found {
		t.Fatalf("the gateway spec registers %v, want the datasource alias %q among them", aliases, tempoAccessAlias)
	}
	certCovered := false
	for _, name := range spec.CertificateNames {
		if name == tempoAccessAlias {
			certCovered = true
		}
	}
	if !certCovered {
		t.Fatalf("the gateway certificate names %v, want the datasource alias %q among them", spec.CertificateNames, tempoAccessAlias)
	}
	rendered := serviceaccess.HTTPGatewayComposeService(
		serviceaccess.HTTPGatewayFiles{
			Caddyfile: "./service-access/Caddyfile",
			Material: serviceaccess.TLSMaterial{
				CA:                "./service-access/runtime/ca.pem",
				ServerCertificate: "./service-access/runtime/server.pem",
				ServerKey:         "./service-access/runtime/server-key.pem",
			},
		},
		spec,
	)
	if !strings.Contains(rendered, strconv.Quote(tempoAccessAlias)) {
		t.Fatalf("the gateway compose service must register the alias the datasource addresses:\n%s", rendered)
	}
	if !strings.Contains(rendered, "127.0.0.1:${"+spec.PublishedPortEnv+"}:8443") {
		t.Fatalf("the gateway compose service must publish the host port onto the access port 8443:\n%s", rendered)
	}
}

// TestPersesTempoDatasourceSharedBackendGatewayStaysPolicyNeutral pins that the
// P2 posture raise is scoped to the tempo-access gateway spec: a dev
// environment's valkey shared-backend gateway (TCPGateway, no RequireClient)
// reconciles server-side material only — no client pair is forced into it by
// anything this card changed. If this cell turns red, the raise leaked out of
// the tempo spec.
func TestPersesTempoDatasourceSharedBackendGatewayStaysPolicyNeutral(t *testing.T) {
	dir := t.TempDir()
	issuer := serviceissuer.New(t)
	policy, err := serviceaccess.ResolveWithPolicy("dev", "cache", serviceaccess.AuthenticationMTLS, serviceaccess.EnvironmentPolicy{TrustedLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := serviceaccess.EnsureTLSMaterial(t.Context(), issuer, policy, filepath.Join(dir, "pki"), "valkey-access"); err != nil {
		t.Fatal(err)
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(dir, "pki"))
	if err != nil {
		t.Fatal(err)
	}
	if material.ClientCertificate != "" || material.ClientKey != "" {
		t.Fatalf("a dev non-RequireClient gateway must not enroll a client pair: %+v", material)
	}
}

// TestPersesTempoDatasourceBuildReadsTheMaterializedGatewayCompose (P1,
// mutation probe): the datasource build itself must refuse a deployment whose
// compose mapping drifted — no gateway stand-in, no probe: BuildPersesTempo-
// Datasource reads the materialized provider compose and fails closed when the
// port it publishes is not the gateway's access port 8443. Reverting the
// compose-mapping check in the build path (tempoProviderPublishedPort) turns
// this cell red even though every runtime-observation cell stays green: the
// static proof stands on its own.
func TestPersesTempoDatasourceBuildReadsTheMaterializedGatewayCompose(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	files := materializeTempoProviderIn(t, state, m)
	composeData, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(composeData), "127.0.0.1:${BASEHARBOR_TEMPO_PORT}:8443") {
		t.Fatalf("the managed gateway compose must publish onto its access port 8443:\n%s", composeData)
	}
	// Drift the compose mapping OFF the gateway access port: the datasource
	// the build emits would address a listener the managed gateway does not
	// own, so the build must fail closed, without any probe running.
	writeTempoCompose(t, files, strings.Replace(string(composeData), "${BASEHARBOR_TEMPO_PORT}:8443", "${BASEHARBOR_TEMPO_PORT}:3200", 1))
	var absent *TempoEndpointAbsentError
	if _, err := BuildPersesTempoDatasource(m); !errors.As(err, &absent) {
		t.Fatalf("a drifted compose mapping must fail the datasource build closed, got %v", err)
	}
	// Restoring the managed mapping builds again — the check compares the
	// endpoint against the gateway mapping, not against this test's state.
	writeTempoCompose(t, files, string(composeData))
	if _, err := BuildPersesTempoDatasource(m); err != nil {
		t.Fatalf("the managed gateway mapping must build the datasource: %v", err)
	}
}
