package metrics

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
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

// applicationTargetPrefix is the file prefix for application-class metrics
// targets. It is structurally disjoint from the provider target family
// ("provider--"/"provider-secure--"): no valid application slug (including
// "provider" or "provider-secure") can ever produce a filename that begins with
// "application--", so no application target can match the provider glob and the
// two job globs can never collide on a single file.
const applicationTargetPrefix = "application--"

// appTargetFileName derives the application--<application>--<environment>--<source>.json
// path an application-class metrics source must be written to. The ID grammar is
// application:metrics:<app>:<env>:<src>; the source segment is parts[2]. Fails
// closed (empty) if the ID does not match the grammar.
func appTargetFileName(source observability.MetricsSource) string {
	const prefix = "application:metrics:"
	if !strings.HasPrefix(source.ID, prefix) {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(source.ID, prefix), ":")
	if len(parts) != 3 {
		return ""
	}
	m := application.Manifest{Name: parts[0], Environment: parts[1]}
	return applicationTargetPrefix + targetFileName(m, parts[2])
}

// appTargetEnvironment returns the environment segment of an application-metrics
// ID (application:metrics:<app>:<env>:<src>); empty when the ID does not match
// the grammar. It is the single source of truth for the filename and the
// baseharbor_environment label, so they can never diverge.
func appTargetEnvironment(source observability.MetricsSource) string {
	const prefix = "application:metrics:"
	if !strings.HasPrefix(source.ID, prefix) {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(source.ID, prefix), ":")
	if len(parts) != 3 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// metricsPathFor returns the baseharbor_metrics_path label value for a target:
// the source's declared path when set, otherwise the default scrape path "/".
func metricsPathFor(source observability.MetricsSource) string {
	path := strings.TrimSpace(source.Path)
	if path == "" {
		return "/"
	}
	return path
}

func syncProviderTargets(dir string, sources []observability.MetricsSource) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	// Room-scoped sweep: delete every stale file in the prefix-rooms this
	// writer owns, then rewrite the rooms from the current source list on the
	// next loop. The two rooms belong to two distinct writers:
	//
	// provider-- / provider-secure-- : this sync OWNS and ALWAYS sweeps this
	//   room — it is the only production writer of these files
	//   (prometheus_files.go:83), and the room is a complete rewrite of a
	//   BaseHarbor-owned dir (normalized back to the container projection
	//   afterwards). Every run removes every provider file in the room; a
	//   deselected source therefore leaves no orphan file in the active
	//   provider glob, regardless of what the current list carries.
	// application-- : the room owned by an application-class source; this
	//   provider sync never touches this room, so an application file owned
	//   by another sync is never rewritten or deleted.
	// Files in neither prefix-room (runtime-target files, anything else owned
	// by another writer) are left untouched.
	hasApplication := false
	for _, source := range sources {
		if source.Class == observability.SourceApplication {
			hasApplication = true
			break
		}
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		switch {
		case strings.HasPrefix(name, applicationTargetPrefix) && hasApplication:
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		case strings.HasPrefix(name, "provider--") || strings.HasPrefix(name, "provider-secure--"):
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	for _, source := range sources {
		if source.Verification == capability.ObservabilityVerifyNone {
			continue
		}
		if source.Class == observability.SourceApplication {
			// An application-class source is written to its own
			// application--<app>--<env>--<source>.json file, never to a
			// provider--<hash>.json file, so the two job globs never collide.
			if err := writeApplicationTargetFile(dir, source); err != nil {
				return err
			}
			continue
		}
		data, err := targetGroupPayload(source)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, providerTargetFileName(source)), data, 0o644); err != nil {
			return err
		}
	}
	return bhruntime.NormalizeContainerReadableProjection(dir)
}

// targetGroupPayload serializes a provider-class source into a single
// targetGroup payload with its baseharbor_* labels.
func targetGroupPayload(source observability.MetricsSource) ([]byte, error) {
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
	return json.MarshalIndent([]targetGroup{target}, "", "  ")
}

