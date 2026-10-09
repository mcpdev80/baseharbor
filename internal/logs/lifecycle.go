package logs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func UnregisterApplication(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, m application.Manifest) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return UnregisterApplicationAt(ctx, runtime, issuer, dataDir, "", m)
}

// ApplicationRegisteredAt checks the existing collector ownership registry
// without materializing provider or trust state. Retained state without its
// registry is incomplete, rather than evidence of an undeployed application.
func ApplicationRegisteredAt(dataDir, namespace string, m application.Manifest) (bool, error) {
	p, err := PlacementForAt(dataDir, namespace, m)
	if err != nil || p.Scope == capability.ScopeExternal {
		return false, err
	}
	files := providerFiles(p)
	registrations, err := readRegistrations(files.Registrations)
	if errors.Is(err, os.ErrNotExist) {
		entries, readErr := os.ReadDir(files.Dir)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return false, readErr
		}
		if len(entries) > 0 {
			return false, errors.New("log collector state is incomplete; inspect the retained provider registration before retrying destroy")
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return hasApplicationRegistration(registrations, m), nil
}

func hasApplicationRegistration(registrations []Registration, m application.Manifest) bool {
	for _, registration := range registrations {
		if registration.Application == m.Name && registration.Environment == m.Environment {
			return true
		}
	}
	return false
}

func UnregisterApplicationAt(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, dataDir, namespace string, m application.Manifest) error {
	p, err := PlacementForAt(dataDir, namespace, m)
	if err != nil || p.Scope == capability.ScopeExternal {
		return err
	}
	files := providerFiles(p)
	existing, err := readRegistrations(files.Registrations)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !hasApplicationRegistration(existing, m) {
		return nil
	}
	registrations, err := reconcileRegistrationAt(files.Registrations, m, namespace, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if p.Scope == capability.ScopeApplication || len(registrations) == 0 {
		return DestroyProviderAt(ctx, runtime, dataDir, namespace, m)
	}
	providerSources, err := providerLogSources(p, registrations)
	if err != nil {
		return err
	}
	platformSyslogPort := 0
	if logCollectionMode(runtime) == bhruntime.LogCollectionSyslog && hasPlatformProviderLogs(providerSources) {
		platformSyslogPort, err = persistedOrAllocatedUDPPort(files.Env, "BASEHARBOR_PLATFORM_PROVIDER_SYSLOG_PORT")
		if err != nil {
			return err
		}
	}
	if err := os.WriteFile(files.AlloyConfig, []byte(alloyConfigForModeSources(registrations, providerSources, logCollectionMode(runtime), platformSyslogPort)), 0o644); err != nil {
		return err
	}
	if err := os.Chmod(files.AlloyConfig, 0o644); err != nil {
		return err
	}
	accessPolicy, err := serviceaccess.Resolve(lokiAccessEnvironment(m, registrations), "loki", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return err
	}
	accessFiles, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, files.Dir, lokiAccessSpec())
	if err != nil {
		return err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAMLForModeAndAccess(p, registrations, logCollectionMode(runtime), accessFiles, platformSyslogPort)), 0o600); err != nil {
		return err
	}
	if err := runtime.ConfigProject(ctx, p.Project, files.Compose, files.Env); err != nil {
		return err
	}
	return runtime.UpProject(ctx, p.Project, files.Compose, files.Env)
}

func StopProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return StopProviderAt(ctx, runtime, dataDir, "", m)
}

func StopProviderAt(ctx context.Context, runtime Runtime, dataDir, namespace string, m application.Manifest) error {
	p, err := PlacementForAt(dataDir, namespace, m)
	if err != nil || p.Scope != capability.ScopeApplication {
		return err
	}
	files, err := ExistingProviderFilesAt(dataDir, namespace, m)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return runtime.StopProject(ctx, p.Project, files.Compose, files.Env)
}

func DestroyProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return DestroyProviderAt(ctx, runtime, dataDir, "", m)
}

func DestroyProviderAt(ctx context.Context, runtime Runtime, dataDir, namespace string, m application.Manifest) error {
	p, err := PlacementForAt(dataDir, namespace, m)
	if err != nil || p.Scope == capability.ScopeExternal {
		return err
	}
	files := providerFiles(p)
	if _, err := os.Stat(files.Compose); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, p.Project, files.Compose, files.Env); err != nil {
		return err
	}
	_ = observability.Remove("loki:" + p.Project)
	return os.RemoveAll(p.Dir)
}

func ProviderEndpoint(files ProviderFiles) (string, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "BASEHARBOR_LOKI_PORT" {
			port, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || port < 1 || port > 65535 {
				return "", errors.New("invalid Loki API port")
			}
			return fmt.Sprintf("https://127.0.0.1:%d", port), nil
		}
	}
	return "", errors.New("Loki API port is missing")
}

func lokiAccessEnvironment(m application.Manifest, registrations []Registration) string {
	managed := !lokiDevelopmentEnvironment(m.Environment)
	for _, registration := range registrations {
		if !lokiDevelopmentEnvironment(registration.Environment) {
			managed = true
			break
		}
	}
	if managed {
		return "prod"
	}
	return "dev"
}

func lokiDevelopmentEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "dev", "development":
		return true
	default:
		return false
	}
}

func lokiAccessSpec() serviceaccess.HTTPGatewaySpec {
	return serviceaccess.HTTPGatewaySpec{
		ServiceName:      "loki-access",
		Upstream:         "http://loki:3100",
		PublishedPortEnv: "BASEHARBOR_LOKI_PORT",
		ContainerPort:    8443,
		Networks:         []string{"logs-internal", "logs-publish"},
		RequireClient:    true,
		// /ready reflects cluster-wide module readiness and can become 503
		// transiently after a member loss even while this process can serve
		// queries. The frontend health check must only evict dead processes;
		// query/ingestion continuity is verified separately by the HA gate.
		HealthURI:    "/loki/api/v1/status/buildinfo",
		HealthStatus: http.StatusOK,
	}
}
