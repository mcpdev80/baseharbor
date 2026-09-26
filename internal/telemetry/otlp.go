package telemetry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	ProviderProject    = "baseharbor-telemetry"
	ProviderService    = "otel-collector"
	ProviderNetwork    = "baseharbor-telemetry"
	ProviderImage      = "docker.io/otel/opentelemetry-collector-contrib:0.161.0"
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
	issuer           serviceaccess.Issuer
	externalEndpoint string
	traceEndpoint    string
	traceNetwork     string
	client           *http.Client
	dataDir          string
	namespace        string
}

type ProviderFiles struct {
	Dir     string
	Compose string
	Env     string
	Config  string
	Project string
	Network string
}

func NewDriver(runtime Runtime, app application.Manifest, files application.RuntimeFiles, issuer serviceaccess.Issuer) *Driver {
	return &Driver{
		runtime:          runtime,
		app:              app,
		files:            files,
		issuer:           issuer,
		externalEndpoint: application.ExternalOTLPEndpoint(),
	}
}

func NewDriverAt(runtime Runtime, app application.Manifest, files application.RuntimeFiles, issuer serviceaccess.Issuer, dataDir, namespace string) *Driver {
	return &Driver{
		runtime:          runtime,
		app:              app,
		files:            files,
		issuer:           issuer,
		externalEndpoint: application.ExternalOTLPEndpoint(),
		dataDir:          filepath.Clean(dataDir),
		namespace:        strings.TrimSpace(namespace),
	}
}

func (d *Driver) ensureProviderFiles(ctx context.Context) (ProviderFiles, error) {
	if d.dataDir != "" && d.dataDir != "." {
		return EnsureProviderFilesWithTraceBackendForEnvironmentAt(ctx, d.issuer, d.traceEndpoint, d.traceNetwork, d.app.Environment, d.dataDir, d.namespace)
	}
	return EnsureProviderFilesWithTraceBackendForEnvironment(ctx, d.issuer, d.traceEndpoint, d.traceNetwork, d.app.Environment)
}

