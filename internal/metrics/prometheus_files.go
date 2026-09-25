package metrics

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func DesiredTargetFiles(m application.Manifest) map[string]struct{} {
	result := make(map[string]struct{}, len(m.Metrics.Sources))
	for _, source := range m.Metrics.Sources {
		result[targetFileName(m, source.Name)] = struct{}{}
	}
	return result
}

func providerSourceToken(id string) string {
	sum := sha256.Sum256([]byte(id))
	return fmt.Sprintf("%x", sum[:10])
}

func providerTargetFileName(source observability.MetricsSource) string {
	prefix := "provider--"
	if source.Security.TLSRequired {
		prefix = "provider-secure--"
	}
	return prefix + providerSourceToken(source.ID) + ".json"
}

func syncProviderTargets(dir string, sources []observability.MetricsSource) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() &&
			(strings.HasPrefix(entry.Name(), "provider--") || strings.HasPrefix(entry.Name(), "provider-secure--")) &&
			strings.HasSuffix(entry.Name(), ".json") {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	for _, source := range sources {
		target := targetGroup{
			Targets: []string{source.Target},
			Labels: map[string]string{
				"job":                     "baseharbor-providers",
				"baseharbor_provider":     string(source.Provider),
				"baseharbor_source":       source.ID,
				"baseharbor_source_class": string(source.Class),
				"baseharbor_metrics_path": source.Path,
			},
		}
		data, err := json.MarshalIndent([]targetGroup{target}, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if err := os.WriteFile(filepath.Join(dir, providerTargetFileName(source)), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func syncProviderSecurity(dir string, sources []observability.MetricsSource) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	secure := false
	for _, source := range sources {
		if source.Security.TLSRequired {
			secure = true
			break
		}
	}
	if !secure {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create provider metrics security directory: %w", err)
	}
	for _, source := range sources {
		if !source.Security.TLSRequired {
			continue
		}
		token := providerSourceToken(source.ID)
		files := []struct {
			label  string
			source string
			target string
		}{
			{label: "CA", source: source.Security.TrustFile, target: token + "-ca.pem"},
		}
		if source.Security.ClientCertificate != "" {
			files = append(files,
				struct {
					label  string
					source string
					target string
				}{label: "client certificate", source: source.Security.ClientCertificate, target: token + "-client.pem"},
				struct {
					label  string
					source string
					target string
				}{label: "client key", source: source.Security.ClientKey, target: token + "-client-key.pem"},
			)
		}
		for _, file := range files {
			data, err := os.ReadFile(file.source)
			if err != nil {
				return fmt.Errorf("read provider metrics %s for %s: %w", file.label, source.ID, err)
			}
			if len(data) == 0 {
				return fmt.Errorf("provider metrics %s for %s is empty", file.label, source.ID)
			}
			path := filepath.Join(dir, file.target)
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return fmt.Errorf("project provider metrics %s for %s: %w", file.label, source.ID, err)
			}
		}
	}
	return nil
}

func hasSecureProviderMetrics(sources []observability.MetricsSource) bool {
	for _, source := range sources {
		if source.Security.TLSRequired {
			return true
		}
	}
	return false
}

func providerMetricNetworks(sources []observability.MetricsSource) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, source := range sources {
		network := strings.TrimSpace(source.Network)
		if network == "" {
			continue
		}
		if _, exists := seen[network]; exists {
			continue
		}
		seen[network] = struct{}{}
		result = append(result, network)
	}
	sort.Strings(result)
	return result
}

func EnsureProviderFiles(ctx context.Context, issuer serviceaccess.Issuer, m application.Manifest) (ProviderFiles, error) {
	return EnsureProviderFilesWithRuntimeCA(ctx, issuer, m, "")
}

func EnsureProviderFilesWithRuntimeCA(ctx context.Context, issuer serviceaccess.Issuer, m application.Manifest, runtimeCASource string) (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	return EnsureProviderFilesWithRuntimeCAAt(ctx, issuer, dataDir, "", m, runtimeCASource)
}

func EnsureProviderFilesWithRuntimeCAAt(ctx context.Context, issuer serviceaccess.Issuer, dataDir, namespace string, m application.Manifest, runtimeCASource string) (ProviderFiles, error) {
	placement, err := PlacementForAt(dataDir, namespace, m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if placement.Scope == capability.ScopeExternal {
		return ProviderFiles{}, errors.New("external metrics provider has no BaseHarbor-owned provider files")
	}
	dir := placement.Dir
	targetsDir := filepath.Join(dir, "targets")
	if err := os.MkdirAll(targetsDir, 0o755); err != nil {
		return ProviderFiles{}, fmt.Errorf("create Prometheus provider state: %w", err)
	}
	files := ProviderFiles{
		Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"),
		Config: filepath.Join(dir, "prometheus.yml"), TargetsDir: targetsDir,
		ProviderSecurityDir: filepath.Join(dir, "provider-security"),
		Registrations:       filepath.Join(dir, "registrations.json"),
	}
	files.RuntimeCA = filepath.Join(dir, "baseharbor-runtime-ca.pem")
	registrations := []sourceRegistration{registrationForAt(m, namespace)}
	if placement.Scope == capability.ScopeShared {
		registrations, err = reconcileSharedRegistrationAt(files.Registrations, m, namespace, true)
		if err != nil {
			return ProviderFiles{}, err
		}
	}

	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	providerPlacement, err := application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return ProviderFiles{}, err
	}
	allowedApplications := []string{m.Name}
	if placement.Scope == capability.ScopeShared {
		allowedApplications = allowedApplications[:0]
		for _, registration := range registrations {
			allowedApplications = append(allowedApplications, registration.Application)
		}
	}
	providerSources, err := observability.ListMetrics(
		providerPlacement,
		allowedApplications,
		policy.Collect[application.MetricsSourceApplicationProvider],
		policy.Collect[application.MetricsSourcePlatformProvider],
	)
	if err != nil {
		return ProviderFiles{}, err
	}
	if err := syncProviderTargets(files.TargetsDir, providerSources); err != nil {
		return ProviderFiles{}, err
	}
	if err := syncProviderSecurity(files.ProviderSecurityDir, providerSources); err != nil {
		return ProviderFiles{}, err
	}
	providerNetworks := providerMetricNetworks(providerSources)

	if strings.TrimSpace(runtimeCASource) != "" {
		caData, err := os.ReadFile(runtimeCASource)
		if err != nil {
			return ProviderFiles{}, fmt.Errorf("read BaseHarbor runtime CA for Prometheus: %w", err)
		}
		if len(caData) == 0 {
			return ProviderFiles{}, errors.New("BaseHarbor runtime CA for Prometheus is empty")
		}
		if err := os.WriteFile(files.RuntimeCA, caData, 0o644); err != nil {
			return ProviderFiles{}, fmt.Errorf("persist BaseHarbor runtime CA for Prometheus: %w", err)
		}
	}
	_, caErr := os.Stat(files.RuntimeCA)
	hasRuntimeCA := caErr == nil
	if caErr != nil && !errors.Is(caErr, os.ErrNotExist) {
		return ProviderFiles{}, caErr
	}

	port := ""
	if data, err := os.ReadFile(files.Env); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && key == "BASEHARBOR_PROMETHEUS_PORT" {
				port = strings.TrimSpace(value)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProviderFiles{}, err
	}
	if port == "" {
		value, err := allocatePort()
		if err != nil {
			return ProviderFiles{}, err
		}
		port = strconv.Itoa(value)
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_PROMETHEUS_PORT="+port+"\n"), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Config, []byte(prometheusConfig(registrations, hasRuntimeCA, providerSources)), 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.Chmod(files.Config, 0o644); err != nil {
		return ProviderFiles{}, err
	}
	accessPolicy, err := serviceaccess.Resolve(prometheusAccessEnvironment(m, registrations), "prometheus", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return ProviderFiles{}, err
	}
	accessFiles, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, files.Dir, prometheusAccessSpec())
	if err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAMLWithProviderNetworksAndAccess(placement, registrations, providerNetworks, hasRuntimeCA, hasSecureProviderMetrics(providerSources), accessFiles, providerSources)), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	return files, nil
}

