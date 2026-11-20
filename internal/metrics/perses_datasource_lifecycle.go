package metrics

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// Perses datasource lifecycle seams (card 0428-083 apply-boundary call site).
//
// internal/metrics imports internal/application, so the application lifecycle
// cannot carry the static edge back — the registration below closes that gap
// without one. The lifecycle calls the registered materializer BEFORE it
// renders the compose file, with the deployment root and the deployment's
// resource project, so the datasource artifacts land in the projection the
// compose render mounts, and the render resolves its mount sources through the
// registered resolver — mounts and materializer output are the same paths by
// construction; no second path constant exists anywhere on this route.
//
// Fail-closed contract: a deselected surface removes its projection (a stale
// datasource never survives a deselect), and a selected surface whose build
// fails never leaves partial provisioning (the materializer error path removes
// the projection and propagates the error).

// PersesDatasourceContainerProvisioningPath is the in-container target of the
// provisioning mount: the folder the materialized config declares in its
// provisioning.folders list (the same constant NewPersesDatasourceConfig
// uses — one source of truth for the path).
const PersesDatasourceContainerProvisioningPath = persesDatasourceProvisioningContainerFolder

func init() {
	// The lifecycle clears datasource artifacts a previous (invalid or failed)
	// build left in the projection root before starting Perses (stale files
	// must never be loaded). The cleanup removes ONLY those artifacts; the
	// sibling TLS material and persistence marker of the surface stay
	// untouched.
	application.RegisterPersesDatasourceArtifactsRemover(datasourceArtifactsCleaner)
}

// datasourceArtifactsCleaner removes the datasource artifacts (config file and
// provisioning folder) under a datasource projection root, leaving every
// other provider file (TLS material, persistence marker) untouched.
func datasourceArtifactsCleaner(projectionRoot string) error {
	if err := os.Remove(filepath.Join(projectionRoot, persesDatasourceConfigFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove perses datasource config: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(projectionRoot, persesDatasourceProvisioningDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove perses datasource provisioning: %w", err)
	}
	return nil
}

// datasourceLifecycleRoots maps an application runtime dir (the
// RuntimeFiles.Dir the compose render uses — the ONE layout formula on this
// route is application.RuntimeFilesFor, internal/application/
// runtime_postgres.go) to its datasource projection root: the
// providers/management-ui/perses sibling of the runtime dir. The render mounts
// relative to its own context dir (the same runtime dir), so mounts and
// materializer output are the same paths by construction on EVERY layout —
// local store and target-scoped alike — and no layout formula is duplicated
// here (0428-083 F1).
func datasourceLifecycleRoots(runtimeDir string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(runtimeDir)), filepath.FromSlash(application.PersesDatasourceRelativeRoot))
}

// datasourceDataRootOf recovers the Target data root an application runtime
// dir was laid out under, from the two runtime layouts of this Core:
// <dataRoot>/<app>/runtime (local store) and
// <dataRoot>/applications/<resourceProject>/runtime (target-scoped).
func datasourceDataRootOf(runtimeDir string) string {
	dir := filepath.Clean(runtimeDir)
	parent := filepath.Dir(dir)
	if filepath.Base(filepath.Dir(parent)) == "applications" {
		return filepath.Dir(parent)
	}
	return filepath.Dir(parent)
}

// RegisterPersesDatasourceLifecycle registers the datasource materializer,
// mount-resolver and removal seams with the application lifecycle. Every
// consumer of this package (including the test builds of dependent packages)
// pulls the registration through its import of internal/metrics: init order
// guarantees all imported packages are initialized before any apply-boundary
// call.
func RegisterPersesDatasourceLifecycle() {
	application.RegisterPersesDatasourceMaterializer(materializePersesDatasourceForLifecycle)
	application.RegisterPersesDatasourceMountResolver(persesDatasourceLifecycleMounts)
	application.RegisterPersesDatasourceRemover(removePersesDatasourceForLifecycle)
}

func init() {
	RegisterPersesDatasourceLifecycle()
}

