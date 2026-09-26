package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

func StopProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return StopProviderAt(ctx, runtime, dataDir, "", m)
}

func StopProviderAt(ctx context.Context, runtime Runtime, dataDir, namespace string, m application.Manifest) error {
	placement, found, err := RegisteredPlacementForAt(dataDir, namespace, m)
	if err != nil {
		return err
	}
	if !found || placement.Scope != capability.ScopeApplication {
		return nil
	}
	files, err := existingProviderFilesForPlacement(placement)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return runtime.DownProject(ctx, placement.Project, files.Compose, files.Env)
}

func DestroyProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return DestroyProviderAt(ctx, runtime, dataDir, "", m)
}

func DestroyProviderAt(ctx context.Context, runtime Runtime, dataDir, namespace string, m application.Manifest) error {
	placement, found, err := RegisteredPlacementForAt(dataDir, namespace, m)
	if err != nil {
		return err
	}
	if !found || placement.Scope == capability.ScopeExternal {
		return nil
	}
	files, err := existingProviderFilesForPlacement(placement)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func SharedProbeManifest() application.Manifest {
	return application.Manifest{Name: "shared-probe", Environment: "dev"}
}

type SharedProviderInstance struct {
	Placement Placement
	Files     ProviderFiles
}

func ExistingSharedProviderInstances() ([]SharedProviderInstance, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return nil, err
	}
	return ExistingSharedProviderInstancesAt(dataDir, "")
}

func ExistingSharedProviderInstancesAt(dataDir, namespace string) ([]SharedProviderInstance, error) {
	root := filepath.Join(filepath.Clean(dataDir), "providers", "prometheus", "shared")
	namespace = strings.TrimSpace(strings.ReplaceAll(namespace, ".", "-"))
	baseProject := bhruntime.SharedProjectName(namespace)
	baseVolume := "baseharbor-prometheus-data"
	if namespace != "" {
		baseVolume = "baseharbor-prometheus-data-" + namespace
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var instances []SharedProviderInstance
	if files, err := providerFilesAt(root); err == nil {
		instances = append(instances, SharedProviderInstance{
			Placement: Placement{
				Scope:   capability.ScopeShared,
				Project: baseProject,
				Volume:  baseVolume,
				Dir:     root,
			},
			Files: files,
		})
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		token := entry.Name()
		dir := filepath.Join(root, token)
		files, err := providerFilesAt(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		instances = append(instances, SharedProviderInstance{
			Placement: Placement{
				Scope:   capability.ScopeShared,
				Project: baseProject + "-" + token,
				Volume:  baseVolume + "-" + token,
				Dir:     dir,
			},
			Files: files,
		})
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Placement.Project < instances[j].Placement.Project
	})
	return instances, nil
}

func providerFilesAt(dir string) (ProviderFiles, error) {
	files := ProviderFiles{
		Dir:                 dir,
		Compose:             filepath.Join(dir, "compose.yaml"),
		Env:                 filepath.Join(dir, "runtime.env"),
		Config:              filepath.Join(dir, "prometheus.yml"),
		TargetsDir:          filepath.Join(dir, "targets"),
		ProviderSecurityDir: filepath.Join(dir, "provider-security"),
		Registrations:       filepath.Join(dir, "registrations.json"),
		RuntimeCA:           filepath.Join(dir, "baseharbor-runtime-ca.pem"),
	}
	for _, path := range []string{files.Compose, files.Env, files.Config, files.TargetsDir} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroyAllSharedProviders(ctx context.Context, runtime Runtime) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return DestroyAllSharedProvidersAt(ctx, runtime, dataDir, "")
}

func DestroyAllSharedProvidersAt(ctx context.Context, runtime Runtime, dataDir, namespace string) error {
	instances, err := ExistingSharedProviderInstancesAt(dataDir, namespace)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		if err := runtime.DestroyProject(ctx, instance.Placement.Project, instance.Files.Compose, instance.Files.Env); err != nil {
			return fmt.Errorf("destroy shared Prometheus project %s: %w", instance.Placement.Project, err)
		}
	}
	if len(instances) == 0 {
		return nil
	}
	return os.RemoveAll(filepath.Join(filepath.Clean(dataDir), "providers", "prometheus", "shared"))
}

func ExistingSharedProviderFiles() (ProviderFiles, error) {
	m := SharedProbeManifest()
	placement, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if placement.Scope != capability.ScopeShared {
		return ProviderFiles{}, os.ErrNotExist
	}
	dir := placement.Dir
	files := ProviderFiles{
		Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"),
		Config: filepath.Join(dir, "prometheus.yml"), TargetsDir: filepath.Join(dir, "targets"),
		ProviderSecurityDir: filepath.Join(dir, "provider-security"),
	}
	files.RuntimeCA = filepath.Join(dir, "baseharbor-runtime-ca.pem")
	for _, path := range []string{files.Compose, files.Env, files.Config, files.TargetsDir} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroySharedProvider(ctx context.Context, runtime Runtime) error {
	m := SharedProbeManifest()
	return DestroyProvider(ctx, runtime, m)
}

func registrationFor(m application.Manifest) sourceRegistration {
	return registrationForAt(m, "")
}

func registrationForAt(m application.Manifest, namespace string) sourceRegistration {
	registration := sourceRegistration{
		Application: m.Name,
		Environment: m.Environment,
		Network:     application.MetricsProviderNetworkNameForNamespace(m, namespace),
	}
	if application.HasRuntimeMetricsPermissions(m) {
		registration.RuntimeVolume = application.MetricsRuntimeTargetVolumeNameForNamespace(m, namespace)
	}
	return registration
}

func readRegistrations(path string) ([]sourceRegistration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var registrations []sourceRegistration
	if err := json.Unmarshal(data, &registrations); err != nil {
		return nil, errors.New("Prometheus shared registration state is invalid")
	}
	return registrations, nil
}

func reconcileSharedRegistration(path string, m application.Manifest, present bool) ([]sourceRegistration, error) {
	return reconcileSharedRegistrationAt(path, m, "", present)
}

func reconcileSharedRegistrationAt(path string, m application.Manifest, namespace string, present bool) ([]sourceRegistration, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return nil, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	var registrations []sourceRegistration
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &registrations); err != nil {
			return nil, errors.New("Prometheus shared registration state is invalid")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	filtered := registrations[:0]
	for _, registration := range registrations {
		if registration.Application == m.Name && registration.Environment == m.Environment {
			continue
		}
		filtered = append(filtered, registration)
	}
	registrations = filtered
	if present {
		registrations = append(registrations, registrationForAt(m, namespace))
	}
	sort.Slice(registrations, func(i, j int) bool {
		if registrations[i].Application != registrations[j].Application {
			return registrations[i].Application < registrations[j].Application
		}
		return registrations[i].Environment < registrations[j].Environment
	})
	data, err := json.MarshalIndent(registrations, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return nil, err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return registrations, nil
}