func ExistingProviderFiles(m application.Manifest) (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	return ExistingProviderFilesAt(dataDir, "", m)
}

func ExistingProviderFilesAt(dataDir, namespace string, m application.Manifest) (ProviderFiles, error) {
	placement, err := PlacementForAt(dataDir, namespace, m)
	if err != nil {
		return ProviderFiles{}, err
	}
	return existingProviderFilesForPlacement(placement)
}

func ExistingRegisteredProviderFiles(m application.Manifest) (ProviderFiles, bool, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, false, err
	}
	return ExistingRegisteredProviderFilesAt(dataDir, "", m)
}

func ExistingRegisteredProviderFilesAt(dataDir, namespace string, m application.Manifest) (ProviderFiles, bool, error) {
	placement, found, err := RegisteredPlacementForAt(dataDir, namespace, m)
	if err != nil || !found {
		return ProviderFiles{}, found, err
	}
	files, err := existingProviderFilesForPlacement(placement)
	if err != nil {
		return ProviderFiles{}, true, err
	}
	return files, true, nil
}

func existingProviderFilesForPlacement(placement Placement) (ProviderFiles, error) {
	if placement.Scope == capability.ScopeExternal {
		return ProviderFiles{}, os.ErrNotExist
	}
	return providerFilesAt(placement.Dir)
}