func (d *Driver) existingProviderFiles() (ProviderFiles, error) {
	if d.dataDir != "" && d.dataDir != "." {
		return ExistingProviderFilesAt(d.dataDir, d.namespace)
	}
	return ExistingProviderFiles()
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
		if err != nil || u.Host == "" || u.Scheme != "https" {
			return errors.New("external OTLP endpoint must be an absolute https URL")
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
	files, err := d.ensureProviderFiles(ctx)
	if err != nil {
		return err
	}
	if err := d.runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate OpenTelemetry Collector configuration: %w", err)
	}
	if err := d.runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("start OpenTelemetry Collector: %w", err)
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return err
	}
	d.client, err = managedOTLPHTTPClient(d.app.Environment, files)
	if err != nil {
		return err
	}
	if err := waitOTLP(ctx, d.client, endpoint); err != nil {
		return err
	}
	if resource.Provider == capability.ProviderOTelCollector {
		metricsPolicy, err := application.MetricsPolicy(d.app)
		if err != nil {
			return err
		}
		metricsEnabled := (application.HasMetricsSources(d.app) || application.HasRuntimeMetricsPermissions(d.app)) &&
			metricsPolicy.Enabled && metricsPolicy.Collect[application.MetricsSourcePlatformProvider]
		signals := map[string]observability.ProviderSignalRuntime{}
		if metricsEnabled {
			signals["collector-metrics"] = observability.ProviderSignalRuntime{
				Network: files.Network,
				Target:  ProviderService + ":8888",
			}
		}
		if err := observability.RegisterProviderSignals(observability.ProviderSignalRegistration{
			ID:         "opentelemetry-collector:" + files.Project,
			Descriptor: capability.OTelCollectorIntegration,
			Class:      observability.SourcePlatformProvider,
			Scope:      capability.ScopeShared,
			Enabled:    map[observability.SignalKind]bool{observability.SignalMetrics: metricsEnabled},
			Signals:    signals,
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
	files, err := d.existingProviderFiles()
	if err != nil {
		return err
	}
	hostEndpoint, err := providerEndpoint(files)
	if err != nil {
		return err
	}
	policy, err := serviceaccess.Resolve(d.app.Environment, "opentelemetry-collector", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return fmt.Errorf("load managed OTLP TLS material: %w", err)
	}
	if err := application.MaterializeOTLPTLSBinding(d.app, d.files, material.CA, material.ClientCertificate, material.ClientKey); err != nil {
		return err
	}
	containerHost := "otel-collector-access"
	if policy.PKISource != serviceaccess.PKIManagedLocal && strings.TrimSpace(policy.ServerName) != "" {
		containerHost = strings.TrimSpace(policy.ServerName)
	}
	return application.MaterializeOTLPBinding(d.app, d.files, resource.Provider, hostEndpoint, "https://"+containerHost+":8443")
}

func VerifyApplication(ctx context.Context, m application.Manifest, files application.RuntimeFiles) error {
	return VerifyApplicationAt(ctx, m, files, "", "")
}

func VerifyApplicationAt(ctx context.Context, m application.Manifest, files application.RuntimeFiles, dataDir, namespace string) error {
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
		dataDir:          filepath.Clean(dataDir),
		namespace:        strings.TrimSpace(namespace),
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
		files, err := d.existingProviderFiles()
		if err != nil {
			return err
		}
		endpoint, err = providerEndpoint(files)
		if err != nil {
			return err
		}
		d.client, err = managedOTLPHTTPClient(d.app.Environment, files)
		if err != nil {
			return err
		}
	} else if d.client == nil {
		d.client = &http.Client{Timeout: 10 * time.Second}
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

func EnsureProviderFiles(ctx context.Context, issuer serviceaccess.Issuer) (ProviderFiles, error) {
	return EnsureProviderFilesWithTraceBackendForEnvironment(ctx, issuer, "", "", "dev")
}

func EnsureProviderFilesWithTraceBackend(ctx context.Context, issuer serviceaccess.Issuer, traceEndpoint, traceNetwork string) (ProviderFiles, error) {
	return EnsureProviderFilesWithTraceBackendForEnvironment(ctx, issuer, traceEndpoint, traceNetwork, "dev")
}

func EnsureProviderFilesWithTraceBackendForEnvironment(ctx context.Context, issuer serviceaccess.Issuer, traceEndpoint, traceNetwork, environment string) (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	return EnsureProviderFilesWithTraceBackendForEnvironmentAt(ctx, issuer, traceEndpoint, traceNetwork, environment, dataDir, "")
}

func EnsureProviderFilesWithTraceBackendForEnvironmentAt(ctx context.Context, issuer serviceaccess.Issuer, traceEndpoint, traceNetwork, environment, dataDir, namespace string) (ProviderFiles, error) {
	dir := filepath.Join(filepath.Clean(dataDir), "providers", "opentelemetry-collector")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ProviderFiles{}, fmt.Errorf("create OpenTelemetry Collector provider state: %w", err)
	}
	files := ProviderFiles{
		Dir:     dir,
		Compose: filepath.Join(dir, "compose.yaml"),
		Env:     filepath.Join(dir, "runtime.env"),
		Config:  filepath.Join(dir, "collector.yaml"),
		Project: bhruntime.SharedProjectName(namespace),
		Network: scopedTelemetryName(ProviderNetwork, namespace),
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
	accessPolicy, err := serviceaccess.Resolve(environment, "opentelemetry-collector", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return ProviderFiles{}, err
	}
	accessFiles, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, files.Dir, otlpAccessSpec())
	if err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAMLWithTraceNetworkAndAccessForNetwork(traceNetwork, accessFiles, files.Network)), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	return files, nil
}

func ExistingProviderFiles() (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	return ExistingProviderFilesAt(dataDir, "")
}

func ExistingProviderFilesAt(dataDir, namespace string) (ProviderFiles, error) {
	dir := filepath.Join(filepath.Clean(dataDir), "providers", "opentelemetry-collector")
	files := ProviderFiles{
		Dir:     dir,
		Compose: filepath.Join(dir, "compose.yaml"),
		Env:     filepath.Join(dir, "runtime.env"),
		Config:  filepath.Join(dir, "collector.yaml"),
		Project: bhruntime.SharedProjectName(namespace),
		Network: scopedTelemetryName(ProviderNetwork, namespace),
	}
	for _, path := range []string{files.Compose, files.Env, files.Config} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroySharedProvider(ctx context.Context, runtime Runtime) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return DestroySharedProviderAt(ctx, runtime, dataDir, "")
}

func DestroySharedProviderAt(ctx context.Context, runtime Runtime, dataDir, namespace string) error {
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return err
	}
	_ = observability.Remove("opentelemetry-collector:" + files.Project)
	return os.RemoveAll(files.Dir)
}

func scopedTelemetryName(base, namespace string) string {
	namespace = strings.TrimSpace(strings.ReplaceAll(namespace, ".", "-"))
	if namespace == "" {
		return base
	}
	return base + "-" + namespace
}

func providerComposeYAML() string {
	return providerComposeYAMLWithTraceNetwork("")
}

