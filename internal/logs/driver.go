package logs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

const (
	LokiImage       = "grafana/loki:3.7.8"
	AlloyImage      = "grafana/alloy:v1.19.2"
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
	app     application.Manifest
	client  *http.Client
}

func NewDriver(runtime Runtime, app application.Manifest) *Driver {
	return &Driver{runtime: runtime, app: app, client: &http.Client{Timeout: 10 * time.Second}}
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
	files, err := EnsureProviderFiles(d.app)
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
	return waitLokiReady(ctx, d.client, endpoint)
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
	return waitForStream(ctx, d.client, endpoint, d.app, binding.Logs.Service)
}
