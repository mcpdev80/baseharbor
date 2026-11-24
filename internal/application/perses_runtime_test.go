package application

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"go.yaml.in/yaml/v3"
)

// persesRuntimeManifest builds a minimal, valid application manifest that
// selects the optional Perses observability surface. Perses is a management UI
// realization that rides the managed compose backend, so a single SQL instance
// grounds the runtime and satisfies CheckSupportedRuntimeServices.
func persesRuntimeManifest(t *testing.T, persesSelected bool) Manifest {
	t.Helper()
	useApplicationScopedDataProviders(t)
	m := New("perses-runtime", "prod", true, false, false)
	m = WithWorkloadComponents(m, "api")
	m = WithSQLInstances(m, "primary")
	m = WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = persesSelected
	if err := m.Validate(); err != nil {
		t.Fatalf("validate perses manifest: %v", err)
	}
	return m
}

// persesRuntimeFiles materializes the runtime environment, compose and provider
// files for a Perses manifest WITHOUT starting a container. It exercises the
// production EnsureRuntime path so Perses provider files are projected exactly
// as they would be on apply.
func persesRuntimeFiles(t *testing.T, m Manifest) RuntimeFiles {
	t.Helper()
	store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "perses-runtime"}
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatalf("ensure Perses runtime files: %v", err)
	}
	return files
}

// TestPersesComposeRenderedWhenSelected asserts that a selected Perses surface
// materializes the gateway-protected compose topology (0428-089): the gateway
// service owns the only host-visible loopback port, the Perses service joins
// the gateway's network namespace through network_mode and declares no ports
// of its own. Fail-closed: deselected Perses renders no service.
func TestPersesComposeRenderedWhenSelected(t *testing.T) {
	selected := persesRuntimeManifest(t, true)
	selectedFiles := persesRuntimeFiles(t, selected)
	rendered, err := RuntimeComposeYAMLForProject(selected, selectedFiles)
	if err != nil {
		t.Fatalf("render selected Perses compose: %v", err)
	}
	persesService := runtimeServiceName("perses", defaultServiceInstance)
	if !strings.Contains(rendered, "  "+persesService+":") {
		t.Fatalf("selected Perses must render a single-instance compose service:\n%s", rendered)
	}
	if !strings.Contains(rendered, capability.PersesDefaultRelease.Image) {
		t.Fatalf("selected Perses must pin the reviewed image %q: %s", capability.PersesDefaultRelease.Image, rendered)
	}
	if !strings.Contains(rendered, serviceaccess.PersesGatewayServiceName+":") {
		t.Fatalf("selected Perses must render its management gateway service:\n%s", rendered)
	}
	if !strings.Contains(rendered, PersesUIHostPortEnv) {
		t.Fatalf("the gateway must publish the loopback host port via %q: %s", PersesUIHostPortEnv, rendered)
	}
	if !strings.Contains(rendered, PersesRuntimeVolumeName(selectedFiles.ResourceProject, persesService)) {
		t.Fatalf("selected Perses must declare a managed data volume:\n%s", rendered)
	}
	var compose struct {
		Services map[string]struct {
			Volumes     []string `yaml:"volumes"`
			Ports       []string `yaml:"ports"`
			NetworkMode string   `yaml:"network_mode"`
		} `yaml:"services"`
		Volumes map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &compose); err != nil {
		t.Fatalf("decode selected Perses compose: %v", err)
	}
	// The Perses service itself is never published: only the gateway owns a
	// host port.
	if len(compose.Services[persesService].Ports) != 0 {
		t.Fatalf("the Perses service must declare no published ports, got %v", compose.Services[persesService].Ports)
	}
	if compose.Services[persesService].NetworkMode != "service:"+serviceaccess.PersesGatewayServiceName {
		t.Fatalf("the Perses service must join the gateway namespace, got %q", compose.Services[persesService].NetworkMode)
	}
	if len(compose.Services[serviceaccess.PersesGatewayServiceName].Ports) == 0 {
		t.Fatal("the gateway must publish the operator-visible loopback port")
	}
	volumeKey := persesService + "-data"
	volume, ok := compose.Volumes[volumeKey]
	if !ok || volume.Name != PersesRuntimeVolumeName(selectedFiles.ResourceProject, persesService) {
		t.Fatalf("selected Perses volume declaration = %+v, want %q -> %q", volume, volumeKey, PersesRuntimeVolumeName(selectedFiles.ResourceProject, persesService))
	}
	mounted := false
	for _, mount := range compose.Services[persesService].Volumes {
		if mount == volumeKey+":/persistence" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatalf("Perses service mounts %q for persistence, want %q:\n%s", compose.Services[persesService].Volumes, volumeKey+":/persistence", rendered)
	}

	deselected := persesRuntimeManifest(t, false)
	deselectedFiles := persesRuntimeFiles(t, deselected)
	deselectedRendered, err := RuntimeComposeYAMLForProject(deselected, deselectedFiles)
	if err != nil {
		t.Fatalf("render deselected Perses compose: %v", err)
	}
	if strings.Contains(deselectedRendered, "  "+persesService+":") {
		t.Fatalf("deselected Perses must render no compose service (fail-closed):\n%s", deselectedRendered)
	}
}

