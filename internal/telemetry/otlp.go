package telemetry

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	ProviderProject    = "baseharbor-telemetry"
	ProviderService    = "otel-collector"
	ProviderNetwork    = "baseharbor-telemetry"
	ProviderImage      = "otel/opentelemetry-collector-contrib:0.161.0"
	ExternalHeadersEnv = "BASEHARBOR_OTLP_HEADERS"
	ProbeTraceIDHex    = "42617365486172626f72303430370001"
)

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	DestroyProject(context.Context, string, string, string) error
}

type Driver struct {
	runtime          Runtime
	app              application.Manifest
	files            application.RuntimeFiles
	externalEndpoint string
	traceEndpoint    string
	traceNetwork     string
	client           *http.Client
}

type ProviderFiles struct {
	Dir     string
	Compose string
	Env     string
	Config  string
}

func NewDriver(runtime Runtime, app application.Manifest, files application.RuntimeFiles) *Driver {
	return &Driver{
		runtime:          runtime,
		app:              app,
		files:            files,
		externalEndpoint: application.ExternalOTLPEndpoint(),
		client:           &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *Driver) Descriptor() capability.Provider {
	return application.TelemetryProviderForDeployment()
}

func (d *Driver) SetTraceBackend(endpoint, network string) {
	d.traceEndpoint = strings.TrimSpace(endpoint)
	d.traceNetwork = strings.TrimSpace(network)
}

func (d *Driver) Preflight(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.TelemetryOTLP {
		return fmt.Errorf("OTLP provider cannot satisfy %s", resource.Kind)
	}
	if binding.TelemetryOTLP == nil {
		return errors.New("OTLP telemetry binding is required")
	}
	if binding.TelemetryOTLP.Direction != "export" || binding.TelemetryOTLP.Protocol != "http/protobuf" {
		return errors.New("OTLP provider requires export over http/protobuf")
	}
	if len(binding.TelemetryOTLP.Signals) == 0 {
		return errors.New("OTLP provider requires at least one telemetry signal")
	}
	if resource.Provider == capability.ProviderExternalOTLP {
		if strings.TrimSpace(d.externalEndpoint) == "" {
			return errors.New("external OTLP provider requires BASEHARBOR_OTLP_ENDPOINT")
		}
		u, err := url.Parse(d.externalEndpoint)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("external OTLP endpoint must be an absolute http or https URL")
		}
		if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("external OTLP endpoint must not contain credentials, query parameters or fragments; use BASEHARBOR_OTLP_HEADERS for authorization")
		}
	}
	return nil
}

func (d *Driver) Provision(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	if resource.Provider == capability.ProviderExternalOTLP {
		return nil
	}
	files, err := EnsureProviderFilesWithTraceBackend(d.traceEndpoint, d.traceNetwork)
	if err != nil {
		return err
	}
	if err := d.runtime.ConfigProject(ctx, ProviderProject, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate OpenTelemetry Collector configuration: %w", err)
	}
	if err := d.runtime.UpProject(ctx, ProviderProject, files.Compose, files.Env); err != nil {
		return fmt.Errorf("start OpenTelemetry Collector: %w", err)
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return err
	}
	if err := waitOTLP(ctx, d.client, endpoint); err != nil {
		return err
	}
	if resource.Provider == capability.ProviderOTelCollector {
		if err := observability.Update(observability.MetricsSource{
			ID:       "opentelemetry-collector:" + ProviderProject,
			Provider: capability.ProviderOTelCollector,
			Class:    observability.SourcePlatformProvider,
			Scope:    capability.ScopeShared,
			Network:  ProviderNetwork,
			Target:   ProviderService + ":8888",
			Path:     "/metrics",
		}); err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) Bind(_ context.Context, resource capability.Resource, _ capability.Binding) error {
	if resource.Provider == capability.ProviderExternalOTLP {
		return application.MaterializeOTLPBinding(d.app, d.files, resource.Provider, d.externalEndpoint, d.externalEndpoint)
	}
	files, err := ExistingProviderFiles()
	if err != nil {
		return err
	}
	hostEndpoint, err := providerEndpoint(files)
	if err != nil {
		return err
	}
	return application.MaterializeOTLPBinding(d.app, d.files, resource.Provider, hostEndpoint, "http://otel-collector:4318")
}

func VerifyApplication(ctx context.Context, m application.Manifest, files application.RuntimeFiles) error {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			values[key] = strings.TrimSpace(value)
		}
	}
	provider := capability.ProviderKind(values["OTLP_PROVIDER"])
	switch provider {
	case capability.ProviderOTelCollector, capability.ProviderExternalOTLP:
	default:
		return errors.New("materialized OTLP provider identity is missing or unsupported")
	}
	d := &Driver{
		app:              m,
		files:            files,
		externalEndpoint: values["OTLP_HOST_ENDPOINT"],
		client:           &http.Client{Timeout: 10 * time.Second},
	}
	resource := capability.Resource{
		Application: m.Name,
		Kind:        capability.TelemetryOTLP,
		Name:        "default",
		Provider:    provider,
	}
	return d.Verify(ctx, resource, capability.Binding{})
}

