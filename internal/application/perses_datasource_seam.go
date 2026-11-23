package application

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// already uses): the datasource projection root of an application deployment
// is the providers/management-ui/perses SIBLING of its runtime dir — for both
// runtime layouts of this Core (local store <dataRoot>/<app>/runtime and
// target-scoped <dataRoot>/applications/<resourceProject>/runtime the app dir
// is the parent of the runtime dir). PersesDatasourceRuntimeRoot below is the
// only derivation; the lifecycle, the render and the materializer agree
// through it (0428-083 F1). The lifecycle resolves mount sources through the
// seam resolver, so this constant exists exactly once on the application side.
const PersesDatasourceRelativeRoot = "providers/management-ui/perses"

// PersesDatasourceRuntimeRoot is the application-side single source of truth
// for the datasource projection root: the providers/management-ui/perses
// sibling of an application runtime dir (RuntimeFiles.Dir). The lifecycle
// materializer seam, the compose render's mount resolver and the no-seam
// fallback all address the datasource through THIS function, so writer and
// reader can never disagree about the layout on either store layout
// (0428-083 F1).
func PersesDatasourceRuntimeRoot(runtimeDir string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(runtimeDir)), filepath.FromSlash(PersesDatasourceRelativeRoot))
}

// persesDatasourceLifecycleMounts is the lifecycle seam to the datasource
// mount resolver of the metrics package (metrics imports application; a static
// import in the other direction would be a cycle). The metrics package
// registers its resolver at package init, so every binary that includes the
// datasource (the CLI) always has the seam installed before any lifecycle
// call (Go init order); the Perses compose render derives the read-only
// datasource mounts from the materializer's own paths through it — never from
// a second constant here.
var persesDatasourceLifecycleMounts atomic.Pointer[persesDatasourceMountFunc]

type persesDatasourceMountFunc func(runtimeProjectionDir, resourceProject string) (configMount, provisioningMount, trustBundleMount string, present bool)

// RegisterPersesDatasourceMountResolver registers the lifecycle-visible mount
// resolver of the metrics package (card 0428-083).
func RegisterPersesDatasourceMountResolver(fn func(runtimeProjectionDir, resourceProject string) (string, string, string, bool)) {
	resolver := persesDatasourceMountFunc(fn)
	persesDatasourceLifecycleMounts.Store(&resolver)
}

// ProviderGatewayHostMapping is the compose extra_hosts entry that makes the
// Prometheus gateway service name resolvable from an APPLICATION container:
// the host-gateway address, where the provider gateway publishes its
// management loopback. The service name is the Prometheus gateway alias
// (metrics.ProviderNetworkEndpointHost); the rendering side of this card uses
// it, the metrics side owns the name.
func ProviderGatewayHostMapping() string {
	return ProviderGatewayServiceName + ":" + ProviderGatewayHostGatewayAddress
}

// ProviderGatewayServiceName and ProviderGatewayHostGatewayAddress are the two
// halves of the mapping; they mirror the metrics-side constants
// (ProviderNetworkEndpointHost / host-gateway) so the application render
// carries no URL formula of its own.
const (
	ProviderGatewayServiceName        = "prometheus"
	ProviderGatewayHostGatewayAddress = "host-gateway"
)

// datasourceLifecycleRoots maps an application runtime dir to its datasource
// projection root through the single application-side derivation
// (PersesDatasourceRuntimeRoot). The former two-argument
// (deploymentRoot, resourceProject) formula re-derived a layout the lifecycle
// already knew; the reader and the writer now agree through the runtime dir
// alone (0428-083 F1).
func datasourceLifecycleRoots(runtimeDir string) string {
	return PersesDatasourceRuntimeRoot(runtimeDir)
}

// datasourceArtifactsPresent reports whether a datasource projection root
// carries the materialized config file and provisioning resource.
func datasourceArtifactsPresent(projectionRoot string) bool {
	configFile := filepath.Join(projectionRoot, "config.yaml")
	provisioningFolder := filepath.Join(projectionRoot, "provisioning")
	if st, err := os.Stat(configFile); err != nil || !st.Mode().IsRegular() {
		return false
	}
	entries, err := os.ReadDir(provisioningFolder)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return true
		}
	}
	return false
}