// TestPersesDatabaseVolumeReplacementRegression pins the fix for the hosted
// runner tmpfs mountpoint defect: the pinned, distroless Perses image has no
// /etc/perses/database directory and its rootfs is read-only, and Perses 0.55.1
// extracts the shipped plugin archives into /etc/perses/plugins at startup.
// Both paths must be writable on the read-only rootfs. The Perses service must
// therefore mount /etc/perses/plugins on a dedicated, application-scoped named
// volume (the writable extraction target), keep /etc/perses/database writable
// via a tmpfs of its own (the mountpoint is created by runc even though the
// image ships no such directory), and both mounts must be declared. The test
// goes RED the moment the named plugin volume is removed, the plugin path is
// reverted to a tmpfs, or the named volume is declared under the wrong key.
func TestPersesDatabaseVolumeReplacementRegression(t *testing.T) {
	selected := persesRuntimeManifest(t, true)
	selectedFiles := persesRuntimeFiles(t, selected)
	rendered, err := RuntimeComposeYAMLForProject(selected, selectedFiles)
	if err != nil {
		t.Fatalf("render selected Perses compose: %v", err)
	}
	persesService := runtimeServiceName("perses", defaultServiceInstance)

	var compose struct {
		Services map[string]struct {
			Volumes []string `yaml:"volumes"`
			Tmpfs   []string `yaml:"tmpfs"`
		} `yaml:"services"`
		Volumes map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &compose); err != nil {
		t.Fatalf("decode selected Perses compose: %v", err)
	}

	// The Perses database folder is writable via a tmpfs of its own: runc
	// creates the mountpoint even though the distroless image has no such
	// directory, and the file-database folder is ephemeral per deployment.
	if !strings.Contains(rendered, serviceaccess.PersesDatabaseContainerPath+":rw,noexec,nosuid,nodev") {
		t.Fatalf("Perses must mount a tmpfs at %s for the writable database folder:\n%s", serviceaccess.PersesDatabaseContainerPath, rendered)
	}

	// The shipped plugin archives extract into /etc/perses/plugins; that path
	// must be a dedicated, application-scoped NAMED VOLUME (not a tmpfs): a
	// tmpfs there would re-trigger the runc mountpoint failure on the runner
	// and block the extraction the hosted runner must support.
	pluginsVolumeKey := persesService + "-plugins"
	if _, ok := compose.Volumes[pluginsVolumeKey]; !ok {
		t.Fatalf("selected Perses must declare a dedicated database volume %q:\n%s", pluginsVolumeKey, rendered)
	}
	mounted := false
	for _, mount := range compose.Services[persesService].Volumes {
		if mount == pluginsVolumeKey+":/"+strings.TrimPrefix(serviceaccess.PersesPluginsContainerPath, "/") {
			mounted = true
		}
	}
	if !mounted {
		t.Fatalf("Perses service must mount the dedicated plugin volume at %s, got %q:\n%s", serviceaccess.PersesPluginsContainerPath, compose.Services[persesService].Volumes, rendered)
	}
	// The plugin path must never be a tmpfs: that mountpoint is what the hosted
	// runner rejects.
	for _, mount := range compose.Services[persesService].Tmpfs {
		if strings.Contains(mount, serviceaccess.PersesPluginsContainerPath) {
			t.Fatalf("Perses must not mount a tmpfs at %s (hosted-runner mountpoint defect):\n%s", serviceaccess.PersesPluginsContainerPath, rendered)
		}
	}
}

