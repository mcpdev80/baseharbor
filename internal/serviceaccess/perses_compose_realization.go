package serviceaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses compose realization.
//
// This file carries the second half of the Perses gateway contract: turning a
// validated PersesGatewayPlan plus the realization's materialized files into
// the service definitions the managed compose lifecycle runs. Every entry
// here fails closed: a rendering needs a proven configuration producer, a
// materialized encryption-key projection, and a persisted plan record that
// still validates.

// serviceAddressOf is the deployment-scoped backend address the gateway
// routes to. It lives on the plan (never on the security posture); the
// non-shared topology renders the command-line bind instead and states no
// address in the configuration.
func serviceAddressOf(plan PersesGatewayPlan) string {
	return plan.ServiceAddress
}

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
	// ComposeSourceDir, when set, is the host path the rendering binds at
	// PersesConfigMountDir instead of the declared HostPath's own directory.
	// The production projection uses it to keep the compose definition
	// relocation-safe (a relative path inside the runtime directory), while
	// HostPath remains where the proof reads the bytes from.
	ComposeSourceDir string
}

// PersesConfigRenderer materializes the Perses configuration file for a plan.
// It is the producer half of the --config=/etc/perses/config.yaml contract:
// without a renderer wired into the deployment there is no configuration to
// mount, and BuildPersesGateway refuses to render service definitions that
// would silently fall back to the upstream anonymous default posture.
type PersesConfigRenderer func(context.Context, PersesGatewayPlan) (PersesRenderedConfig, error)

// SetPersesConfigRenderer registers the process-wide fallback renderer. The
// production composition passes its renderer to PersesComposeServices
// directly; this registration exists so a realization without a per-call
// renderer still fails closed instead of silently serving the upstream
// anonymous default posture. A nil renderer is an explicit statement that no
// realization can materialize the file.
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

func registeredPersesConfigRenderer() PersesConfigRenderer {
	persesConfigRendererMu.RLock()
	defer persesConfigRendererMu.RUnlock()
	return persesConfigRenderer
}

var (
	persesConfigRendererMu sync.RWMutex
	persesConfigRenderer   PersesConfigRenderer
)

// ErrPersesConfigRendererMissing states the fail-closed reason rendering has no
// configuration producer: a deployment must never fall back to the upstream
// anonymous default posture by omission.
var ErrPersesConfigRendererMissing = errors.New("Perses service rendering requires a configuration renderer: the --config file must have a proven producer, otherwise the deployment silently runs the upstream anonymous default posture")

// PersesComposeInput is the explicit, per-call rendering contract of the
// production composition. Passing the renderer as input (instead of relying on
// process-global registration) is what makes the compose writer's provenance
// chain testable and deterministic: every rendering names its producer.
type PersesComposeInput struct {
	// Plan is the validated gateway plan Perses is deployed under.
	Plan PersesGatewayPlan
	// Gateway carries the materialized gateway access files.
	Gateway HTTPGatewayFiles
	// Release is the pinned Perses release the plan was validated against.
	Release capability.PersesRelease
	// RenderConfig produces the configuration file the rendered service
	// definition mounts read-only. A nil renderer fails closed.
	RenderConfig PersesConfigRenderer
	// EncryptionKeyProjection is the host file the realization materialized
	// the datasource encryption key in, container-readable. Its directory is
	// mounted read-only at PersesEncryptionKeyMountDir, the path the rendered
	// configuration references as security.encryption_key_file. The file must
	// exist; the rendering itself is relative to the compose file.
	EncryptionKeyProjection string
	// EncryptionKeyComposeBind is the compose-relative directory the key is
	// mounted from (the production layout's deterministic projection path).
	// When set it replaces the derived-from-projection bind source; the
	// projection file is still proven to exist.
	EncryptionKeyComposeBind string
	// Mounts, when set, replaces the default config/key mounts with the
	// shared projection contract (PersesServiceMounts). The production
	// composition uses it so the mount form has exactly one source of truth;
	// the existence proofs still run against the declared paths.
	Mounts []string
}

