package application

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// PersesUIImage is the pinned, reviewed Perses visualization image. It is
// realization metadata (product version), never a provider or capability
// version. Bump only on an reviewed security-current candidate.
const PersesUIImage = "docker.io/persesdev/perses:v0.55.1"

// PersesUIHostPortEnv is the application-scoped loopback host port the Perses
// management GATEWAY publishes on. Perses itself never owns a host port. It is
// materialized in the runtime env only when Perses is selected.
const PersesUIHostPortEnv = "BASEHARBOR_PERSES_UI_HOST_PORT"

// persesUIContainerTLSPort is the in-container HTTPS port the legacy renderer
// published directly. The gateway topology serves the plain upstream HTTP port
// on the namespace loopback instead; see writePersesGatewayComposeServices.
const persesUIContainerTLSPort = 8080

// PersesConfigHostPath returns the deployment-relative host file the
// realization materializes with the deterministic rendering of the validated
// security policy. The compose service bind-mounts it read-only at
// serviceaccess.PersesConfigContainerPath, so the --config flag always names a
// file with a proven producer; a rendering with absent or drifted contents
// fails the deployment closed instead of serving the anonymous upstream
// default. Perses writes only under /etc/perses/database and the mounted
// plugins tmpfs, so the read-only mount of /etc/perses itself is safe.
func PersesConfigHostPath(runtimeDir string) string {
	return filepath.Join(runtimeDir, "providers", "management-ui", "perses", "config.yaml")
}

// PersesEncryptionKeyHostPath is the authoritative 0600 owner-only host file
// holding the datasource encryption key of one deployment. It sits outside
// every mounted tree; only the projection under PersesEncryptionKeyProjection
// is ever bind-mounted into the container.
func PersesEncryptionKeyHostPath(runtimeDir string) string {
	return filepath.Join(runtimeDir, "secrets", "perses", "encryption.key")
}

// PersesEncryptionKeyProjection is the container-readable 0644 copy the
// rendering mounts read-only at serviceaccess.PersesEncryptionKeyMountDir. It
// is regenerated from the authoritative key on every reconciliation, never a
// second source of truth. A file bind would make the compose layer resolve the
// parents through root-owned directories the distroless nonroot UID cannot
// traverse, which is why the projection lives directly under the runtime root
// (0700 root, world-traversable by compose convention) with the file itself
// 0644 — the same shape as the shared-backend etcd pki projections.
func PersesEncryptionKeyProjection(runtimeDir string) string {
	return filepath.Join(runtimeDir, "providers", "perses-management", "secrets", "encryption-key")
}

// Perses is an OPTIONAL, provider-owned visualization surface that realizes the
// observability management-ui seam defined by the accepted Perses contract
// (task t-20261114-r28-perses-contract-seam, task t-20261114-r28-perses-runtime).
// It is never an application capability and never a required Core bundle: it
// rides the existing Docker/Podman compose lifecycle as a single-instance,
// managed, fail-closed service that is materialized ONLY when an application
// selects it (services.perses) AND a pinned release validates.

// PersesRuntimeVolumeName is the managed, application-scoped data volume for the
// optional Perses UI. It is a pure volume (no HA member, no gateway): Perses is
// a single-instance surface.
func PersesRuntimeVolumeName(resourceProject, service string) string {
	return resourceProject + "_" + service + "-data"
}

// PersesGatewayComposeFiles is the host Caddyfile the gateway service definition
// bind-mounts (its directory, read-only, at /etc/caddy).
func PersesGatewayComposeFiles(files RuntimeFiles) string {
	return filepath.Join(PersesGatewayStateDir(files), "config", "Caddyfile")
}

// PersesGatewayStateDir is the gateway projection directory for one deployment.
// It is deliberately NOT a subdirectory of providers/management-ui/perses:
// that tree belongs to the Perses service mounts (the rendered configuration
// directory binds into /etc/perses), and unrelated files there would appear
// inside the Perses container. The gateway service bind-mounts this tree.
func PersesGatewayStateDir(files RuntimeFiles) string {
	return filepath.Join(files.Dir, "providers", "perses-management", "gateway")
}