// writeApplicationTargetFile writes a single application-class metrics source to
// its own application--<app>--<env>--<source>.json file in the baseharbor-
// applications job path, carrying the baseharbor_application/environment/source
// labels the application verification query keys on. It never touches a
// provider--<hash>.json file, so the app-class source leaves the provider file
// entirely.
func writeApplicationTargetFile(dir string, source observability.MetricsSource) error {
	name := appTargetFileName(source)
	if name == "" {
		return fmt.Errorf("metrics target for %q has no application filename", source.ID)
	}
	target := targetGroup{
		Targets: []string{source.Target},
		Labels: map[string]string{
			"job":                       "baseharbor-applications",
			"baseharbor_application":    source.OwnerApplication,
			"baseharbor_environment":    appTargetEnvironment(source),
			"baseharbor_source":         source.ID,
			"baseharbor_source_class":   string(source.Class),
			"baseharbor_metrics_path":   metricsPathFor(source),
			"baseharbor_metrics_scheme": source.Scheme,
		},
	}
	data, err := json.MarshalIndent([]targetGroup{target}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return err
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
	return bhruntime.NormalizeContainerReadableProjection(dir)
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
		if source.Verification == capability.ObservabilityVerifyNone {
			continue
		}
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
	members, err := providertopology.ResolveMembers(filepath.Join(placement.Dir, "compose.yaml"), "prometheus", 2, application.AvailabilityIntent(m).Resolve("metrics"))
	if err != nil {
		return ProviderFiles{}, err
	}
	placement.Members = members
	dir := placement.Dir
	targetsDir := filepath.Join(dir, "targets")
	if err := os.MkdirAll(targetsDir, 0o755); err != nil {
		return ProviderFiles{}, fmt.Errorf("create Prometheus provider state: %w", err)
	}
	files := ProviderFiles{
		Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"),
		Config: filepath.Join(dir, "prometheus.yml"), WebConfig: filepath.Join(dir, "web-config.yml"), TargetsDir: targetsDir,
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
		m.Environment,
	)
	if err != nil {
		return ProviderFiles{}, err
	}
	// An application-class metrics source must be scrapeable through the
	// baseharbor-applications job (card 0428-123). ListMetrics excludes
	// application-class sources by contract, so collect them separately and
	// fold them into the sources written to the target files. The
	// application-class sources are written to their own
	// application--<app>--<env>--<source>.json file (disjoint from the
	// provider--<hash>.json family) by syncProviderTargets; a
	// Verification==none application-class source stays a workload-only
	// snapshot and never enters the scrape chain.
	if policy.Collect[application.MetricsSourceApplication] {
		appSources, appErr := observability.ListApplicationMetrics(providerPlacement, allowedApplications, m.Environment)
		if appErr != nil {
			return ProviderFiles{}, appErr
		}
		providerSources = append(providerSources, appSources...)
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
	if hasRuntimeCA {
		if err := os.Chmod(files.RuntimeCA, 0o644); err != nil {
			return ProviderFiles{}, fmt.Errorf("set BaseHarbor runtime CA permissions for Prometheus: %w", err)
		}
	}

	values := map[string]string{}
	if data, err := os.ReadFile(files.Env); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && strings.TrimSpace(key) != "" {
				values[strings.TrimSpace(key)] = value
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProviderFiles{}, err
	}
	port := strings.TrimSpace(values["BASEHARBOR_PROMETHEUS_PORT"])
	if port == "" {
		value, err := allocatePort()
		if err != nil {
			return ProviderFiles{}, err
		}
		port = strconv.Itoa(value)
		values["BASEHARBOR_PROMETHEUS_PORT"] = port
	}

	accessEnvironment := prometheusAccessEnvironment(m, registrations)
	if accessEnvironment == "dev" && m.Services.ObservabilityManagementUI {
		target := strings.TrimSpace(namespace)
		if target == "" {
			target = "local"
		}
		credentials, err := devaccess.Ensure(target, m.Environment)
		if err != nil {
			return ProviderFiles{}, fmt.Errorf("prepare Prometheus developer access: %w", err)
		}
		values["BASEHARBOR_PROMETHEUS_UI_USER"] = credentials.Username
		values["BASEHARBOR_PROMETHEUS_UI_PASSWORD"] = credentials.Password
	}
	var envBuilder strings.Builder
	if username, password := strings.TrimSpace(values["BASEHARBOR_PROMETHEUS_UI_USER"]), values["BASEHARBOR_PROMETHEUS_UI_PASSWORD"]; username != "" && password != "" {
		values["BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
	} else {
		delete(values, "BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION")
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&envBuilder, "%s=%s\n", key, values[key])
	}
	if err := os.WriteFile(files.Env, []byte(envBuilder.String()), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Config, []byte(prometheusConfig(registrations, hasRuntimeCA, providerSources)), 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.Chmod(files.Config, 0o644); err != nil {
		return ProviderFiles{}, err
	}
	accessPolicy, err := serviceaccess.Resolve(accessEnvironment, "prometheus", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return ProviderFiles{}, err
	}
	accessPolicy.ServerName = "prometheus"
	memberPolicy := accessPolicy
	memberPolicy.AuthenticationRequired = false
	memberPolicy.Authentication = serviceaccess.AuthenticationNative
	memberTLS, err := serviceaccess.EnsureNativeTLS(ctx, issuer, memberPolicy, filepath.Join(files.Dir, "members"), append([]string{"prometheus"}, providertopology.Names("prometheus", placement.Members)...)...)
	if err != nil {
		return ProviderFiles{}, err
	}
	if err := writePrometheusWebConfig(files.WebConfig, memberPolicy, values); err != nil {
		return ProviderFiles{}, err
	}
	accessSpec := prometheusHAAccessSpec(memberTLS.Material.CA, placement.Members)
	if values["BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION"] != "" {
		accessSpec.HealthAuthorizationEnv = "BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION"
	}
	accessFiles, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, files.Dir, accessSpec)
	if err != nil {
		return ProviderFiles{}, err
	}
	if err := writeRuntimeSourceSnapshot(files, placement, providerSources); err != nil {
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
		m.Environment,
	)
	if err != nil {
		return err
	}
	if policy.Collect[application.MetricsSourceApplication] {
		appSources, appErr := observability.ListApplicationMetrics(providerPlacement, allowedApplications, m.Environment)
		if appErr != nil {
			return appErr
		}
		providerSources = append(providerSources, appSources...)
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

	members, err := providertopology.ResolveMembers(files.Compose, "prometheus", 2)
	if err != nil {
		return err
	}
	placement.Members = members
	accessEnvironment := prometheusAccessEnvironment(m, registrations)
	accessPolicy, err := serviceaccess.Resolve(accessEnvironment, "prometheus", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return err
	}
	accessPolicy.ServerName = "prometheus"
	memberPolicy := accessPolicy
	memberPolicy.AuthenticationRequired = false
	memberPolicy.Authentication = serviceaccess.AuthenticationNative
	memberTLS, err := serviceaccess.EnsureNativeTLS(ctx, issuer, memberPolicy, filepath.Join(files.Dir, "members"), append([]string{"prometheus"}, providertopology.Names("prometheus", placement.Members)...)...)
	if err != nil {
		return err
	}
	values, err := readPrometheusEnvironment(files.Env)
	if err != nil {
		return err
	}
	if err := writePrometheusWebConfig(files.WebConfig, memberPolicy, values); err != nil {
		return err
	}
	accessSpec := prometheusHAAccessSpec(memberTLS.Material.CA, placement.Members)
	if values["BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION"] != "" {
		accessSpec.HealthAuthorizationEnv = "BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION"
	}
	accessFiles, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, files.Dir, accessSpec)
	if err != nil {
		return err
	}
	if err := writeRuntimeSourceSnapshot(files, placement, providerSources); err != nil {
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

func prometheusHAAccessSpec(memberCA string, requested ...int) serviceaccess.HTTPGatewaySpec {
	members := 2
	if len(requested) > 0 {
		members = requested[0]
	}
	if members < 1 {
		members = 1
	}
	var upstreams []string
	for _, name := range providertopology.Names("prometheus", members) {
		upstreams = append(upstreams, "https://"+name+":9090")
	}
	return serviceaccess.HTTPGatewaySpec{
		ServiceName:        "prometheus-access",
		Upstreams:          upstreams,
		UpstreamTrustFile:  memberCA,
		UpstreamServerName: "prometheus",
		PublishedPortEnv:   "BASEHARBOR_PROMETHEUS_PORT",
		ContainerPort:      9090,
		Networks:           []string{"access", "publish"},
		NetworkAliases:     []string{"prometheus"},
		RequireClient:      true,
		HealthURI:          "/-/ready",
	}
}

func readPrometheusEnvironment(path string) (map[string]string, error) {
	values := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && strings.TrimSpace(key) != "" {
			values[strings.TrimSpace(key)] = value
		}
	}
	return values, nil
}

func writePrometheusWebConfig(path string, policy serviceaccess.Policy, values map[string]string) error {
	var b strings.Builder
	b.WriteString("tls_server_config:\n")
	b.WriteString("  cert_file: /run/baseharbor/tls/server.pem\n")
	b.WriteString("  key_file: /run/baseharbor/tls/server-key.pem\n")
	b.WriteString("  min_version: TLS12\n")
	if policy.AuthenticationRequired && policy.Authentication == serviceaccess.AuthenticationMTLS {
		b.WriteString("  client_ca_file: /run/baseharbor/tls/ca.pem\n")
		b.WriteString("  client_auth_type: RequireAndVerifyClientCert\n")
	}
	username := strings.TrimSpace(values["BASEHARBOR_PROMETHEUS_UI_USER"])
	password := values["BASEHARBOR_PROMETHEUS_UI_PASSWORD"]
	if username != "" || password != "" {
		if username == "" || password == "" {
			return errors.New("Prometheus native basic auth credentials are incomplete")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash Prometheus native basic auth password: %w", err)
		}
		b.WriteString("basic_auth_users:\n")
		fmt.Fprintf(&b, "  %s: %s\n", strconv.Quote(username), strconv.Quote(string(hash)))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write Prometheus native web config: %w", err)
	}
	// The Core service uses a restrictive umask. Explicitly restore the
	// container-readable mode; the containing provider directory remains
	// private on the host and the file is mounted read-only into Prometheus.
	if err := os.Chmod(path, 0o644); err != nil {
		return fmt.Errorf("set Prometheus native web config permissions: %w", err)
	}
	return nil
}