func providerComposeYAMLWithTraceNetwork(traceNetwork string) string {
	access := serviceaccess.HTTPGatewayFiles{Caddyfile: "./service-access/Caddyfile", Material: serviceaccess.TLSMaterial{CA: "./service-access/runtime/ca.pem", ServerCertificate: "./service-access/runtime/server.pem", ServerKey: "./service-access/runtime/server-key.pem"}}
	return providerComposeYAMLWithTraceNetworkAndAccess(traceNetwork, access)
}

func providerComposeYAMLWithTraceNetworkAndAccess(traceNetwork string, access serviceaccess.HTTPGatewayFiles) string {
	return providerComposeYAMLWithTraceNetworkAndAccessForNetwork(traceNetwork, access, ProviderNetwork)
}

func providerComposeYAMLWithTraceNetworkAndAccessForNetwork(traceNetwork string, access serviceaccess.HTTPGatewayFiles, telemetryNetwork string) string {
	var networks = "      - telemetry\n"
	var networkDecl = ""
	if strings.TrimSpace(traceNetwork) != "" {
		networks += "      - traces\n"
		networkDecl = fmt.Sprintf("  traces:\n    external: true\n    name: %q\n", strings.TrimSpace(traceNetwork))
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`services:
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
    volumes:
      - ./collector.yaml:/etc/otelcol-contrib/config.yaml:ro
    networks:
%s`, networks))
	b.WriteString(serviceaccess.HTTPGatewayComposeService(access, otlpAccessSpec()))
	b.WriteString(fmt.Sprintf(`networks:
  telemetry:
    name: ${BASEHARBOR_TELEMETRY_NETWORK}
%s`, networkDecl))
	return strings.ReplaceAll(b.String(), "${BASEHARBOR_TELEMETRY_NETWORK}", telemetryNetwork)
}

func collectorConfig() string {
	return collectorConfigWithTraceBackend("")
}

func collectorConfigWithTraceBackend(traceEndpoint string) string {
	traceExporters := "[debug]"
	extraExporter := ""
	if strings.TrimSpace(traceEndpoint) != "" {
		traceExporters = "[debug, otlp_http/tempo]"
		extraExporter = fmt.Sprintf("  otlp_http/tempo:\n    endpoint: %s\n", strings.TrimRight(strings.TrimSpace(traceEndpoint), "/"))
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
  telemetry:
    metrics:
      readers:
        - pull:
            exporter:
              prometheus:
                host: 0.0.0.0
                port: 8888
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

func ProviderEndpoint(files ProviderFiles) (string, error) {
	return providerEndpoint(files)
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
				return "", errors.New("invalid OpenTelemetry Collector port")
			}
			return "https://127.0.0.1:" + strconv.Itoa(port), nil
		}
	}
	return "", errors.New("OpenTelemetry Collector port is not materialized")
}

func otlpAccessSpec() serviceaccess.HTTPGatewaySpec {
	return serviceaccess.HTTPGatewaySpec{
		ServiceName:      "otel-collector-access",
		Upstream:         "http://otel-collector:4318",
		PublishedPortEnv: "BASEHARBOR_OTLP_PORT",
		ContainerPort:    8443,
		Networks:         []string{"telemetry"},
		RequireClient:    true,
	}
}

func managedOTLPHTTPClient(environment string, files ProviderFiles) (*http.Client, error) {
	policy, err := serviceaccess.Resolve(environment, "opentelemetry-collector", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return nil, err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return nil, fmt.Errorf("load OpenTelemetry Collector service access identity: %w", err)
	}
	return serviceaccess.NewHTTPClientForPolicy(material, policy)
}

func waitOTLP(ctx context.Context, client *http.Client, endpoint string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/v1/traces", bytes.NewReader(probeTracePayload(application.Manifest{Name: "probe", Environment: "probe"})))
		req.Header.Set("Content-Type", "application/x-protobuf")
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			if last == nil {
				last = ctx.Err()
			}
			return last
		case <-ticker.C:
		}
	}
}

func allocatePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func externalHeaders() map[string]string {
	result := map[string]string{}
	for _, item := range strings.Split(os.Getenv(ExternalHeadersEnv), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(item), "=")
		if ok && strings.TrimSpace(key) != "" {
			result[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return result
}

func ExportProviderInteractionTrace(ctx context.Context, m application.Manifest, source observability.SignalSource) (string, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return "", err
	}
	return ExportProviderInteractionTraceAt(ctx, m, source, dataDir, "")
}

func ExportProviderInteractionTraceAt(ctx context.Context, m application.Manifest, source observability.SignalSource, dataDir, namespace string) (string, error) {
	if source.Kind != observability.SignalTraces || source.Protocol != "interaction" {
		return "", fmt.Errorf("provider interaction trace source %q is not an interaction trace", source.ID)
	}
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		return "", err
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return "", err
	}
	client, err := managedOTLPHTTPClient(m.Environment, files)
	if err != nil {
		return "", err
	}
	payload, traceID := providerInteractionTracePayload(m, source)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/v1/traces", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("export provider interaction trace %s: %w", source.ID, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("export provider interaction trace %s: endpoint returned HTTP %d", source.ID, resp.StatusCode)
	}
	return traceID, nil
}