// PersesGatewayPlanRecord is the persisted, credential-free form of a gateway
// plan. The production projection writes it AFTER the gateway material and
// Caddyfile exist, and the pure compose rendering reads it back: the rendered
// service definition is then a function of materialized, provenance-checked
// state instead of hidden per-process environment. Anything else (a missing,
// stale or weakened record) fails the deployment closed. Secrets never appear
// in it — the bearer token reference is dropped at persistence because the
// gateway rendering resolves it from the projection directory again.
type PersesGatewayPlanRecord struct {
	Version               int                             `json:"version"`
	Policy                capability.PersesSecurityPolicy `json:"policy"`
	Spec                  HTTPGatewaySpec                 `json:"spec"`
	DeniedPaths           []string                        `json:"denied_paths"`
	RequireClientIdentity bool                            `json:"require_client_identity"`
	ProxyDenyReason       string                          `json:"proxy_deny_reason,omitempty"`
	UpstreamBindAddress   string                          `json:"upstream_bind_address"`
	SharedGatewayNetwork  bool                            `json:"shared_gateway_network"`
	ServiceAddress        string                          `json:"service_address,omitempty"`
	EncryptionKeyFile     string                          `json:"encryption_key_file,omitempty"`
}

// PersistPersesGatewayPlan writes the credential-free plan record into the
// gateway projection directory.
func PersistPersesGatewayPlan(stateDir string, plan PersesGatewayPlan) error {
	record := PersesGatewayPlanRecord{
		Version:               1,
		Policy:                plan.Policy,
		Spec:                  plan.Spec,
		DeniedPaths:           append([]string(nil), plan.DeniedPaths...),
		RequireClientIdentity: plan.RequireClientIdentity,
		ProxyDenyReason:       plan.ProxyDenyReason,
		UpstreamBindAddress:   plan.UpstreamBindAddress,
		SharedGatewayNetwork:  plan.SharedGatewayNetwork,
		ServiceAddress:        plan.ServiceAddress,
		EncryptionKeyFile:     plan.EncryptionKeyFile,
	}
	// The bearer token path is resolution-time state, not plan truth, and a
	// path that outlives its file invites a substitution; drop it (never a
	// value) from the persisted record.
	// HTTPGatewaySpec carries no credential reference: nothing to strip.
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(stateDir, "gateway-plan.json"), append(data, '\n'), 0o600)
}