// TestPersesRuntimeFilesProjectProviderFiles asserts the production EnsureRuntime
// path projects Perses provider files ONLY when the surface is selected, so an
// unselected surface leaves no runtime artifacts behind.
func TestPersesRuntimeFilesProjectProviderFiles(t *testing.T) {
	selected := persesRuntimeManifest(t, true)
	selectedFiles := persesRuntimeFiles(t, selected)
	if _, err := os.Stat(filepath.Join(PersesManagementUIPath(selectedFiles), "persistence")); err != nil {
		t.Fatalf("selected Perses must project a persistence marker under providers/management-ui/perses: %v", err)
	}

	deselected := persesRuntimeManifest(t, false)
	deselectedFiles := persesRuntimeFiles(t, deselected)
	if _, err := os.Stat(filepath.Join(PersesManagementUIPath(deselectedFiles), "persistence")); err == nil {
		t.Fatalf("deselected Perses must not project provider files:\n%+v", deselectedFiles)
	}
}

// persesComposeFragment renders the compose definition and returns its service
// block. The production renderer normalizes YAML quoting (mounts and port specs
// can arrive quoted), so the Perses assertions must parse the services they
// target instead of the whole document.
func persesComposeFragment(t *testing.T, m Manifest, files RuntimeFiles) string {
	t.Helper()
	rendered, err := RuntimeComposeYAMLForProject(m, files)
	if err != nil {
		t.Fatalf("render selected Perses compose: %v", err)
	}
	services, ok := strings.CutPrefix(rendered, "services:\n")
	if !ok {
		t.Fatalf("unexpected compose document shape:\n%s", rendered)
	}
	if end := strings.Index(services, "\nvolumes:\n"); end >= 0 {
		services = services[:end]
	}
	return services
}

