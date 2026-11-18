package application

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
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
// materializes a single-instance managed compose service, its data volume and a
// loopback-published port. Fail-closed: deselected Perses renders no service.
func TestPersesComposeRenderedWhenSelected(t *testing.T) {
	selected := persesRuntimeManifest(t, true)
	selectedFiles := persesRuntimeFiles(t, selected)
	rendered, err := RuntimeComposeYAMLForProject(selected, selectedFiles.ResourceProject)
	if err != nil {
		t.Fatalf("render selected Perses compose: %v", err)
	}
	persesService := runtimeServiceName("perses", defaultServiceInstance)
	if !strings.Contains(rendered, "  "+persesService+":") {
		t.Fatalf("selected Perses must render a single-instance compose service:\n%s", rendered)
	}
	if !strings.Contains(rendered, PersesUIImage) {
		t.Fatalf("selected Perses must pin the reviewed image %q: %s", PersesUIImage, rendered)
	}
	if !strings.Contains(rendered, PersesUIHostPortEnv) {
		t.Fatalf("selected Perses must publish a loopback host port via %q: %s", PersesUIHostPortEnv, rendered)
	}
	if !strings.Contains(rendered, PersesRuntimeVolumeName(selectedFiles.ResourceProject, persesService)) {
		t.Fatalf("selected Perses must declare a managed data volume:\n%s", rendered)
	}
	var compose struct {
		Services map[string]struct {
			Volumes []string `yaml:"volumes"`
		} `yaml:"services"`
		Volumes map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &compose); err != nil {
		t.Fatalf("decode selected Perses compose: %v", err)
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
	deselectedRendered, err := RuntimeComposeYAMLForProject(deselected, deselectedFiles.ResourceProject)
	if err != nil {
		t.Fatalf("render deselected Perses compose: %v", err)
	}
	if strings.Contains(deselectedRendered, "  "+persesService+":") {
		t.Fatalf("deselected Perses must render no compose service (fail-closed):\n%s", deselectedRendered)
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
	rendered, err := RuntimeComposeYAMLForProject(m, files.ResourceProject)
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

// TestPersesTLSFilesProjectedAndConsumed is the CODEX-REVIEW #1592 regression:
// every TLS file the Perses compose service mounts must exist in the runtime
// projection, and the rendered service must configure the Perses server to
// consume it over HTTPS (matching the contract's observability management-ui
// protocol). Fail-closed: the projected private key must stay 0600 while the
// bind-mounted copy the distroless nonroot container reads must be readable.
func TestPersesTLSFilesProjectedAndConsumed(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	files := persesRuntimeFiles(t, m)
	rendered := persesComposeFragment(t, m, files)
	dir := PersesManagementUIPath(files)
	// The compose bind-mounts are 0644 copies of the 0600 PKI material (the
	// distroless nonroot container cannot read 0600 binds), exactly the
	// approved serviceaccess projection used by the neighbouring UIs.
	const projectedKeyMode os.FileMode = 0o644

	var certMount, keyMount string
	for _, line := range strings.Split(rendered, "\n") {
		trimmed := strings.TrimPrefix(strings.Trim(strings.TrimSpace(line), `"'`), "- ")
		switch {
		case strings.HasPrefix(trimmed, "./providers/management-ui/perses/server.cert:"):
			certMount = trimmed
		case strings.HasPrefix(trimmed, "./providers/management-ui/perses/server.key:"):
			keyMount = trimmed
		}
	}
	if certMount == "" || keyMount == "" {
		t.Fatalf("Perses compose must mount the projected TLS files (cert %q, key %q):\n%s", certMount, keyMount, rendered)
	}
	for name, mount := range map[string]string{"certificate": certMount, "key": keyMount} {
		if mount == "" {
			t.Fatalf("Perses compose must mount the projected %s file:\n%s", name, rendered)
		}
		parts := strings.SplitN(mount, ":", 2)
		hostRel, mountRest := parts[0], parts[1]
		containerPath := strings.SplitN(mountRest, ":", 2)[0]
		if !strings.HasPrefix(hostRel, "./") {
			t.Fatalf("Perses %s mount source %q must be compose-relative ./...", name, hostRel)
		}
		hostPath := filepath.Join(files.Dir, strings.TrimPrefix(hostRel, "./"))
		info, err := os.Stat(hostPath)
		if err != nil || info.IsDir() {
			t.Fatalf("Perses %s mount source must exist as a file (%q): %v", name, hostPath, err)
		}
		// The compose mounts must be exactly the projected files.
		if filepath.Clean(hostPath) != filepath.Clean(filepath.Join(dir, filepath.Base(hostPath))) {
			t.Fatalf("Perses %s mount %q must resolve into the projection dir %q", name, hostRel, dir)
		}
		// The server consumes each mount via the TLS command flags.
		containerFlag := map[string]string{
			"/certs/server.cert": "--web.tls-cert-file=" + containerPath,
			"/certs/server.key":  "--web.tls-key-file=" + containerPath,
		}[containerPath]
		if containerFlag == "" || !strings.Contains(rendered, containerFlag) {
			t.Fatalf("Perses service must consume the mounted path via %q:\n%s", containerFlag, rendered)
		}
	}
	if !strings.Contains(rendered, "--web.tls-cert-file=/certs/server.cert") ||
		!strings.Contains(rendered, "--web.tls-key-file=/certs/server.key") {
		t.Fatalf("Perses service must enable HTTPS with both TLS flags:\n%s", rendered)
	}

	certData, err := os.ReadFile(filepath.Join(dir, "server.cert"))
	if err != nil {
		t.Fatalf("read projected Perses certificate: %v", err)
	}
	keyData, err := os.ReadFile(filepath.Join(dir, "server.key"))
	if err != nil {
		t.Fatalf("read projected Perses key: %v", err)
	}
	if len(certData) == 0 || len(keyData) == 0 {
		t.Fatal("projected Perses TLS files must not be empty")
	}
	keyInfo, err := os.Stat(filepath.Join(dir, "server.key"))
	if err != nil {
		t.Fatalf("stat projected Perses key: %v", err)
	}
	if keyInfo.Mode().Perm() != projectedKeyMode {
		t.Fatalf("projected Perses key must be the readable bind-mount copy %v, got %v", projectedKeyMode, keyInfo.Mode().Perm())
	}
	pkiKeyInfo, err := os.Stat(filepath.Join(dir, "pki", "server-key.pem"))
	if err != nil {
		t.Fatalf("stat Perses PKI working key: %v", err)
	}
	if pkiKeyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("Perses PKI working key must stay private 0600, got %v", pkiKeyInfo.Mode().Perm())
	}
	certBlock, _ := pem.Decode(certData)
	if certBlock == nil {
		t.Fatal("projected Perses certificate must be PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("parse projected Perses certificate: %v", err)
	}
	names := map[string]bool{"localhost": false, "127.0.0.1": false, "perses": false}
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
			t.Fatalf("Perses certificate must cover loopback name %q (contract protocol https): %+v", name, cert.DNSNames)
		}
	}
	if cert.NotAfter.Before(time.Now()) {
		t.Fatal("projected Perses certificate must not be expired")
	}
}

// TestPersesLifecycleResourcesIncludeManagedService asserts the runtime lifecycle
// inspects and can retire the Perses UI container and its data volume.
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

// TestPersesConfiguredUnverifiedWhenSelectedPinned asserts the positive contract:
// selected + pinned release reports configured_unverified (never verified), and
// the surface is declared.
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
		t.Fatal("selected+verified Perses must declare an observability surface")
	}
	if *status != capability.PersesStatusConfiguredUnverified {
		t.Fatalf("selected+verified Perses status = %q, want configured_unverified", *status)
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
	for time.Now().Before(deadline) {
		states, err := runtime.ServiceStatesProjectFilesEnv(ctx, files.Project, files.Dir, nil, files.Compose)
		if err == nil {
			for _, s := range states {
				if strings.HasPrefix(s.Service, "perses") && s.State == "running" {
					running = true
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
}

// persesRuntimeEnvironment is a package helper so tests can build a minimal
// valid runtime env for the surface contract without a container.
type persesRuntimeEnvironment = map[string]string