// ReadPersesGatewayPlan loads a persisted plan record and re-validates it
// against the security contract: a record whose posture was weakened since it
// was written cannot be rendered from.
func ReadPersesGatewayPlan(stateDir string) (PersesGatewayPlan, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, "gateway-plan.json"))
	if err != nil {
		return PersesGatewayPlan{}, fmt.Errorf("read the persisted Perses gateway plan: %w", err)
	}
	var record PersesGatewayPlanRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return PersesGatewayPlan{}, fmt.Errorf("decode the persisted Perses gateway plan: %w", err)
	}
	if record.Version != 1 {
		return PersesGatewayPlan{}, fmt.Errorf("unsupported persisted Perses gateway plan version %d", record.Version)
	}
	if err := capability.ValidatePersesSecurityPolicy(record.Policy); err != nil {
		return PersesGatewayPlan{}, fmt.Errorf("the persisted Perses gateway plan no longer validates: %w", err)
	}
	if len(record.DeniedPaths) == 0 || record.DeniedPaths[0] != capability.PersesProxyPrefix {
		return PersesGatewayPlan{}, errors.New("the persisted Perses gateway plan must refuse the datasource proxy prefix")
	}
	// Re-derive the topology from the bind form so a tampered or stale record
	// cannot silently switch the rendering to the published branch: the
	// shared-namespace form binds a service address (a name), the
	// namespace-local form binds a loopback literal. A routable literal bind
	// is never a valid persisted plan.
	shared := true
	if addr, addrErr := netip.ParseAddr(record.UpstreamBindAddress); addrErr == nil {
		if !addr.Unmap().IsLoopback() {
			return PersesGatewayPlan{}, fmt.Errorf("the persisted Perses gateway plan bind %q is a routable literal address", record.UpstreamBindAddress)
		}
	} else if !validPersesBindAddress(record.UpstreamBindAddress) {
		return PersesGatewayPlan{}, fmt.Errorf("the persisted Perses gateway plan bind %q is neither a loopback literal nor a service address", record.UpstreamBindAddress)
	} else if upstreamHost := persesPersistedUpstreamHost(record.Spec.Upstream); upstreamHost != "" && upstreamHost != "127.0.0.1" && strings.EqualFold(upstreamHost, record.UpstreamBindAddress) {
		// A service-name bind whose upstream names the very same host is the
		// namespace-local pairing BuildPersesGateway verified at plan time: the
		// peer lives on a reachable backend network and the gateway routes to
		// that name. The shared-namespace form binds the namespace loopback and
		// routes to that same loopback literal, so a loopback-upstream pairing
		// is never the namespace-local form.
		shared = false
	}
	// The topology must be self-consistent. The namespace-local form binds the
	// loopback host its upstream names and reaches the peer over a routable
	// backend alias. The shared-namespace form binds the loopback of the
	// namespace its gateway joins and routes to that SAME loopback, because
	// Perses has no network of its own (network_mode joins the gateway
	// namespace) and the deployment-scoped service alias would otherwise
	// resolve to the gateway itself. The two forms are distinguished by the
	// bind: the namespace-local form binds a service name, the shared form
	// binds a loopback literal.
	upstreamHost := persesPersistedUpstreamHost(record.Spec.Upstream)
	if !record.SharedGatewayNetwork && !strings.EqualFold(upstreamHost, record.UpstreamBindAddress) {
		return PersesGatewayPlan{}, fmt.Errorf("the persisted Perses gateway plan routes to upstream host %q but binds %q: the peer can never be reached", upstreamHost, record.UpstreamBindAddress)
	}
	if record.SharedGatewayNetwork && strings.EqualFold(upstreamHost, record.UpstreamBindAddress) && validPersesBindAddress(record.UpstreamBindAddress) && !isLoopbackLiteral(record.UpstreamBindAddress) {
		return PersesGatewayPlan{}, fmt.Errorf("the persisted Perses gateway plan binds a service name %q while declaring the shared-namespace topology: that pairing is the namespace-local form", record.UpstreamBindAddress)
	}
	if record.SharedGatewayNetwork != shared {
		return PersesGatewayPlan{}, fmt.Errorf("the persisted Perses gateway plan declares shared=%v for bind %q, which does not match", record.SharedGatewayNetwork, record.UpstreamBindAddress)
	}
	return PersesGatewayPlan{
		Policy:                record.Policy,
		Spec:                  record.Spec,
		DeniedPaths:           append([]string(nil), record.DeniedPaths...),
		RequireClientIdentity: record.RequireClientIdentity,
		ProxyDenyReason:       record.ProxyDenyReason,
		UpstreamBindAddress:   record.UpstreamBindAddress,
		SharedGatewayNetwork:  record.SharedGatewayNetwork,
		ServiceAddress:        record.ServiceAddress,
		EncryptionKeyFile:     record.EncryptionKeyFile,
	}, nil
}

func persesPersistedUpstreamHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(parsed.Hostname(), "["), "]")
}

// PersesServiceMounts is the projection contract the production rendering
// (writePersesGatewayComposeServices) must match: the configuration mount, the
// key mount and the persistence volume, in the exact compose bind form. The
// single source of truth keeps the gateway renderer, the application
// composition and the deployment tests from drifting apart.
func PersesServiceMounts() []string {
	return []string{
		// The configuration is mounted FILE-wise at its contract path
		// (PersesConfigContainerPath) rather than the whole /etc/perses
		// directory: a directory bind of /etc/perses would shadow the
		// /etc/perses/database mountpoint. Mounting the single config file
		// read-only keeps /etc/perses writable so the tmpfs mountpoint for the
		// database folder and the named-volume mountpoint for the plugin
		// extraction target can both be created. Each bind owns its subtree,
		// so the two never shadow each other. The database tmpfs and the plugin
		// named volume are emitted separately (writePersesGatewayComposeServices),
		// not here, because they are not file binds.
		"./providers/management-ui/perses/config.yaml:" + PersesConfigContainerPath + ":ro",
		PersesEncryptionKeyComposeBindDir + ":" + PersesEncryptionKeyMountDir + ":ro",
		PersesPersistenceVolumeName + ":/persistence",
	}
}

// PersesEncryptionKeyComposeBindDir is the compose-relative host directory
// holding ONLY the container-readable encryption-key projection. The
// application realization lays it out exactly here (the compose bind source is
// contract, not derivation).
const PersesEncryptionKeyComposeBindDir = "./providers/perses-management/secrets"

