package logs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"path/filepath"
)

const (
	LokiImage       = "docker.io/grafana/loki:3.7.8"
	AlloyImage      = "docker.io/grafana/alloy:v1.19.2"
	LokiRuntimeUID  = 10001
	LokiRuntimeGID  = 10001
	AlloyRuntimeUID = 473
	AlloyRuntimeGID = 473
)

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	StopProject(context.Context, string, string, string) error
	DestroyProject(context.Context, string, string, string) error
}

type Driver struct {
	runtime Runtime
	engine  string
	app     application.Manifest
	client  *http.Client
}

type runtimeEngine interface {
	Engine() string
}

func runtimeKind(runtime Runtime) string {
	if detected, ok := runtime.(runtimeEngine); ok {
		switch strings.ToLower(strings.TrimSpace(detected.Engine())) {
		case "podman":
			return "podman"
		case "docker":
			return "docker"
		}
	}
	return "docker"
}

func NewDriver(runtime Runtime, app application.Manifest) *Driver {
	return &Driver{runtime: runtime, engine: runtimeKind(runtime), app: app}
}

func lokiHTTPClient(m application.Manifest, files ProviderFiles) (*http.Client, error) {
	policy, err := serviceaccess.Resolve(m.Environment, "loki", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return nil, err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return nil, fmt.Errorf("load Loki service access identity: %w", err)
	}
	return serviceaccess.NewHTTPClientForPolicy(material, policy)
}

func (d *Driver) Descriptor() capability.Provider { return capability.Loki }

func (d *Driver) Preflight(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.Logs {
		return fmt.Errorf("Loki provider cannot satisfy %s", resource.Kind)
	}
	if binding.Logs == nil {
		return errors.New("logs binding is required")
	}
	if binding.Logs.Direction != "collect" || binding.Logs.Format != "syslog-rfc5424" || strings.TrimSpace(binding.Logs.Service) == "" {
		return errors.New("Loki provider requires collect/syslog-rfc5424 logs binding with a workload service")
	}
	policy, err := application.LogsPolicy(d.app)
	if err != nil {
		return err
	}
	if !policy.Enabled || !policy.Collect[application.LogsSourceApplication] {
		return errors.New("application log collection is disabled by deployment policy")
	}
	placement, err := application.ResolveProviderPlacement(d.app, capability.ProviderLoki)
	if err != nil {
		return err
	}
	if placement.Scope == capability.ScopeExternal {
		return errors.New("external Loki placement is not implemented by the Compose log collector")
	}
	return nil
}

func (d *Driver) Provision(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	files, err := EnsureProviderFilesForRuntime(d.app, d.engine)
	if err != nil {
		return err
	}
	placement, err := PlacementFor(d.app)
	if err != nil {
		return err
	}
	if err := d.runtime.ConfigProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate Loki provider configuration: %w", err)
	}
	if err := d.runtime.UpProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("start Loki provider: %w", err)
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	d.client, err = lokiHTTPClient(d.app, files)
	if err != nil {
		return err
	}
	if err := waitLokiReady(ctx, d.client, endpoint); err != nil {
		return err
	}
	class := observability.SourcePlatformProvider
	if placement.Scope == capability.ScopeApplication {
		class = observability.SourceApplicationProvider
	}
	return observability.Update(observability.MetricsSource{
		ID:               "loki:" + placement.Project,
		Provider:         capability.ProviderLoki,
		Class:            class,
		Scope:            placement.Scope,
		SharingBoundary:  placement.SharingBoundary,
		OwnerApplication: placement.OwnerApplication,
		Network:          placement.Network,
		Target:           "loki:3100",
		Path:             "/metrics",
	})
}

func (d *Driver) Bind(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if binding.Logs == nil || binding.Logs.Service != resource.Name {
		return errors.New("logs binding does not match logical log source")
	}
	_, err := ApplicationRegistration(d.app)
	return err
}

func (d *Driver) Verify(ctx context.Context, _ capability.Resource, binding capability.Binding) error {
	if binding.Logs == nil {
		return errors.New("logs binding is required")
	}
	files, err := ExistingProviderFiles(d.app)
	if err != nil {
		return err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	if d.client == nil {
		d.client, err = lokiHTTPClient(d.app, files)
		if err != nil {
			return err
		}
	}
	return waitForStream(ctx, d.client, endpoint, d.app, binding.Logs.Service)
}