// TestPersesTLSFilesProjectedAndConsumed is the CODEX-REVIEW #1592 regression
// carried onto the gateway topology (0428-089): the operator-visible TLS
// listener is the management gateway, so every gateway TLS file the compose
// service mounts must exist in the runtime projection with the right modes
// (the projected copy readable 0644 for the unprivileged gateway, the PKI
// working key private 0600), and the gateway certificate must cover the names
// the contract's observability management-ui protocol (https) is served under.
// The Perses service itself serves plain HTTP on the namespace loopback behind
// the gateway (its HTTPS is the gateway's job).
func TestPersesTLSFilesProjectedAndConsumed(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	files := persesRuntimeFiles(t, m)
	rendered := persesComposeFragment(t, m, files)
	stateDir := PersesGatewayStateDir(files)
	if !strings.Contains(rendered, serviceaccess.PersesGatewayServiceName+":") {
		t.Fatalf("the compose must render the Perses management gateway service:\n%s", rendered)
	}
	accessDir := filepath.Join(stateDir, "service-access")
	for _, required := range []string{
		filepath.Join(accessDir, "config", "Caddyfile"),
		filepath.Join(accessDir, "runtime", "server.pem"),
		filepath.Join(accessDir, "runtime", "server-key.pem"),
		filepath.Join(accessDir, "runtime", "ca.pem"),
	} {
		if _, err := os.Stat(required); err != nil {
			t.Fatalf("the gateway projection must materialize %s: %v", required, err)
		}
	}
	// The projected key copy is the readable bind-mount file; the authoritative
	// PKI material stays private.
	keyInfo, err := os.Stat(filepath.Join(accessDir, "runtime", "server-key.pem"))
	if err != nil {
		t.Fatalf("stat gateway projected key: %v", err)
	}
	if keyInfo.Mode().Perm() != 0o644 {
		t.Fatalf("the projected gateway key must be the readable bind-mount copy 0644, got %v", keyInfo.Mode().Perm())
	}
	pkiKeyInfo, err := os.Stat(filepath.Join(accessDir, "pki", "server-key.pem"))
	if err != nil {
		t.Fatalf("stat gateway PKI working key: %v", err)
	}
	if pkiKeyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("the gateway PKI working key must stay private 0600, got %v", pkiKeyInfo.Mode().Perm())
	}
	certData, err := os.ReadFile(filepath.Join(accessDir, "runtime", "server.pem"))
	if err != nil {
		t.Fatalf("read projected gateway certificate: %v", err)
	}
	certBlock, _ := pem.Decode(certData)
	if certBlock == nil {
		t.Fatal("projected gateway certificate must be PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("parse projected gateway certificate: %v", err)
	}
	names := map[string]bool{"localhost": false, "127.0.0.1": false, serviceaccess.PersesGatewayServiceName: false}
	for _, san := range cert.DNSNames {
		names[san] = true
	}
	for _, ip := range cert.IPAddresses {
		if ip.String() == "127.0.0.1" {
			names["127.0.0.1"] = true
		}
	}
	for name, covered := range names {
		if !covered {
			t.Fatalf("the gateway certificate must cover %q (contract protocol https): %+v", name, cert.DNSNames)
		}
	}
	if cert.NotAfter.Before(time.Now()) {
		t.Fatal("projected gateway certificate must not be expired")
	}
}

func TestPersesLifecycleResourcesIncludeManagedService(t *testing.T) {
	selected := persesRuntimeManifest(t, true)
	resources := ExpectedRuntimeResources(selected)
	persesService := runtimeServiceName("perses", defaultServiceInstance)
	var haveContainer, haveVolume bool
	for _, r := range resources {
		if r.Kind == "container" && strings.Contains(r.Name, "perses") {
			haveContainer = true
		}
		if r.Kind == "volume" && r.Name == PersesRuntimeVolumeName(RuntimeProjectName(selected), persesService) {
			haveVolume = true
		}
	}
	if !haveContainer {
		t.Fatalf("ExpectedRuntimeResources must include the managed Perses UI container (got %v)", resources)
	}
	if !haveVolume {
		t.Fatalf("ExpectedRuntimeResources must include the managed Perses data volume (got %v)", resources)
	}
}

// TestPersesNotConfiguredWithoutSelection asserts the three-state contract stays
// fail-closed: without selection the surface declares no observability entry and
// reports not_configured, and no Perses provider files are projected.
func TestPersesNotConfiguredWithoutSelection(t *testing.T) {
	m := persesRuntimeManifest(t, false)
	files := persesRuntimeFiles(t, m)
	surfaces, status, err := ApplicationManagementUISurfaces(m, files)
	if err != nil {
		t.Fatalf("manage Perses surfaces: %v", err)
	}
	for _, s := range surfaces {
		if s.Service == "observability" {
			t.Fatalf("deselected Perses must not declare an observability surface: %+v", surfaces)
		}
	}
	if *status != capability.PersesStatusNotConfigured {
		t.Fatalf("deselected Perses status = %q, want not_configured", *status)
	}
}

// TestPersesConfiguredUnverifiedWhenSelectedPinned asserts the fail-closed
// positive contract: selected + pinned release with no real per-target health
// probe reports "configured_unverified" (0428-186, CODEX-REVIEW correction —
// verified is a reserved probe result, no longer derived from the static
// reference catalog), and the surface is declared.
func TestPersesConfiguredUnverifiedWhenSelectedPinned(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	files := persesRuntimeFiles(t, m)
	surfaces, status, err := ApplicationManagementUISurfaces(m, files)
	if err != nil {
		t.Fatalf("manage Perses surfaces: %v", err)
	}
	var declared bool
	for _, s := range surfaces {
		if s.Service == "observability" {
			declared = true
		}
	}
	if !declared {
		t.Fatal("selected+pinned Perses must declare an observability surface")
	}
	if *status != capability.PersesStatusConfiguredUnverified {
		t.Fatalf("selected+pinned Perses without a real probe status = %q, want configured_unverified", *status)
	}
}

