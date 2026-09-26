package logs

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"os"
	"path/filepath"
	"strings"
)

const (
	providerProject      = "baseharbor-logs"
	workloadOverrideName = "workload.logging.override.yaml"
	providerOverrideName = "provider.logging.override.yaml"
)

type Placement struct {
	Scope            capability.ProviderScope
	Project          string
	Network          string
	Dir              string
	LokiVolume       string
	AlloyVolume      string
	SharingBoundary  string
	OwnerApplication string
}

type Registration struct {
	Application        string `json:"application"`
	Environment        string `json:"environment"`
	Namespace          string `json:"namespace,omitempty"`
	SyslogPort         int    `json:"syslog_port"`
	ProviderSyslogPort int    `json:"provider_syslog_port"`
}

type ProviderFiles struct {
	Dir           string
	Compose       string
	Env           string
	LokiConfig    string
	AlloyConfig   string
	Registrations string
}

func PlacementFor(m application.Manifest) (Placement, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return Placement{}, err
	}
	return PlacementForAt(dataDir, "", m)
}

func PlacementForAt(dataDir, namespace string, m application.Manifest) (Placement, error) {
	p, err := application.ResolveProviderPlacement(m, capability.ProviderLoki)
	if err != nil {
		return Placement{}, err
	}
	namespace = strings.TrimSpace(strings.ReplaceAll(namespace, ".", "-"))
	prefix := ""
	if namespace != "" {
		prefix = namespace + "-"
	}
	switch p.Scope {
	case capability.ScopeShared:
		project := bhruntime.SharedProjectName(namespace)
		dir := filepath.Join(filepath.Clean(dataDir), "providers", "loki", "shared")
		lokiVolume := "baseharbor-loki-data"
		alloyVolume := "baseharbor-alloy-data"
		if p.SharingBoundary != "" {
			token := application.ProviderPlacementNameToken(p.SharingBoundary)
			project += "-" + token
			dir = filepath.Join(dir, token)
			lokiVolume += "-" + token
			alloyVolume += "-" + token
		}
		network := project + "-internal"
		return Placement{Scope: p.Scope, Project: project, Network: network, Dir: dir, LokiVolume: lokiVolume, AlloyVolume: alloyVolume, SharingBoundary: p.SharingBoundary}, nil
	case capability.ScopeApplication:
		suffix := prefix + m.Name + "-" + m.Environment
		project := providerProject + "-" + suffix
		return Placement{
			Scope:            p.Scope,
			Project:          project,
			Network:          project + "-internal",
			Dir:              filepath.Join(filepath.Clean(dataDir), "providers", "loki", "applications", m.Name, m.Environment),
			LokiVolume:       "baseharbor-loki-data-" + suffix,
			AlloyVolume:      "baseharbor-alloy-data-" + suffix,
			OwnerApplication: m.Name,
		}, nil
	case capability.ScopeExternal:
		return Placement{Scope: p.Scope}, nil
	default:
		return Placement{}, fmt.Errorf("unsupported Loki provider scope %q", p.Scope)
	}
}

func providerFiles(p Placement) ProviderFiles {
	return ProviderFiles{
		Dir:           p.Dir,
		Compose:       filepath.Join(p.Dir, "compose.yaml"),
		Env:           filepath.Join(p.Dir, "runtime.env"),
		LokiConfig:    filepath.Join(p.Dir, "loki.yaml"),
		AlloyConfig:   filepath.Join(p.Dir, "config.alloy"),
		Registrations: filepath.Join(p.Dir, "registrations.json"),
	}
}

func EnsureProviderFiles(ctx context.Context, issuer serviceaccess.Issuer, m application.Manifest) (ProviderFiles, error) {
	return EnsureProviderFilesForRuntime(ctx, issuer, m, "docker")
}

func EnsureProviderFilesForRuntime(ctx context.Context, issuer serviceaccess.Issuer, m application.Manifest, runtimeKind string) (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	return EnsureProviderFilesForRuntimeAt(ctx, issuer, dataDir, "", m, runtimeKind)
}

func EnsureProviderFilesForRuntimeAt(ctx context.Context, issuer serviceaccess.Issuer, dataDir, namespace string, m application.Manifest, runtimeKind string) (ProviderFiles, error) {
	p, err := PlacementForAt(dataDir, namespace, m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if p.Scope == capability.ScopeExternal {
		return ProviderFiles{}, errors.New("external Loki provider has no BaseHarbor-owned provider files")
	}
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.Chmod(p.Dir, 0o700); err != nil {
		return ProviderFiles{}, err
	}
	files := providerFiles(p)
	registrations, err := reconcileRegistrationAt(files.Registrations, m, namespace, true)
	if err != nil {
		return ProviderFiles{}, err
	}
	providerSources, err := providerLogSources(p, registrations)
	if err != nil {
		return ProviderFiles{}, err
	}
	lokiPort, err := persistedOrAllocatedPort(files.Env, "BASEHARBOR_LOKI_PORT")
	if err != nil {
		return ProviderFiles{}, err
	}
	platformSyslogPort := 0
	if strings.EqualFold(strings.TrimSpace(runtimeKind), "docker") && hasPlatformProviderLogs(providerSources) {
		platformSyslogPort, err = persistedOrAllocatedUDPPort(files.Env, "BASEHARBOR_PLATFORM_PROVIDER_SYSLOG_PORT")
		if err != nil {
			return ProviderFiles{}, err
		}
	}
	var env strings.Builder
	fmt.Fprintf(&env, "BASEHARBOR_LOKI_PORT=%d\n", lokiPort)
	if platformSyslogPort > 0 {
		fmt.Fprintf(&env, "BASEHARBOR_PLATFORM_PROVIDER_SYSLOG_PORT=%d\n", platformSyslogPort)
	}
	if err := os.WriteFile(files.Env, []byte(env.String()), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.LokiConfig, []byte(lokiConfig()), 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.Chmod(files.LokiConfig, 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.AlloyConfig, []byte(alloyConfigForRuntimeSources(registrations, providerSources, runtimeKind, platformSyslogPort)), 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.Chmod(files.AlloyConfig, 0o644); err != nil {
		return ProviderFiles{}, err
	}
	accessPolicy, err := serviceaccess.Resolve(lokiAccessEnvironment(m, registrations), "loki", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return ProviderFiles{}, err
	}
	accessFiles, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, files.Dir, lokiAccessSpec())
	if err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAMLForRuntimeAndAccess(p, registrations, runtimeKind, accessFiles, platformSyslogPort)), 0o600); err != nil {
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
	p, err := PlacementForAt(dataDir, namespace, m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if p.Scope == capability.ScopeExternal {
		return ProviderFiles{}, errors.New("external Loki provider has no BaseHarbor-owned provider files")
	}
	files := providerFiles(p)
	for _, path := range []string{files.Compose, files.Env, files.LokiConfig, files.AlloyConfig, files.Registrations} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}