func UnregisterSharedApplication(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, m application.Manifest) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return UnregisterSharedApplicationAt(ctx, runtime, issuer, dataDir, "", m)
}

func UnregisterSharedApplicationAt(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, dataDir, namespace string, m application.Manifest) error {
	providerPlacement, found, err := application.RegisteredProviderPlacementAt(dataDir, m, capability.ProviderPrometheus)
	if err != nil {
		return err
	}
	if !found || providerPlacement.Scope != capability.ScopeShared {
		return nil
	}
	placement, err := placementFromProviderPlacementAt(dataDir, namespace, m, providerPlacement)
	if err != nil {
		return err
	}
	files, err := existingProviderFilesForPlacement(placement)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	registrations, err := reconcileSharedRegistrationAt(files.Registrations, m, namespace, false)
	if err != nil {
		return err
	}

	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return err
	}
	allowedApplications := make([]string, 0, len(registrations))
	for _, registration := range registrations {
		allowedApplications = append(allowedApplications, registration.Application)
	}
	providerSources, err := observability.ListMetrics(
		providerPlacement,
		allowedApplications,
		policy.Collect[application.MetricsSourceApplicationProvider],
		policy.Collect[application.MetricsSourcePlatformProvider],
	)
	if err != nil {
		return err
	}
	if err := syncProviderTargets(files.TargetsDir, providerSources); err != nil {
		return err
	}
	if err := syncProviderSecurity(files.ProviderSecurityDir, providerSources); err != nil {
		return err
	}
	providerNetworks := providerMetricNetworks(providerSources)

	_, caErr := os.Stat(files.RuntimeCA)
	hasRuntimeCA := caErr == nil
	if caErr != nil && !errors.Is(caErr, os.ErrNotExist) {
		return caErr
	}
	if err := os.WriteFile(files.Config, []byte(prometheusConfig(registrations, hasRuntimeCA, providerSources)), 0o644); err != nil {
		return err
	}
	if err := os.Chmod(files.Config, 0o644); err != nil {
		return err
	}

	accessPolicy, err := serviceaccess.Resolve(prometheusAccessEnvironment(m, registrations), "prometheus", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return err
	}
	accessFiles, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, files.Dir, prometheusAccessSpec())
	if err != nil {
		return err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAMLWithProviderNetworksAndAccess(placement, registrations, providerNetworks, hasRuntimeCA, hasSecureProviderMetrics(providerSources), accessFiles, providerSources)), 0o600); err != nil {
		return err
	}
	if err := runtime.ConfigProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate shared Prometheus after application unregister: %w", err)
	}
	if err := runtime.UpProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("reconcile shared Prometheus after application unregister: %w", err)
	}
	return nil
}