// TestPersesRuntimeVolumeNameIsStableAndIsolated pins the volume naming so a
// Perses surface is never shared across projects.
func TestPersesRuntimeVolumeNameIsStableAndIsolated(t *testing.T) {
	a := PersesRuntimeVolumeName("baseharbor-app-a-prod", "perses-default")
	b := PersesRuntimeVolumeName("baseharbor-app-b-prod", "perses-default")
	if a == b {
		t.Fatalf("Perses volumes must be project-scoped: %q == %q", a, b)
	}
	if !strings.HasPrefix(a, "baseharbor-app-a-prod_") {
		t.Fatalf("unexpected Perses volume name %q", a)
	}
}

// TestPersesUITLSRequiredIsFailClosed pins that TLS/provider projection for
// Perses is a no-op unless the surface is selected with observability
// management_ui.
func TestPersesUITLSRequiredIsFailClosed(t *testing.T) {
	if PersesUITLSRequired(persesRuntimeManifest(t, false)) {
		t.Fatal("Perses TLS must not be required when the surface is deselected")
	}
	m := persesRuntimeManifest(t, true)
	m.Services.ObservabilityManagementUI = false
	if PersesUITLSRequired(m) {
		t.Fatal("Perses TLS must not be required without observability.management_ui")
	}
	m.Services.ObservabilityManagementUI = true
	if !PersesUITLSRequired(m) {
		t.Fatal("Perses TLS must be required when selected with observability.management_ui")
	}
}