func (d *Driver) Verify(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	endpoint := d.externalEndpoint
	if resource.Provider != capability.ProviderExternalOTLP {
		files, err := ExistingProviderFiles()
		if err != nil {
			return err
		}
		endpoint, err = providerEndpoint(files)
		if err != nil {
			return err
		}
	}
	payload := probeTracePayload(d.app)
	target := strings.TrimRight(endpoint, "/") + "/v1/traces"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	if resource.Provider == capability.ProviderExternalOTLP {
		for key, value := range externalHeaders() {
			req.Header.Set(key, value)
		}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("export OTLP verification trace: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("export OTLP verification trace: endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func EnsureProviderFiles() (ProviderFiles, error) {
	return EnsureProviderFilesWithTraceBackend("", "")
}

func EnsureProviderFilesWithTraceBackend(traceEndpoint, traceNetwork string) (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	dir := filepath.Join(dataDir, "providers", "opentelemetry-collector")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ProviderFiles{}, fmt.Errorf("create OpenTelemetry Collector provider state: %w", err)
	}
	files := ProviderFiles{
		Dir:     dir,
		Compose: filepath.Join(dir, "compose.yaml"),
		Env:     filepath.Join(dir, "runtime.env"),
		Config:  filepath.Join(dir, "collector.yaml"),
	}
	port := ""
	if data, err := os.ReadFile(files.Env); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && key == "BASEHARBOR_OTLP_PORT" {
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
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_OTLP_PORT="+port+"\n"), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Config, []byte(collectorConfigWithTraceBackend(traceEndpoint)), 0o644); err != nil {
		return ProviderFiles{}, err
	}
	// The Collector image runs as a non-root user. This generated configuration
	// contains no credentials and must be readable through the read-only bind
	// mount, while the provider directory and runtime.env remain owner-only.
	if err := os.Chmod(files.Config, 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAMLWithTraceNetwork(traceNetwork)), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	return files, nil
}

func ExistingProviderFiles() (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	dir := filepath.Join(dataDir, "providers", "opentelemetry-collector")
	files := ProviderFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"), Config: filepath.Join(dir, "collector.yaml")}
	for _, path := range []string{files.Compose, files.Env, files.Config} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroySharedProvider(ctx context.Context, runtime Runtime) error {
	files, err := ExistingProviderFiles()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, ProviderProject, files.Compose, files.Env); err != nil {
		return err
	}
	_ = observability.Remove("opentelemetry-collector:" + ProviderProject)
	return os.RemoveAll(files.Dir)
}

func providerComposeYAML() string {
	return providerComposeYAMLWithTraceNetwork("")
}

func providerComposeYAMLWithTraceNetwork(traceNetwork string) string {
	var networks = "      - telemetry\n"
	var networkDecl = ""
	if strings.TrimSpace(traceNetwork) != "" {
		networks += "      - traces\n"
		networkDecl = fmt.Sprintf("  traces:\n    external: true\n    name: %q\n", strings.TrimSpace(traceNetwork))
	}
	return fmt.Sprintf(`services:
  otel-collector:
    image: otel/opentelemetry-collector-contrib:0.161.0
    restart: unless-stopped
    user: "10001:10001"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    command: ["--config=/etc/otelcol-contrib/config.yaml"]
    ports:
      - "127.0.0.1:${BASEHARBOR_OTLP_PORT}:4318"
    volumes:
      - ./collector.yaml:/etc/otelcol-contrib/config.yaml:ro
    networks:
%s
networks:
  telemetry:
    name: baseharbor-telemetry
%s`, networks, networkDecl)
}

func collectorConfig() string {
	return collectorConfigWithTraceBackend("")
}

func collectorConfigWithTraceBackend(traceEndpoint string) string {
	traceExporters := "[debug]"
	extraExporter := ""
	if strings.TrimSpace(traceEndpoint) != "" {
		traceExporters = "[debug, otlphttp/tempo]"
		extraExporter = fmt.Sprintf("  otlphttp/tempo:\n    endpoint: %s\n", strings.TrimRight(strings.TrimSpace(traceEndpoint), "/"))
	}
	return fmt.Sprintf(`receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
exporters:
  debug:
    verbosity: basic
%sservice:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: %s
    metrics:
      receivers: [otlp]
      exporters: [debug]
    logs:
      receivers: [otlp]
      exporters: [debug]
`, extraExporter, traceExporters)
}

func providerEndpoint(files ProviderFiles) (string, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "BASEHARBOR_OTLP_PORT" {
			port, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || port < 1 || port > 65535 {
				return "", errors.New("invalid OpenTelemetry Collect