// writePersesUIComposeService is the legacy single-instance direct-publish
// renderer. As of fix-6 NO production or test path calls it: the production
// composition uses writePersesGatewayComposeServices, because the direct form
// publishes Perses itself on the host loopback. It is retained only so the
// workload security analyzer keeps a documented negative shape of a
// published management surface.
// grep witness (fix-6): `grep -rn writePersesUIComposeService --include=*.go .`
// shows only these comment lines and the function definition.
func writePersesUIComposeService(b *strings.Builder, m Manifest, resourceProject string) {
	service := runtimeServiceName("perses", defaultServiceInstance)
	b.WriteString("  " + service + ":\n")
	b.WriteString("    image: " + PersesUIImage + "\n")
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    entrypoint: [\"/bin/perses\"]\n")
	fmt.Fprintf(b, "    command: [\"--config=%s\", \"--web.listen-address=0.0.0.0:%d\", \"--web.tls-cert-file=/certs/server.cert\", \"--web.tls-key-file=/certs/server.key\", \"--log.level=error\"]\n", serviceaccess.PersesConfigContainerPath, persesUIContainerTLSPort)
	b.WriteString("    tmpfs:\n      - /etc/perses/plugins:rw,noexec,nosuid,nodev,mode=1777\n")
	b.WriteString("    ports:\n")
	fmt.Fprintf(b, "      - \"127.0.0.1:${%s}:%d\"\n", PersesUIHostPortEnv, persesUIContainerTLSPort)
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./providers/management-ui/perses/server.cert:/certs/server.cert:ro\n")
	b.WriteString("      - ./providers/management-ui/perses/server.key:/certs/server.key:ro\n")
	fmt.Fprintf(b, "      - ./providers/management-ui/perses/config.yaml:%s:ro\n", serviceaccess.PersesConfigMountDir)
	fmt.Fprintf(b, "      - %s-data:/persistence\n", service)
	b.WriteString("    networks:\n")
	b.WriteString("      default:\n")
	fmt.Fprintf(b, "        aliases:\n          - %q\n\n", devaccess.ApplicationAlias(m.Name, service))
}

// writePersesGatewayComposeServices renders the optional Perses surface for the
// production composition: the gateway service (TLS, authentication, denied
// datasource-proxy routes, the only host-visible address) plus the Perses
// service itself with network_mode into the gateway namespace, a loopback bind
// and no published port. The gateway reaches Perses through the backend
// network alias of the Perses service; inside the shared namespace Perses
// listens on loopback only.
//
// It is a pure rendering function of already-materialized state: every file it
// mounts must exist, produced by EnsurePersesManagementUI on the production
// ensure path. A deployment whose projection did not run (or whose mounted
// files drifted from their deterministic producers) fails closed here instead
// of rendering a service that cannot start or silently loses its claimed
// security.
func writePersesGatewayComposeServices(b *strings.Builder, m Manifest, files RuntimeFiles) error {
	// The rendering is a pure function of the materialized projection: the
	// plan the projection computed (upstream alias, backend network, key
	// provenance) must not be recomputed from hidden environment here, or a
	// drifted compose-time plan would fail closed on an applied deployment.
	plan, err := serviceaccess.ReadPersesGatewayPlan(PersesGatewayStateDir(files))
	if err != nil {
		return err
	}
	// The configuration's advertised address is part of the persisted plan
	// identity check: the record must name exactly this deployment's Perses
	// service alias, so a copied-in record cannot masquerade. In the
	// shared-namespace topology the gateway dials the namespace loopback (the
	// Perses service binds loopback inside the joined namespace and has no
	// network of its own), so the dial upstream is 127.0.0.1; the advertised
	// ServiceAddress still carries the deployment alias for the certificate
	// and configuration identities.
	wantService := mPersesServiceAlias(m)
	wantUpstream := "http://" + wantService + ":" + strconv.Itoa(serviceaccess.PersesUpstreamPort)
	loopbackUpstream := "http://127.0.0.1:" + strconv.Itoa(serviceaccess.PersesUpstreamPort)
	if plan.ServiceAddress != wantService ||
		(plan.Spec.Upstream != wantUpstream && plan.Spec.Upstream != loopbackUpstream) {
		return errors.New("the persisted Perses gateway plan does not name this deployment's Perses service address: reconcile the deployment")
	}
	if !plan.SharedGatewayNetwork {
		return errors.New("the Perses compose rendering expects the shared-namespace gateway topology the plan derives")
	}
	// The configuration bytes are re-proved against the materialized file
	// inside the renderer (RenderPersesCompose fails closed on drift); the
	// existence of the mounted projection as a whole is guaranteed by
	// EnsurePersesManagementUI, which the managed lifecycle runs before this
	// rendering and which checkPersesCompositionFiles asserts there.
	input := persesComposeInput(files, plan)
	// The gateway bind sources in the rendering are deterministic; existence
	// of every mounted file (gateway Caddyfile and material included) is
	// re-proved at composition time, so a projection that was tampered with
	// after EnsurePersesManagementUI ran cannot render either.
	if err := checkPersesCompositionFiles(files); err != nil {
		return err
	}
	rendered, err := serviceaccess.RenderPersesCompose(context.Background(), input)
	if err != nil {
		return err
	}
	b.WriteString(rendered)
	return nil
}