// TestPersesRuntimeSmokeDockerIsolated is a real-container smoke test for the
// Perses lifecycle. It is OFF by default (no container in ordinary CI): set
// BASEHARBOR_PERSES_RUNTIME_SMOKE=1 to run it against an isolated Docker project.
// It brings the Perses project up, asserts the single container is running and
// the published loopback port is mapped, then tears the whole project down so no
// container/network/volume survives the test.
func TestPersesRuntimeSmokeDockerIsolated(t *testing.T) {
	if os.Getenv("BASEHARBOR_PERSES_RUNTIME_SMOKE") != "1" {
		t.Skip("Perses runtime smoke requires BASEHARBOR_PERSES_RUNTIME_SMOKE=1 (isolated Docker project)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatalf("resolve runtime provider: %v", err)
	}

	m := persesRuntimeManifest(t, true)
	if err := m.Validate(); err != nil {
		t.Fatalf("validate perses manifest: %v", err)
	}
	store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "perses-runtime-smoke"}
	files, err := EnsureRuntime(ctx, serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatalf("ensure Perses runtime files: %v", err)
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatalf("perses compose config invalid: %v", err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatalf("start Perses runtime: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		if err := runtime.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy Perses runtime project: %v", err)
		}
	})

	// The single Perses UI container must become running within the deadline.
	deadline := time.Now().Add(90 * time.Second)
	running := false
	hostPort := 0
	for time.Now().Before(deadline) {
		states, err := runtime.ServiceStatesProjectFilesEnv(ctx, files.Project, files.Dir, nil, files.Compose)
		if err == nil {
			for _, s := range states {
				if !strings.HasPrefix(s.Service, "perses") || s.State != "running" || len(s.Publishers) == 0 {
					continue
				}
				// The loopback management surface publishes its host port on one
				// of its publishers (the internal ports report PublishedPort 0).
				// Take the first publisher that actually maps a host port, so the
				// probe reaches the gateway rather than an internal container port.
				for _, p := range s.Publishers {
					if p.PublishedPort != 0 {
						running = true
						hostPort = p.PublishedPort
						break
					}
				}
			}
		}
		if running {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !running {
		t.Fatal("Perses UI container never became running")
	}
	if hostPort == 0 {
		t.Fatal("Perses UI container did not publish a loopback port for the plugin extraction probe")
	}

	// The reviewed acceptance requires real plugin availability at runtime:
	// Perses 0.55.1 extracts the shipped plugin archives into /etc/perses/plugins
	// and serves the loaded modules from GET /api/v1/plugins. Under a read-only
	// root filesystem without a writable extraction target the extraction fails
	// (the server starts, but the endpoint reports 0 modules). The dedicated
	// named volume at PersesPluginsContainerPath makes the path extractable;
	// the endpoint must therefore report the shipped UI plugin.
	body := fetchPersesHTTPS(t, ctx, hostPort, files.Dir)
	if !strings.Contains(body, "BarChart") && !strings.Contains(body, "Dashboard") && !strings.Contains(body, "panels") {
		t.Fatalf("Perses GET /api/v1/plugins must report the shipped UI plugin after extraction, got body:\n%s", body)
	}
}

// fetchPersesHTTPS performs an HTTPS GET to the running Perses UI over the
// loopback host port. It verifies the projected serviceaccess certificate
// against the projected CA and presents the projected mTLS client identity so
// the smoke can assert the shipped UI plugin actually loads after the
// extraction target is writable.
func fetchPersesHTTPS(t *testing.T, ctx context.Context, port int, dir string) string {
	t.Helper()
	// Authority is the DNS name "localhost", not the loopback IP. The gateway
	// is a Caddy reverse proxy that serves Perses under its canonical name;
	// addressing it by numeric IP authority returns HTTP 421 (a misdirected
	// request under HTTP/2) and, on a loopback QUIC-capable listener, an
	// application-protocol alert, because the connection is never bound to the
	// named virtual host the contract serves (docs/how-to/developer-access.md:157).
	url := fmt.Sprintf("https://localhost:%d/api/v1/plugins", port)
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build perses probe request: %v", err)
		}
		resp, err := httpsPersesClient(t, dir).Do(req)
		if err == nil {
			defer resp.Body.Close()
			data, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				lastErr = readErr
			} else if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("GET %s -> HTTP %d: %s", url, resp.StatusCode, string(data))
			} else {
				return string(data)
			}
		} else {
			lastErr = err
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("perses plugin probe never succeeded (%s): %v", url, lastErr)
	return ""
}

// httpsPersesClient returns a loopback-only HTTPS client that verifies the
// gateway certificate against the projected CA and presents the projected mTLS
// client certificate the Perses gateway requires, so the smoke can reach the
// authenticated plugin endpoint. The client key lives in the run's
// service-access pki projection under dir. The dialer routes "localhost" to
// the loopback IP so SNI still names the canonical server, not the numeric
// authority the Caddy proxy refuses.
func httpsPersesClient(t *testing.T, dir string) *http.Client {
	t.Helper()
	certDir := filepath.Join(dir, "providers", "perses-management", "gateway", "service-access", "pki")
	ca, err := os.ReadFile(filepath.Join(certDir, "ca.pem"))
	if err != nil {
		t.Fatalf("read projected mTLS CA bundle at %s: %v", certDir, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		t.Fatalf("projected mTLS CA bundle at %s is not a valid PEM certificate", certDir)
	}
	cert, err := tls.LoadX509KeyPair(
		filepath.Join(certDir, "client-cert.pem"),
		filepath.Join(certDir, "client-key.pem"),
	)
	if err != nil {
		t.Fatalf("load projected mTLS client key pair at %s: %v", certDir, err)
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			TLSClientConfig: &tls.Config{
				RootCAs:      pool,
				Certificates: []tls.Certificate{cert},
				ServerName:   "localhost",
			},
		},
	}
}

// persesRuntimeEnvironment is a package helper so tests can build a minimal
// valid runtime env for the surface contract without a container.
type persesRuntimeEnvironment = map[string]string