// PersesDatasourceContainerConfigPath and PersesDatasourceContainerProvisioningPath
// are the in-container targets of the datasource mounts: the --config file the
// Perses command consumes, and the folder the materialized config declares in
// provisioning.folders.
const (
	PersesDatasourceContainerConfigPath       = "/etc/perses/config.yaml"
	PersesDatasourceContainerProvisioningPath = "/etc/perses/provisioning"
)

// PersesDatasourceContainerTrustBundlePath is the in-container target of the
// public trust bundle mount (the file the provisioned GlobalSecret's caFile
// names). The metrics side owns the constant's value; the application side
// mirrors it for the render.
const PersesDatasourceContainerTrustBundlePath = "/etc/perses/datasource-trust/baseharbor-runtime-ca.pem"

// persesDatasourceMounts resolves the datasource mounts (config file,
// provisioning folder, public trust bundle) for a compose context dir through
// the registered resolver (absent resolver = this package's layout
// convention). The resourceProject is the deployment's runtime resource
// project; the datasource projection root is the providers/management-ui/
// perses sibling of the context dir (F1).
func persesDatasourceMounts(runtimeProjectionDir, resourceProject string) (configMount, provisioningMount, trustBundleMount string, present bool) {
	resolver := persesDatasourceLifecycleMounts.Load()
	if resolver == nil {
		return localPersesDatasourceMounts(runtimeProjectionDir, resourceProject)
	}
	if strings.TrimSpace(runtimeProjectionDir) == "" || strings.TrimSpace(resourceProject) == "" {
		return "", "", "", false
	}
	return (*resolver)(runtimeProjectionDir, resourceProject)
}

// localPersesDatasourceMounts resolves the datasource mounts from the layout
// convention this package owns, for builds without the metrics-side seam
// (including this package's own tests). The mount sources are the projection
// paths relative to the compose context dir; absent artifacts mean no mounts.
func localPersesDatasourceMounts(runtimeProjectionDir, resourceProject string) (configMount, provisioningMount, trustBundleMount string, present bool) {
	if strings.TrimSpace(runtimeProjectionDir) == "" || strings.TrimSpace(resourceProject) == "" {
		return "", "", "", false
	}
	_, projectionRoot := datasourceMountRoots(runtimeProjectionDir, resourceProject)
	if !datasourceArtifactsPresent(projectionRoot) {
		return "", "", "", false
	}
	configRelative, relErr := filepath.Rel(runtimeProjectionDir, filepath.Join(projectionRoot, "config.yaml"))
	provisioningRelative, provRelErr := filepath.Rel(runtimeProjectionDir, filepath.Join(projectionRoot, "provisioning"))
	if relErr != nil || provRelErr != nil {
		return "", "", "", false
	}
	trustBundleMount = ""
	if _, err := os.Stat(filepath.Join(projectionRoot, "root", "ca.pem")); err == nil {
		bundleRelative, err := filepath.Rel(runtimeProjectionDir, filepath.Join(projectionRoot, "root", "ca.pem"))
		if err != nil {
			return "", "", "", false
		}
		trustBundleMount = "./" + filepath.ToSlash(bundleRelative) + ":" + PersesDatasourceContainerTrustBundlePath + ":ro"
	}
	return "./" + filepath.ToSlash(configRelative) + ":" + PersesDatasourceContainerConfigPath + ":ro",
		"./" + filepath.ToSlash(provisioningRelative) + ":" + PersesDatasourceContainerProvisioningPath + ":ro",
		trustBundleMount, true
}

// datasourceMountRoots maps an application runtime projection dir to the
// datasource projection root of this deployment. The root is the sibling of
// the runtime dir (PersesDatasourceRuntimeRoot) on every layout; the second
// return is the recovered data root (for consumers that address the same tree
// from the top).
func datasourceMountRoots(runtimeProjectionDir, resourceProject string) (dataRoot, projectionRoot string) {
	dir := filepath.Clean(runtimeProjectionDir)
	parent := filepath.Dir(dir)
	if filepath.Base(filepath.Dir(parent)) == "applications" {
		dataRoot = filepath.Dir(parent) // target-scoped layout
	} else {
		dataRoot = filepath.Dir(parent) // local store layout
	}
	return dataRoot, PersesDatasourceRuntimeRoot(runtimeProjectionDir)
}

// persesDatasourceArtifactsPresentForRender reports whether the datasource
// mounts of a runtime projection are backed by materialized artifacts (the
// same seam the render resolves its mounts through).
func persesDatasourceArtifactsPresentForRender(files RuntimeFiles) bool {
	_, projectionRoot := datasourceMountRoots(files.Dir, files.ResourceProject)
	return datasourceArtifactsPresent(projectionRoot)
}