// persesComposeInput builds the production rendering input: the projected
// gateway files, the pinned release, and the configuration renderer that proves
// the mounted file against the deterministic rendering instead of rewriting it
// from a pure function.
func persesComposeInput(files RuntimeFiles, plan serviceaccess.PersesGatewayPlan) serviceaccess.PersesComposeInput {
	// The service definitions mount these relative projection paths, so they
	// are the rendering contract; the absolute host paths below only prove
	// the files exist before a single byte renders.
	return serviceaccess.PersesComposeInput{
		Plan: plan,
		Gateway: serviceaccess.HTTPGatewayFiles{
			// The rendering binds the gateway projection exactly where
			// EnsurePersesManagementUI materialized it (the serviceaccess
			// layout); the compose bind source is the deterministic relative
			// form of the same tree.
			Dir:       "./providers/perses-management/gateway/service-access",
			Caddyfile: "./providers/perses-management/gateway/service-access/config/Caddyfile",
			Material: serviceaccess.TLSMaterial{
				CA:                "./providers/perses-management/gateway/service-access/runtime/ca.pem",
				ServerCertificate: "./providers/perses-management/gateway/service-access/runtime/server.pem",
				ServerKey:         "./providers/perses-management/gateway/service-access/runtime/server-key.pem",
				ServerName:        serviceaccess.PersesGatewayServiceName,
			},
		},
		Release: capability.PersesDefaultRelease,
		// The rendering mounts these projected paths directly (relative to the
		// compose file): the absolute host sources of a managed composition
		// would pin the deployment root into the definition and break its
		// relocation, so the bind sources are the deterministic projection
		// layout EnsurePersesManagementUI owns. The renderer below re-proves
		// the mounted configuration byte-for-byte before any output.
		EncryptionKeyProjection:  PersesEncryptionKeyProjection(files.Dir),
		EncryptionKeyComposeBind: serviceaccess.PersesEncryptionKeyComposeBindDir,
		// Persistence volume rides the gateway-persisted layout: the data the
		// server holds is rebuilt from BaseHarbor-owned provisioning, mounted
		// under the managed volume name the lifecycle declares.
		Mounts: serviceaccess.PersesServiceMounts(),
		RenderConfig: func(_ context.Context, current serviceaccess.PersesGatewayPlan) (serviceaccess.PersesRenderedConfig, error) {
			// The compose rendering must reproduce the bytes the projection
			// materialized. The projection's effective policy is pure
			// contract state (no manifest-derived fields), so rendering it is
			// deterministic in the persisted plan alone.
			contents, err := serviceaccess.RenderPersesConfig(current.Policy, current.ServiceAddress)
			if err != nil {
				return serviceaccess.PersesRenderedConfig{}, err
			}
			hostPath := PersesConfigHostPath(files.Dir)
			onDisk, err := os.ReadFile(hostPath)
			if err != nil {
				return serviceaccess.PersesRenderedConfig{}, err
			}
			if string(onDisk) != contents {
				return serviceaccess.PersesRenderedConfig{}, fmt.Errorf("the materialized Perses configuration at %s drifted from the deterministic rendering: reconcile the deployment", filepath.Base(hostPath))
			}
			return serviceaccess.PersesRenderedConfig{HostPath: hostPath, Contents: contents}, nil
		},
	}
}