// PersesPersistenceVolumeName is the compose volume key the Perses service
// mounts for its file database.
const PersesPersistenceVolumeName = "perses-data"

// RenderPersesCompose is the single entry the production compose writer calls.
// It takes the renderer explicitly, so no deployment depends on hidden global
// registration state.
func RenderPersesCompose(ctx context.Context, input PersesComposeInput) (string, error) {
	return persesComposeServices(ctx, input.Plan, input.Gateway, input.Release, input.EncryptionKeyProjection, input.EncryptionKeyComposeBind, input.Mounts, func() PersesConfigRenderer {
		if input.RenderConfig != nil {
			return input.RenderConfig
		}
		return registeredPersesConfigRenderer()
	}())
}

// PersesComposeServices renders the Perses service and its gateway. The Perses
// process binds the address from the plan and is never published; the gateway
// is the only host-visible address and carries the TLS and authentication
// requirements from the plan. The configuration the command line names is
// rendered from the validated policy by the registered realization renderer
// and mounted read-only, so the posture the contract verified is the posture
// the process actually serves.
// PersesComposeServices renders the Perses service and its gateway. The Perses
// process binds the address from the plan and is never published; the gateway
// is the only host-visible address and carries the TLS and authentication
// requirements from the plan. The configuration the command line names is
// rendered from the validated policy by the registered realization renderer
// and mounted read-only, so the posture the contract verified is the posture
// the process actually serves. This entry takes the encryption-key projection
// from the plan's effective policy: a realization that materializes the key
// records its path in the policy, and a rendering without one fails closed
// (the production composition uses RenderPersesCompose with an explicit
// projection field).
func PersesComposeServices(ctx context.Context, plan PersesGatewayPlan, files HTTPGatewayFiles, release capability.PersesRelease) (string, error) {
	return persesComposeServices(ctx, plan, files, release, plan.EncryptionKeyFile, "", nil, registeredPersesConfigRenderer())
}

