package logs

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"os"
	"strconv"
	"strings"
)

func UnregisterApplication(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, m application.Manifest) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return UnregisterApplicationAt(ctx, runtime, issuer, dataDir, "", m)
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
	registered := false
	for _, registration := range existing {
		if registration.Application == m.Name && registration.Environment == m.Environment {
			registered = true
			break
		}
	}
	if !registered {
		return nil
	}
	registrations, err := reconcileRegistration(files.Registrations, m, false)
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
	if runtimeKind(runtime) == "docker" && hasPlatformProviderLogs(providerSources) {
		platformSyslogPort, err = persistedOrAllocatedUDPPort(files.Env, "BASEHARBOR_PLATFORM_PROVIDER_SYSLOG_PORT")
		if err != nil {
			return err
		}
	}
	if err := os.WriteFile(files.AlloyConfig, []byte(alloyConfigForRuntimeSources(registrations, providerSources, runtimeKind(runtime), platformSyslogPort)), 0o644); err != nil {
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
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAMLForRuntimeAndAccess(p, registrations, runtimeKind(runtime), accessFiles, platformSyslogPort)), 0o600); err != nil {
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
	}
}