// checkPersesCompositionFiles proves every host file the rendering mounts
// exists before a single byte renders: the relative bind sources in the
// output are the projection contract EnsurePersesManagementUI materializes,
// and an absent file would start containers that silently lose their claimed
// security (B2 of the fix-6 contract).
func checkPersesCompositionFiles(files RuntimeFiles) error {
	// The gateway service definition bind-mounts its Caddyfile's directory
	// and the material directory (serviceaccess renders both bind sources
	// from the projected paths), so the proof below looks at exactly the
	// locations the rendering mounts — the serviceaccess layout under
	// providers/perses-management/gateway/service-access.
	accessDir := filepath.Join(files.Dir, "providers", "perses-management", "gateway", "service-access")
	for _, required := range []string{
		filepath.Join(accessDir, "config", "Caddyfile"),
		filepath.Join(accessDir, "runtime", "server.pem"),
		filepath.Join(accessDir, "runtime", "server-key.pem"),
		filepath.Join(accessDir, "runtime", "ca.pem"),
		PersesConfigHostPath(files.Dir),
		PersesEncryptionKeyProjection(files.Dir),
	} {
		if _, err := os.Stat(required); err != nil {
			return fmt.Errorf("the Perses composition requires the materialized file %s: %w", required, err)
		}
	}
	return nil
}

// isPersesLoopbackLiteral reports whether a bind value is a loopback address
// literal (the value the command line pins), as opposed to a service address.
func isPersesLoopbackLiteral(raw string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	return err == nil && addr.Unmap().IsLoopback()
}

// generatePersesEncryptionKey draws 32 cryptographically random bytes for the
// datasource encryption key of one deployment.
func generatePersesEncryptionKey() ([]byte, error) {
	raw := make([]byte, 32)
	n, err := rand.Read(raw)
	if err != nil || n != len(raw) {
		return nil, errors.New("generate Perses encryption key: cryptographic randomness unavailable")
	}
	return raw, nil
}