// materializePersesDatasourceForLifecycle is the apply-boundary entry the
// lifecycle seam calls: (manifest, application runtime dir, namespace). The
// runtime dir is the RuntimeFiles.Dir the lifecycle computed from
// application.RuntimeFilesFor (the one layout formula of this Core), so the
// datasource projection root is its providers/management-ui/perses sibling —
// the SAME root the compose render (whose context dir is that runtime dir)
// mounts its datasource pair from. The build resolves its provider state
// against the recovered data root, so the endpoint and trust material are
// the ones the deployment actually runs.
func materializePersesDatasourceForLifecycle(m application.Manifest, runtimeDir, namespace string) error {
	root := datasourceLifecycleRoots(runtimeDir)
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return removePersesDatasourceProjection(root)
	}
	_, _, _, err := materializePersesDatasourceAtRoot(m, root, datasourceDataRootOf(runtimeDir), namespace)
	return err
}

// removePersesDatasourceForLifecycle tears down the projection of a deselected
// surface (the lifecycle never calls it for a selected one).
func removePersesDatasourceForLifecycle(m application.Manifest, runtimeDir, namespace string) error {
	if m.Services.Perses {
		return nil
	}
	return removePersesDatasourceProjection(datasourceLifecycleRoots(runtimeDir))
}

// persesDatasourceLifecycleMounts resolves the read-only datasource mounts for
// an application runtime projection dir (the compose context dir): the
// datasource projection is the providers/management-ui/perses sibling of the
// runtime dir (application.PersesDatasourceRuntimeRoot — the same derivation
// the materializer uses, F1), so the mount sources are always its ../-relative
// paths. The third mount is the public trust bundle the provisioned
// GlobalSecret's caFile names, emitted only when it exists.
// Fail-closed when the artifacts do not exist.
func persesDatasourceLifecycleMounts(runtimeProjectionDir, resourceProject string) (configMount, provisioningMount, trustBundleMount string, present bool) {
	root := persesDatasourceProjectionRootForRuntime(runtimeProjectionDir)
	configFile, provisioningFolder, err := persesDatasourceArtifactPathsAt(root)
	if err != nil {
		return "", "", "", false
	}
	if st, statErr := os.Stat(configFile); statErr != nil || !st.Mode().IsRegular() {
		return "", "", "", false
	}
	entries, readErr := os.ReadDir(provisioningFolder)
	if readErr != nil {
		return "", "", "", false
	}
	provisioned := false
	for _, entry := range entries {
		if !entry.IsDir() {
			provisioned = true
		}
	}
	if !provisioned {
		return "", "", "", false
	}
	rel := func(path string) string {
		p, relErr := filepath.Rel(runtimeProjectionDir, filepath.Clean(path))
		if relErr != nil || p == "." {
			return ""
		}
		return "./" + filepath.ToSlash(p)
	}
	configRelative, provisioningRelative := rel(configFile), rel(provisioningFolder)
	if configRelative == "" || provisioningRelative == "" {
		return "", "", "", false
	}
	trustBundle := ""
	if st, statErr := os.Stat(PersesDatasourceTrustBundlePath(root)); statErr == nil && st.Mode().IsRegular() {
		bundleRelative := rel(PersesDatasourceTrustBundlePath(root))
		if bundleRelative == "" {
			return "", "", "", false
		}
		trustBundle = bundleRelative + ":" + application.PersesDatasourceContainerTrustBundlePath + ":ro"
	}
	return configRelative + ":" + PersesDatasourceContainerConfigPath + ":ro",
		provisioningRelative + ":" + PersesDatasourceContainerProvisioningPath + ":ro",
		trustBundle, true
}

// persesDatasourceProjectionRootForRuntime derives the datasource projection
// root of an application deployment from its runtime projection dir — the
// providers/management-ui/perses sibling, through the single application-side
// formula (0428-083 F1).
func persesDatasourceProjectionRootForRuntime(runtimeProjectionDir string) string {
	return application.PersesDatasourceRuntimeRoot(runtimeProjectionDir)
}

// persesDatasourceArtifactPathsAt resolves the datasource projection paths
// (config file + provisioning folder) under a projection root.
func persesDatasourceArtifactPathsAt(projectionRoot string) (configFile, provisioningFolder string, err error) {
	if strings.TrimSpace(projectionRoot) == "" {
		return "", "", errors.New("perses datasource projection root is required")
	}
	root := filepath.Clean(projectionRoot)
	return filepath.Join(root, persesDatasourceConfigFile), filepath.Join(root, persesDatasourceProvisioningDir), nil
}