func persesComposeServices(ctx context.Context, plan PersesGatewayPlan, files HTTPGatewayFiles, release capability.PersesRelease, keyProjection, keyComposeBind string, mounts []string, renderer PersesConfigRenderer) (string, error) {
	if err := capability.ValidatePersesSecurityPolicy(plan.Policy); err != nil {
		return "", err
	}
	if strings.TrimSpace(release.Image) == "" {
		return "", &capability.PersesNotConfiguredError{}
	}
	if strings.TrimSpace(files.Caddyfile) == "" {
		return "", errors.New("Perses management gateway requires its Caddyfile before rendering")
	}
	if renderer == nil {
		return "", ErrPersesConfigRendererMissing
	}
	configured, err := renderer(ctx, plan)
	if err != nil {
		return "", err
	}
	if expected, err := RenderPersesConfig(plan.Policy, serviceAddressOf(plan)); err != nil {
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
		// The Perses service joins the gateway container's network namespace
		// through network_mode. Docker Compose forbids network_mode and
		// networks on one service, so this branch emits network_mode
		// exclusively and never a networks: block — see fix-7 B1. Two
		// consequences the renderers must honor together: (a) Perses is NOT
		// registered as a backend-network alias of its own container, so the
		// gateway upstream cannot address it by the deployment alias — the
		// upstream is rewritten to the namespace loopback below, which the
		// shared namespace always reaches; (b) Perses still needs its own
		// backend alias for peer services (metric scrape, provisioning
		// callbacks), which the gateway's alias list carries instead, so the
		// alias exists on a network the gateway joins. The loopback bind on
		// the command line keeps the Perses port unforwardable on the host.
		fmt.Fprintf(&b, "    network_mode: %s\n", strconv.Quote("service:"+gatewayName))
	} else {
		// Namespace-local loopback: no published port, no expose, no network —
		// nothing can route to Perses from outside its own namespace.
		b.WriteString("    networks: []\n")
	}
	// The database folder /etc/perses/database does not exist as a directory
	// inside the pinned, distroless Perses image, and the container rootfs is
	// read-only. The pinned Perses release extracts the shipped plugin archives
	// into /etc/perses/plugins at startup, so that path is the writable target
	// the extraction needs: a deployment-scoped named volume replaces the
	// original tmpfs there, writable and owned by the deployment, never a host
	// bind. Perses never executes from it, so it keeps the isolation the tmpfs
	// motivated. The database folder is writable via a tmpfs of its own: the
	// mountpoint is created by runc even though the image has no such
	// directory, and the file-database folder is ephemeral per deployment. The
	// /tmp tmpfs is kept unchanged: /tmp exists in every image, so its
	// mountpoint is safe.
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - /tmp:rw,noexec,nosuid,nodev\n")
	b.WriteString("      - /etc/perses/database:rw,noexec,nosuid,nodev\n")
	// The configuration is rendered by the realization and mounted read-only;
	// BaseHarbor never rewrites it with a shell so that a rendering stays
	// deterministic and free of credential interpolation. Binding loopback is
	// the flag the product documents for this purpose: when the bind is a
	// loopback literal it is additionally pinned on the command line so no
	// configuration key can widen it. A Target-internal service-name bind is
	// what the gateway resolves over the backend network; the Perses flag only
	// accepts an address (not a hostname), so that topology binds through the
	// mounted configuration instead, and the plan has already verified the
	// address equals the upstream host the gateway routes to.
	b.WriteString("    command:\n")
	fmt.Fprintf(&b, "      - --config=%s\n", PersesConfigContainerPath)
	if addr, addrErr := netip.ParseAddr(plan.UpstreamBindAddress); addrErr == nil && addr.Unmap().IsLoopback() {
		fmt.Fprintf(&b, "      - --web.listen-address=%s:%d\n", plan.UpstreamBindAddress, PersesUpstreamPort)
	}
	fmt.Fprintf(&b, "    volumes:\n")
	// The datasource encryption key the rendered configuration references by
	// file. The realization must declare where it materialized the projected
	// key; a deployment that renders without declaring it cannot honor the
	// posture its configuration states (enable_auth without an encryption key
	// file that exists), so it fails closed here.
	if strings.TrimSpace(keyProjection) == "" {
		return "", errors.New("Perses service rendering requires the encryption-key projection path: the security configuration references it by file, so a rendering without a materialized key cannot honor the posture it states")
	}
	if _, err := os.Stat(keyProjection); err != nil {
		return "", fmt.Errorf("the Perses encryption key projection at %s is not materialized: %w", keyProjection, err)
	}
	if len(mounts) == 0 {
		// Registry-composition form: derive the bind sources from the declared
		// paths. The production composition passes the shared contract below.
		configBind := filepath.Dir(configured.HostPath)
		if configured.ComposeSourceDir != "" {
			configBind = configured.ComposeSourceDir
		}
		keyBind := filepath.Dir(keyProjection)
		if keyComposeBind != "" {
			keyBind = keyComposeBind
		}
		mounts = []string{
			composeBindSource(configBind) + ":" + PersesConfigMountDir + ":ro",
			composeBindSource(keyBind) + ":" + PersesEncryptionKeyMountDir + ":ro",
		}
	} else {
		// The shared contract binds the same files the proofs above checked;
		// the production layout is fixed, so the rendering cannot silently
		// widen or drop a mount.
		want := PersesServiceMounts()
		if len(mounts) != len(want) {
			return "", fmt.Errorf("the Perses mounts %v do not match the projection contract: reconcile the deployment", mounts)
		}
		for i := range mounts {
			if mounts[i] != want[i] {
				return "", fmt.Errorf("the Perses mount %q does not match the projection contract %q: reconcile the deployment", mounts[i], want[i])
			}
		}
	}
	for _, mount := range mounts {
		fmt.Fprintf(&b, "      - %s\n", strconv.Quote(mount))
	}
	// The shipped plugin archives extract into /etc/perses/plugins at startup;
	// the pinned distroless image has no such directory and its rootfs is
	// read-only, so a deployment-scoped named volume provides the writable,
	// host-isolated extraction target the tmpfs used to. Perses writes the
	// extracted module trees to it and never executes from it, so the isolation
	// the tmpfs motivated is preserved.
	fmt.Fprintf(&b, "      - perses-plugins:%s\n", PersesPluginsContainerPath)
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
	if expected, err := RenderPersesConfig(plan.Policy, serviceAddressOf(plan)); err != nil {
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