// ensurePersesEncryptionKey produces the authoritative Perses datasource
// encryption key for one deployment and the container-readable projection the
// rendering mounts. It is deterministic on reuse: an existing 0600 key is kept
// (the Perses database decrypts with it), a key file that lost its owner-only
// mode or a too-short file fails closed instead of silently weakening or
// destroying the secret. The key value never appears in a log, compose file or
// rendered configuration; the configuration references the projected path only.
func ensurePersesEncryptionKey(runtimeDir string) error {
	keyPath := PersesEncryptionKeyHostPath(runtimeDir)
	projection := PersesEncryptionKeyProjection(runtimeDir)
	key, err := os.ReadFile(keyPath)
	if os.IsNotExist(err) {
		key, err = generatePersesEncryptionKey()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
			return fmt.Errorf("create Perses encryption key directory: %w", err)
		}
		// writeOwnerOnlyFile is the shared atomic 0600 owner-only path (temp
		// file in-directory, chmod before rename), so no window exists where
		// the material is group- or world-readable.
		if err := writeOwnerOnlyFile(keyPath, key); err != nil {
			return fmt.Errorf("persist the Perses encryption key: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("read the Perses encryption key: %w", err)
	} else {
		info, statErr := os.Stat(keyPath)
		if statErr != nil {
			return fmt.Errorf("inspect the Perses encryption key: %w", statErr)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("the Perses encryption key at %s is not owner-only (mode %o): it must be 0600", filepath.Base(keyPath), info.Mode().Perm())
		}
		if len(key) < 32 {
			return fmt.Errorf("the Perses encryption key at %s is too short to be a generated key: refusing to reuse it", filepath.Base(keyPath))
		}
	}
	// The bind directory is minimal and traversal-readable (0755 under the
	// 0700-private providers parent is NOT enough for the compose bind: the
	// whole path must be traversable by the container UID, and this tree
	// carries only the 0644 projection).
	if err := os.MkdirAll(filepath.Dir(projection), 0o755); err != nil {
		return fmt.Errorf("create the Perses projection directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(projection), 0o755); err != nil {
		return fmt.Errorf("set the Perses projection directory permissions: %w", err)
	}
	// The projection copy is rewritten from the authoritative key on every
	// reconciliation; its 0644 mode (writeUIRuntimeProjection) is what makes
	// the distroless nonroot container able to read it.
	if err := writeUIRuntimeProjection(projection, key); err != nil {
		return fmt.Errorf("project the Perses encryption key: %w", err)
	}
	return nil
}

// writePersesGatewayPlanState persists the credential-free gateway plan record
// through the serviceaccess codec.
func writePersesGatewayPlanState(stateDir string, plan serviceaccess.PersesGatewayPlan) error {
	return serviceaccess.PersistPersesGatewayPlan(stateDir, plan)
}

// PersesUITLSRequired reports whether Perses TLS provider files must be projected
// for the current manifest. TLS is projected only for a selected, observability
// surface so a deselected surface never materializes any runtime artifact.
func PersesUITLSRequired(m Manifest) bool {
	return m.Services.Perses && m.Services.ObservabilityManagementUI
}

// EnsurePersesManagementUI projects the optional Perses provider files (TLS +
// persistence marker) under providers/management-ui/perses/runtime. It is
// fail-closed: it is a no-op unless Perses is selected with observability
// management_ui, so an unselected surface leaves no files behind.
func EnsurePersesManagementUI(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest) error {
	if !PersesUITLSRequired(m) {
		return nil
	}
	if issuer == nil {
		return fmt.Errorf("perses TLS material requires a service issuer")
	}
	dir := filepath.Join(files.Dir, "providers", "management-ui", "perses")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// The mounted configuration is rendered from the validated security
	// policy before anything else: a deployment whose policy cannot validate
	// must not materialize a Perses service that would then serve the upstream
	// anonymous default. The managed deployment validates the strict posture
	// itself, so the rendering is deterministic by construction.
	// The mounted configuration is rendered from the effective policy of the
	// validated gateway plan: service address (the backend-network alias the
	// gateway routes to) plus the Target-scoped proxy deny lists, exactly the
	// bytes the compose-time drift check re-derives from the persisted plan.
	// The placeholder rendering below only proves the posture validates.
	security := capability.DefaultPersesSecurityPolicy()
	if _, err := serviceaccess.RenderPersesConfig(security, ""); err != nil {
		return err
	}
	// its container-readable projection here: the rendered configuration
	// names PersesEncryptionKeyContainerPath, and a deployment whose key
	// cannot be produced fails closed before any service definition renders.
	if err := ensurePersesEncryptionKey(files.Dir); err != nil {
		return err
	}
	// Perses is reached only through its management gateway (0428-089): the
	// gateway owns the operator-visible TLS listener and the authentication
	// the resolved service-access policy enforces. The projection uses the
	// same EnsureHTTPGateway machinery as every other managed surface, so the
	// compose rendering can mount material with a proven producer.
	plan, err := PersesGatewayPlanFor(m, security)
	if err != nil {
		return err
	}
	if plan.ServiceAddress == "" {
		return errors.New("the Perses gateway plan must state the service address the configuration advertises")
	}
	// fix-7 B1: the Perses service joins the gateway namespace through
	// network_mode, which Docker Compose forbids alongside a networks:
	// entry, so Perses itself registers NO backend-network alias and the
	// alias can never resolve to a routable Perses address — peers that
	// address bh-dev-<name>-perses:8080 are refused, because Perses serves
	// the namespace loopback only. The alias therefore rides the GATEWAY
	// service (the sole backend-network member of this surface) through
	// NetworkAliases, which HTTPGatewayComposeService emits under the
	// FIRST network (http_gateway.go i==0 branch), and the gateway's
	// upstream names it on that network. Both the spec networks and the
	// alias list are set HERE (the projection owns the deployment-scoped
	// compose membership) and persist in the plan record before the
	// config.yaml is rendered, so the configuration and the compose
	// rendering derive from one record and stay byte-identical.
	plan.Spec.Networks = []string{"default"}
	plan.Spec.NetworkAliases = []string{plan.ServiceAddress}
	contents, err := serviceaccess.RenderPersesConfig(plan.Policy, plan.ServiceAddress)
	if err != nil {
		return err
	}
	if err := writeUIRuntimeProjection(filepath.Join(dir, "config.yaml"), []byte(contents)); err != nil {
		return err
	}
	accessPolicy, err := serviceaccess.Resolve(m.Environment, serviceaccess.PersesGatewayProvider, serviceaccess.AuthenticationMTLS)
	if err != nil {
		return err
	}
	stateDir := PersesGatewayStateDir(files)
	if _, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, stateDir, plan.Spec); err != nil {
		return fmt.Errorf("project the Perses management gateway: %w", err)
	}
	// The gateway plan is persisted only AFTER the gateway material and
	// Caddyfile it describes exist: the compose rendering reads this record,
	// so a half-projected deployment never renders a service that mounts
	// missing files.
	// The key projection this deployment materialized is part of the plan
	// record the pure compose rendering reads back (reference path only,
	// never a value).
	plan.EncryptionKeyFile = filepath.Join(PersesEncryptionKeyProjection(files.Dir), "encryption-key")
	if err := writePersesGatewayPlanState(stateDir, plan); err != nil {
		return err
	}
	// The gateway owns the operator-visible TLS listener; the Perses service
	// itself serves plain HTTP on the namespace loopback, so no UI-side
	// server.cert/server.key projection is needed (the legacy renderer's
	// --web.tls-* flags are gone with the gateway topology). The three-state
	// contract still never declares a verified health path.
	// The config directory is bind-mounted whole into the container, so its
	// host tree carries ONLY contract files: the rendered config.yaml plus the
	// persistence marker the lifecycle verify gate reads. Legacy direct-publish
	// material (server.cert/server.key and their UI PKI tree, projected by the
	// pre-gateway renderer) is retired so it can never leak into /etc/perses.
	for _, stale := range []string{"server.cert", "server.key"} {
		if err := os.Remove(filepath.Join(dir, stale)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("retire the stale Perses projection file %s: %w", stale, err)
		}
	}
	if err := os.RemoveAll(filepath.Join(dir, "pki")); err != nil {
		return fmt.Errorf("retire the stale Perses UI PKI tree: %w", err)
	}
	marker := filepath.Join(dir, "persistence")
	if err := writeUIRuntimeProjection(marker, []byte("perses\n")); err != nil {
		return err
	}
	// Every file the compose rendering mounts must exist before this function
	// returns: the projection, not the pure rendering, is the fail-closed
	// checkpoint for the mounted set (B1/B2 of the fix-6 contract).
	return checkPersesCompositionFiles(files)
}

// PersesManagementUIPath returns the runtime projection directory for the
// optional Perses UI, used by the lifecycle verify gate.
func PersesManagementUIPath(files RuntimeFiles) string {
	return filepath.Join(files.Dir, "providers", "management-ui", "perses")
}

// persesRenderingProver is the production configuration renderer: it renders
// the deterministic configuration of the plan's effective policy and proves
// the materialized host file carries exactly those bytes. A deployment whose
// mounted configuration drifted fails closed here. When the composition runs
// without a deployment directory (pure projections), the proof resolves the
// mounted path relative to the compose file's own directory, which is where
// the relative bind sources point.
func persesRenderingProver(files RuntimeFiles) serviceaccess.PersesConfigRenderer {
	return func(_ context.Context, current serviceaccess.PersesGatewayPlan) (serviceaccess.PersesRenderedConfig, error) {
		contents, err := serviceaccess.RenderPersesConfig(current.Policy, current.ServiceAddress)
		if err != nil {
			return serviceaccess.PersesRenderedConfig{}, err
		}
		hostPath := PersesConfigHostPath(files.Dir)
		if files.Dir == "" {
			hostPath = filepath.Join("providers", "management-ui", "perses", "config.yaml")
		}
		onDisk, err := os.ReadFile(hostPath)
		if err != nil {
			return serviceaccess.PersesRenderedConfig{}, err
		}
		if string(onDisk) != contents {
			return serviceaccess.PersesRenderedConfig{}, fmt.Errorf("the materialized Perses configuration at %s drifted from the deterministic rendering: reconcile the deployment", filepath.Base(hostPath))
		}
		// The compose mount source is always the deterministic relative
		// projection path (the file's location inside the runtime directory),
		// never this deployment's absolute path: the definition must survive
		// relocation exactly like every other managed bind.
		mountSource := filepath.Join("providers", "management-ui", "perses", "config.yaml")
		return serviceaccess.PersesRenderedConfig{HostPath: hostPath, Contents: contents, ComposeSourceDir: mountSource}, nil
	}
}

// mPersesServiceAlias is the backend-network alias of this deployment's Perses
// service (the empty-name form matches the plan derivation in
// PersesGatewayPlanFor so the identity proof compares equal forms).
func mPersesServiceAlias(m Manifest) string {
	if strings.TrimSpace(m.Name) == "" {
		return serviceaccess.PersesServiceName
	}
	return devaccess.ApplicationAlias(m.Name, serviceaccess.PersesServiceName)
}