// persesDatasourceProjectionForRoot is the projection root of a deployment
// root/resource project pair — the same formula the manifest-bound
// persesDatasourceProjectionRoot implements.
func persesDatasourceProjectionForRoot(deploymentRoot, resourceProject string) string {
	return filepath.Join(filepath.Clean(deploymentRoot), "applications", resourceProject, "providers", "management-ui", "perses")
}

// materializePersesDatasourceWithProject provisions the datasource under an
// explicit deployment root and resource project (the lifecycle seam: the
// project is the deployment truth, never re-derived from the manifest).
func materializePersesDatasourceWithProject(m application.Manifest, deploymentRoot, namespace, resourceProject string) (PersesPrometheusDatasourceResult, string, string, error) {
	res, err := BuildPersesPrometheusDatasourceAt(m, deploymentRoot, namespace)
	root := persesDatasourceProjectionForRoot(deploymentRoot, resourceProject)
	if err != nil {
		// Fail-closed teardown: no datasource may remain provisioned when the
		// build (selection, pin, authorization, sources, endpoint) fails.
		if rmErr := removePersesDatasourceProjection(root); rmErr != nil {
			return PersesPrometheusDatasourceResult{}, "", "", errors.Join(err, rmErr)
		}
		return PersesPrometheusDatasourceResult{}, "", "", err
	}
	resource, err := NewPersesGlobalDatasourceResource(res)
	if err != nil {
		if rmErr := removePersesDatasourceProjection(root); rmErr != nil {
			return PersesPrometheusDatasourceResult{}, "", "", errors.Join(err, rmErr)
		}
		return PersesPrometheusDatasourceResult{}, "", "", err
	}
	configData, err := PersesDatasourceConfigJSON(NewPersesDatasourceConfig())
	if err != nil {
		return res, "", "", fmt.Errorf("perses datasource config: %w", err)
	}
	resourceData, err := PersesGlobalDatasourceResourceJSON(resource)
	if err != nil {
		return res, "", "", fmt.Errorf("perses datasource provisioning resource: %w", err)
	}
	configPath := filepath.Join(root, persesDatasourceConfigFile)
	provisioningDir := filepath.Join(root, persesDatasourceProvisioningDir)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return res, "", "", fmt.Errorf("perses datasource projection directory: %w", err)
	}
	if err := writePersesDatasourceFile(configPath, configData); err != nil {
		return res, "", "", fmt.Errorf("write perses datasource config: %w", err)
	}
	if err := os.MkdirAll(provisioningDir, 0o755); err != nil {
		return res, "", "", fmt.Errorf("perses datasource provisioning folder: %w", err)
	}
	// Clear stale resource files of a previous materialization first: the
	// provisioning loader loads EVERY file in the folder, so a leftover
	// datasource (or trust-anchor secret) from an earlier apply under a
	// DIFFERENT endpoint must never ride into this one.
	if err := clearPersesDatasourceProvisioningFolder(provisioningDir); err != nil {
		return res, "", "", err
	}
	if err := writePersesDatasourceFile(filepath.Join(provisioningDir, persesDatasourceProvisioningFile), resourceData); err != nil {
		return res, "", "", fmt.Errorf("write perses datasource provisioning resource: %w", err)
	}
	return res, configPath, filepath.Join(provisioningDir, persesDatasourceProvisioningFile), nil
}

// removePersesDatasourceProjectionAt removes the datasource projection of an
// explicit deployment root/resource project (the lifecycle seam of the
// deselected surface; ownership-safe like the manifest-bound removal).
func removePersesDatasourceProjectionAt(deploymentRoot, resourceProject string) error {
	return removePersesDatasourceProjection(persesDatasourceProjectionForRoot(deploymentRoot, resourceProject))
}

// clearPersesDatasourceProvisioningFolder removes the regular files of the
// datasource provisioning folder (never directories, never the folder itself),
// so the materializer's write is the only provisioning resource left.
func clearPersesDatasourceProvisioningFolder(provisioningDir string) error {
	entries, err := os.ReadDir(provisioningDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read perses datasource provisioning folder: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if rmErr := os.Remove(filepath.Join(provisioningDir, entry.Name())); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return fmt.Errorf("clear stale perses datasource provisioning file: %w", rmErr)
		}
	}
	return nil
}