func providerInteractionTracePayload(m application.Manifest, source observability.SignalSource) ([]byte, string) {
	now := uint64(time.Now().UnixNano())
	identity := m.Name + "\x00" + m.Environment + "\x00" + string(source.Provider) + "\x00" + source.ID + "\x00" + strconv.FormatUint(now, 10)
	traceHash := sha256.Sum256([]byte("trace\x00" + identity))
	spanHash := sha256.Sum256([]byte("span\x00" + identity))
	traceID := append([]byte(nil), traceHash[:16]...)
	spanID := append([]byte(nil), spanHash[:8]...)

	span := appendBytes(nil, 1, traceID)
	span = appendBytes(span, 2, spanID)
	span = appendString(span, 5, "baseharbor.provider.interaction.verify")
	span = appendFixed64(span, 7, now)
	span = appendFixed64(span, 8, now+1)
	span = appendMessage(span, 9, keyValue("baseharbor.provider", string(source.Provider)))
	span = appendMessage(span, 9, keyValue("baseharbor.source", source.ID))
	span = appendMessage(span, 9, keyValue("baseharbor.source_class", string(source.Class)))
	span = appendMessage(span, 9, keyValue("baseharbor.resource", source.Target))
	if source.SemanticConvention != "" {
		span = appendMessage(span, 9, keyValue("baseharbor.semantic_convention", source.SemanticConvention))
	}

	scopeSpans := appendMessage(nil, 2, span)
	resource := []byte{}
	resource = appendMessage(resource, 1, keyValue("service.name", "baseharbor"))
	resource = appendMessage(resource, 1, keyValue("service.namespace", m.Name))
	resource = appendMessage(resource, 1, keyValue("deployment.environment.name", m.Environment))
	resource = appendMessage(resource, 1, keyValue("baseharbor.application", m.Name))
	resourceSpans := appendMessage(nil, 1, resource)
	resourceSpans = appendMessage(resourceSpans, 2, scopeSpans)
	return appendMessage(nil, 1, resourceSpans), hex.EncodeToString(traceID)
}

func VerificationTracePayload(m application.Manifest) []byte {
	return probeTracePayload(m)
}

func probeTracePayload(m application.Manifest) []byte {
	now := uint64(time.Now().UnixNano())
	traceID := []byte{0x42, 0x61, 0x73, 0x65, 0x48, 0x61, 0x72, 0x62, 0x6f, 0x72, 0x30, 0x34, 0x30, 0x37, 0x00, 0x01}
	spanID := []byte{0x42, 0x48, 0x30, 0x34, 0x30, 0x37, 0x00, 0x01}
	span := appendBytes(nil, 1, traceID)
	span = appendBytes(span, 2, spanID)
	span = appendString(span, 5, "baseharbor.otlp.verify")
	span = appendFixed64(span, 7, now)
	span = appendFixed64(span, 8, now+1)
	scopeSpans := appendMessage(nil, 2, span)
	resource := []byte{}
	resource = appendMessage(resource, 1, keyValue("service.name", m.Name))
	resource = appendMessage(resource, 1, keyValue("service.namespace", m.Name))
	resource = appendMessage(resource, 1, keyValue("deployment.environment.name", m.Environment))
	resource = appendMessage(resource, 1, keyValue("baseharbor.application", m.Name))
	resourceSpans := appendMessage(nil, 1, resource)
	resourceSpans = appendMessage(resourceSpans, 2, scopeSpans)
	return appendMessage(nil, 1, resourceSpans)
}

func keyValue(key, value string) []byte {
	any := appendString(nil, 1, value)
	msg := appendString(nil, 1, key)
	return appendMessage(msg, 2, any)
}

func appendTag(dst []byte, field int, wire byte) []byte {
	return binary.AppendUvarint(dst, uint64(field<<3)|uint64(wire))
}
func appendMessage(dst []byte, field int, msg []byte) []byte {
	dst = appendTag(dst, field, 2)
	dst = binary.AppendUvarint(dst, uint64(len(msg)))
	return append(dst, msg...)
}
func appendBytes(dst []byte, field int, value []byte) []byte {
	return appendMessage(dst, field, value)
}
func appendString(dst []byte, field int, value string) []byte {
	return appendMessage(dst, field, []byte(value))
}
func appendFixed64(dst []byte, field int, value uint64) []byte {
	dst = appendTag(dst, field, 1)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], value)
	return append(dst, buf[:]...)
}
