package logs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
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

type runtimeLogMode interface {
	LogCollectionMode() bhruntime.LogCollectionMode
}

func logCollectionMode(runtime Runtime) bhruntime.LogCollectionMode {
	if detected, ok := runtime.(runtimeLogMode); ok {
		return detected.LogCollectionMode()
	}
	return bhruntime.LogCollectionSyslog
}

type Driver struct {
	runtime     Runtime
	realization LokiRealization
	app         application.Manifest
	dataDir     string
	namespace   string
}

func NewDriver(runtime Runtime, app application.Manifest, issuer serviceaccess.Issuer) *Driver {
	return &Driver{
		runtime:     runtime,
		realization: newRuntimeLokiRealization(runtime, app, issuer, "", ""),
		app:         app,
	}
}

func NewDriverAt(runtime Runtime, app application.Manifest, issuer serviceaccess.Issuer, dataDir, namespace string) *Driver {
	dataDir = filepath.Clean(dataDir)
	namespace = strings.TrimSpace(namespace)
	return &Driver{
		runtime:     runtime,
		realization: newRuntimeLokiRealization(runtime, app, issuer, dataDir, namespace),
		app:         app,
		dataDir:     dataDir,
		namespace:   namespace,
	}
}

func NewDriverWithRealization(realization LokiRealization, app application.Manifest) *Driver {
	return &Driver{realization: realization, app: app}
}

func (d *Driver) Descriptor() capability.Provider { return capability.Loki }

func (d *Driver) Preflight(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.Logs {
		return fmt.Errorf("Loki provider cannot satisfy %s", resource.Kind)
	}
	if binding.Logs == nil {
		return errors.New("logs binding is required")
	}
	format := strings.TrimSpace(binding.Logs.Format)
	if binding.Logs.Direction != "collect" ||
		(format != "runtime-stream" && format != "syslog-rfc5424") ||
		strings.TrimSpace(binding.Logs.Service) == "" {
		return errors.New("Loki provider requires collect/runtime-stream logs binding with a workload service")
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
		return errors.New("external Loki placement is not implemented by the managed log realization")
	}
	if d.realization == nil {
		return errors.New("managed Loki realization is required")
	}
	return nil
}

func (d *Driver) Provision(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	if d.realization == nil {
		return errors.New("managed Loki realization is required")
	}
	_, err := d.realization.Apply(ctx)
	return err
}

func (d *Driver) Bind(ctx context.Context, resource capability.Resource, binding capability.Binding) error {
	if binding.Logs == nil || binding.Logs.Service != resource.Name {
		return errors.New("logs binding does not match logical log source")
	}
	if d.realization == nil {
		return errors.New("managed Loki realization is required")
	}
	return d.realization.RegisterSource(ctx, LogSource{
		Application: d.app.Name,
		Environment: d.app.Environment,
		Class:       application.LogsSourceApplication,
		Service:     binding.Logs.Service,
	})
}

func (d *Driver) Verify(ctx context.Context, _ capability.Resource, binding capability.Binding) error {
	if binding.Logs == nil {
		return errors.New("logs binding is required")
	}
	if d.realization == nil {
		return errors.New("managed Loki realization is required")
	}
	return d.realization.VerifySource(ctx, LogSource{
		Application: d.app.Name,
		Environment: d.app.Environment,
		Class:       application.LogsSourceApplication,
		Service:     binding.Logs.Service,
	})
}