// RemovePersesDatasourceProjection tears down the datasource projection of a
// deselected Perses surface through the metrics-side removal seam (same path
// formula as the materializer). Fail-closed: a deselected surface must not
// leave a stale datasource that a re-selected Perses would load; without a
// registered seam the lifecycle reports the error instead of pretending the
// teardown happened.
func RemovePersesDatasourceProjection(files RuntimeFiles, m Manifest) error {
	if m.Services.Perses {
		return nil
	}
	remove := persesDatasourceRemover.Load()
	if remove == nil {
		// No seam registered in this binary: the lifecycle still removes the
		// datasource projection layout it knows (config file + provisioning
		// folder under the deployment's datasource root). A deselected
		// surface must not keep a stale datasource a re-selected Perses would
		// load.
		_, projectionRoot := datasourceMountRoots(files.Dir, files.ResourceProject)
		if err := os.Remove(filepath.Join(projectionRoot, "config.yaml")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove deselected perses datasource config: %w", err)
		}
		if err := os.RemoveAll(filepath.Join(projectionRoot, "provisioning")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove deselected perses datasource provisioning: %w", err)
		}
		return nil
	}
	return (*remove)(m, files.Dir, files.Namespace)
}

// persesDatasourceRemover is the lifecycle seam to the metrics-side datasource
// removal (registered together with the mount resolver). The root argument is
// the application runtime dir (see RegisterPersesDatasourceMaterializer).
var persesDatasourceRemover atomic.Pointer[persesDatasourceRemoveFunc]

type persesDatasourceRemoveFunc func(m Manifest, runtimeDir, namespace string) error

// RegisterPersesDatasourceRemover registers the lifecycle-visible removal
// entry of the metrics package (card 0428-083).
func RegisterPersesDatasourceRemover(fn func(Manifest, string, string) error) {
	remover := persesDatasourceRemoveFunc(fn)
	persesDatasourceRemover.Store(&remover)
}

// clearStalePersesDatasourceArtifacts removes datasource artifacts left by an
// earlier build/validation so a Perses start never loads a stale datasource.
// It runs before the apply-boundary materializer (which validates and
// re-provisions the datasource) and is ownership-safe: only the datasource
// config file and provisioning folder of this deployment's datasource root.
func clearStalePersesDatasourceArtifacts(files RuntimeFiles, m Manifest) error {
	_, projectionRoot := datasourceMountRoots(files.Dir, files.ResourceProject)
	remove := persesDatasourceArtifactsRemover.Load()
	if remove == nil {
		return removeDatasourceArtifactsLocal(projectionRoot)
	}
	return (*remove)(projectionRoot)
}

// removeDatasourceArtifactsLocal removes the datasource config file and
// provisioning folder of a projection root without the metrics-side seam.
func removeDatasourceArtifactsLocal(projectionRoot string) error {
	if err := os.Remove(filepath.Join(projectionRoot, "config.yaml")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove perses datasource config: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(projectionRoot, "provisioning")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove perses datasource provisioning: %w", err)
	}
	return nil
}

// persesDatasourceArtifactsRemover is the lifecycle seam to the metrics-side
// datasource artifact cleanup (registered by the metrics package).
var persesDatasourceArtifactsRemover atomic.Pointer[persesDatasourceArtifactsRemoveFunc]

type persesDatasourceArtifactsRemoveFunc func(projectionRoot string) error

// RegisterPersesDatasourceArtifactsRemover registers the lifecycle-visible
// datasource artifact cleanup of the metrics package (card 0428-083).
func RegisterPersesDatasourceArtifactsRemover(fn func(projectionRoot string) error) {
	remove := persesDatasourceArtifactsRemoveFunc(fn)
	persesDatasourceArtifactsRemover.Store(&remove)
}

// EnsurePersesManagementUI projects the optional Perses provider files (TLS +
// persistence marker) under providers/management-ui/perses/runtime. It is
// fail-closed: it is a no-op unless Perses is selected with observability
// management_ui, so an unselected surface leaves no files behind. For a
// selected surface it also clears any datasource artifacts a previous
// (invalid or failed) build left in the projection root: the datasource
// content is owned by the apply-boundary materializer, and stale files must
// never reach a Perses start — the mount render only names paths that exist
// after this point, and the materializer (or the next apply) provisions them.
