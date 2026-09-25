package runtimebroker

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

type runtimeBrokerComposeConfig struct {
	manifest              application.Manifest
	appFiles              application.RuntimeFiles
	mtls                  openbao.RuntimeMTLSFiles
	tokenPath             string
	credPath              string
	permissionsPath       string
	serviceTokensPath     string
	image                 string
	docsPort              string
	otlp                  *application.RuntimeOTLPBinding
	backendNetwork        string
	secretsNetwork        string
	runtimeControlNetwork string
	telemetryNetwork      string
	metricsEnabled        bool
}

func prepareRuntimeBrokerComposeConfig(m application.Manifest, appFiles application.RuntimeFiles, mtls openbao.RuntimeMTLSFiles, tokenPath, credPath, permissionsPath, serviceTokensPath, image, docsPort string, otlp *application.RuntimeOTLPBinding) (runtimeBrokerComposeConfig, error) {
	backendNetwork := application.ApplicationBackendNetworkName(m)
	if scoped := application.ApplicationBackendNetworkNameForProject(appFiles.Project); scoped != "" {
		backendNetwork = scoped
	}

	namespace := strings.TrimSpace(strings.ReplaceAll(appFiles.Namespace, ".", "-"))
	secretsNetwork := "baseharbor-secrets"
	runtimeControlNetwork := "baseharbor-runtime-control"
	telemetryNetwork := "baseharbor-telemetry"
	if namespace != "" {
		secretsNetwork = "baseharbor-" + namespace + "-secrets"
		runtimeControlNetwork = "baseharbor-runtime-control-" + namespace
		telemetryNetwork = "baseharbor-telemetry-" + namespace
	}

	metricsPolicy, err := application.MetricsPolicy(m)
	if err != nil {
		return runtimeBrokerComposeConfig{}, err
	}
	metricsEnabled := metricsPolicy.Enabled && metricsPolicy.Collect[application.MetricsSourceApplicationProvider]

	paths := map[string]string{
		"runtime token":              tokenPath,
		"runtime permissions":        permissionsPath,
		"runtime service identities": serviceTokensPath,
		"runtime CA":                 mtls.CA,
		"broker certificate":         mtls.BrokerCert,
		"broker private key":         mtls.BrokerKey,
		"probe client certificate":   mtls.ClientCert,
		"probe client private key":   mtls.ClientKey,
	}
	if m.Services.Secrets {
		paths["OpenBao credentials"] = credPath
	}
	if otlp != nil {
		if otlp.CAFile != "" {
			paths["OTLP CA"] = otlp.CAFile
		}
		if otlp.ClientCertFile != "" {
			paths["OTLP client certificate"] = otlp.ClientCertFile
			paths["OTLP client private key"] = otlp.ClientKeyFile
		}
	}
	for label, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return runtimeBrokerComposeConfig{}, fmt.Errorf("resolve %s path: %w", label, err)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return runtimeBrokerComposeConfig{}, fmt.Errorf("inspect %s path: %w", label, err)
		}
		if !info.Mode().IsRegular() {
			return runtimeBrokerComposeConfig{}, fmt.Errorf("%s path is not a regular file", label)
		}
		paths[label] = absolute
	}

	tokenPath = paths["runtime token"]
	permissionsPath = paths["runtime permissions"]
	serviceTokensPath = paths["runtime service identities"]
	if m.Services.Secrets {
		credPath = paths["OpenBao credentials"]
	}
	mtls.CA = paths["runtime CA"]
	mtls.BrokerCert = paths["broker certificate"]
	mtls.BrokerKey = paths["broker private key"]
	mtls.ClientCert = paths["probe client certificate"]
	mtls.ClientKey = paths["probe client private key"]
	if otlp != nil {
		if otlp.CAFile != "" {
			otlp.CAFile = paths["OTLP CA"]
		}
		if otlp.ClientCertFile != "" {
			otlp.ClientCertFile = paths["OTLP client certificate"]
			otlp.ClientKeyFile = paths["OTLP client private key"]
		}
	}

	return runtimeBrokerComposeConfig{
		manifest:              m,
		appFiles:              appFiles,
		mtls:                  mtls,
		tokenPath:             tokenPath,
		credPath:              credPath,
		permissionsPath:       permissionsPath,
		serviceTokensPath:     serviceTokensPath,
		image:                 image,
		docsPort:              docsPort,
		otlp:                  otlp,
		backendNetwork:        backendNetwork,
		secretsNetwork:        secretsNetwork,
		runtimeControlNetwork: runtimeControlNetwork,
		telemetryNetwork:      telemetryNetwork,
		metricsEnabled:        metricsEnabled,
	}, nil
}

func (c runtimeBrokerComposeConfig) writeService(b *strings.Builder) {
	m := c.manifest
	b.WriteString("services:\n")
	b.WriteString("  broker:\n")
	fmt.Fprintf(b, "    image: %s\n", strconv.Quote(c.image))
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    user: \"65532:65532\"\n")
	b.WriteString("    command: [\"serve\"]\n")
	c.writeEnvironment(b)
	c.writePortsAndSecurity(b)
	c.writeServiceVolumes(b)
	c.writeServiceSecrets(b)
	c.writeHealthcheck(b)
	c.writeServiceNetworks(b)

	_ = m
}

func (c runtimeBrokerComposeConfig) writeEnvironment(b *strings.Builder) {
	m := c.manifest
	b.WriteString("    environment:\n")
	b.WriteString("      BASEHARBOR_API_LISTEN_ADDR: \"0.0.0.0:8443\"\n")
	b.WriteString("      BASEHARBOR_API_TLS_CERT_FILE: \"/run/baseharbor/identity/broker-cert.pem\"\n")
	b.WriteString("      BASEHARBOR_API_TLS_KEY_FILE: \"/run/secrets/broker-key\"\n")
	b.WriteString("      BASEHARBOR_API_TLS_CLIENT_CA_FILE: \"/run/baseharbor/identity/ca.pem\"\n")
	fmt.Fprintf(b, "      BASEHARBOR_RUNTIME_APP_NAME: %s\n", strconv.Quote(m.Name))
	fmt.Fprintf(b, "      BASEHARBOR_RUNTIME_ENVIRONMENT: %s\n", strconv.Quote(m.Environment))

	if c.otlp != nil {
		fmt.Fprintf(b, "      OTEL_EXPORTER_OTLP_ENDPOINT: %s\n", strconv.Quote(c.otlp.ContainerEndpoint))
		b.WriteString("      OTEL_EXPORTER_OTLP_PROTOCOL: \"http/protobuf\"\n")
		b.WriteString("      OTEL_SERVICE_NAME: \"runtime-broker\"\n")
		fmt.Fprintf(b, "      OTEL_RESOURCE_ATTRIBUTES: %s\n", strconv.Quote("service.namespace="+m.Name+",deployment.environment.name="+m.Environment+",baseharbor.application="+m.Name+",baseharbor.component=runtime-broker"))
		if c.otlp.CAFile != "" {
			b.WriteString("      OTEL_EXPORTER_OTLP_CERTIFICATE: \"/run/baseharbor/telemetry/ca.pem\"\n")
		}
		if c.otlp.ClientCertFile != "" {
			b.WriteString("      OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE: \"/run/baseharbor/telemetry/client-cert.pem\"\n")
			b.WriteString("      OTEL_EXPORTER_OTLP_CLIENT_KEY: \"/run/baseharbor/telemetry/client-key.pem\"\n")
		}
		if c.otlp.Headers != "" {
			fmt.Fprintf(b, "      OTEL_EXPORTER_OTLP_HEADERS: %s\n", strconv.Quote(c.otlp.Headers))
		}
	}
	if c.docsPort != "" {
		b.WriteString("      BASEHARBOR_RUNTIME_DOCS_LISTEN_ADDR: \"0.0.0.0:8081\"\n")
	}
	if m.Services.Secrets {
		b.WriteString("      BASEHARBOR_RUNTIME_SECRETS_ENABLED: \"true\"\n")
		b.WriteString("      BASEHARBOR_RUNTIME_OPENBAO_URL: \"http://openbao:8200\"\n")
		b.WriteString("      BASEHARBOR_RUNTIME_OPENBAO_CREDENTIALS_FILE: \"/run/secrets/openbao-credentials\"\n")
	}
	b.WriteString("      BASEHARBOR_RUNTIME_TOKEN_FILE: \"/run/secrets/runtime-token\"\n")
	b.WriteString("      BASEHARBOR_RUNTIME_PERMISSIONS_FILE: \"/run/baseharbor/runtime/permissions.json\"\n")
	b.WriteString("      BASEHARBOR_RUNTIME_SERVICE_TOKENS_FILE: \"/run/baseharbor/runtime/service-tokens.json\"\n")
	if len(m.Runtime.Permissions) > 0 {
		b.WriteString("      BASEHARBOR_RUNTIME_EXECUTOR_URL: \"https://baseharbor-runtime-executor:9443\"\n")
		b.WriteString("      BASEHARBOR_RUNTIME_EXECUTOR_CA_FILE: \"/run/baseharbor/identity/ca.pem\"\n")
		b.WriteString("      BASEHARBOR_RUNTIME_EXECUTOR_CERT_FILE: \"/run/secrets/probe-client-cert\"\n")
		b.WriteString("      BASEHARBOR_RUNTIME_EXECUTOR_KEY_FILE: \"/run/secrets/probe-client-key\"\n")
		b.WriteString("      BASEHARBOR_RUNTIME_OPERATIONS_DIR: \"/var/lib/baseharbor/runtime-operations\"\n")
		if application.HasRuntimeMetricsPermissions(m) {
			b.WriteString("      BASEHARBOR_RUNTIME_METRICS_TARGETS_DIR: \"/var/lib/baseharbor/runtime-metrics\"\n")
		}
	}
}

func (c runtimeBrokerComposeConfig) writePortsAndSecurity(b *strings.Builder) {
	if c.docsPort != "" {
		b.WriteString("    ports:\n")
		fmt.Fprintf(b, "      - %s\n", strconv.Quote("127.0.0.1:"+c.docsPort+":8081"))
	}
	b.WriteString("    read_only: true\n")
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - \"/tmp:rw,noexec,nosuid,nodev,size=16m\"\n")
	b.WriteString("    cap_drop:\n")
	b.WriteString("      - ALL\n")
	b.WriteString("    security_opt:\n")
	b.WriteString("      - \"no-new-privileges:true\"\n")
}

func (c runtimeBrokerComposeConfig) writeServiceVolumes(b *strings.Builder) {
	b.WriteString("    volumes:\n")
	fmt.Fprintf(b, "      - %s\n", strconv.Quote(c.mtls.CA+":/run/baseharbor/identity/ca.pem:ro"))
	fmt.Fprintf(b, "      - %s\n", strconv.Quote(c.mtls.BrokerCert+":/run/baseharbor/identity/broker-cert.pem:ro"))
	fmt.Fprintf(b, "      - %s\n", strconv.Quote(c.permissionsPath+":/run/baseharbor/runtime/permissions.json:ro"))
	fmt.Fprintf(b, "      - %s\n", strconv.Quote(c.serviceTokensPath+":/run/baseharbor/runtime/service-tokens.json:ro"))
	if c.otlp != nil {
		if c.otlp.CAFile != "" {
			fmt.Fprintf(b, "      - %s\n", strconv.Quote(c.otlp.CAFile+":/run/baseharbor/telemetry/ca.pem:ro"))
		}
		if c.otlp.ClientCertFile != "" {
			fmt.Fprintf(b, "      - %s\n", strconv.Quote(c.otlp.ClientCertFile+":/run/baseharbor/telemetry/client-cert.pem:ro"))
			fmt.Fprintf(b, "      - %s\n", strconv.Quote(c.otlp.ClientKeyFile+":/run/baseharbor/telemetry/client-key.pem:ro"))
		}
	}
	if len(c.manifest.Runtime.Permissions) > 0 {
		b.WriteString("      - runtime-operations:/var/lib/baseharbor/runtime-operations\n")
	}
	if application.HasRuntimeMetricsPermissions(c.manifest) {
		b.WriteString("      - runtime-metrics:/var/lib/baseharbor/runtime-metrics\n")
	}
}

func (c runtimeBrokerComposeConfig) writeServiceSecrets(b *strings.Builder) {
	b.WriteString("    secrets:\n")
	b.WriteString("      - broker-key\n")
	if c.manifest.Services.Secrets {
		b.WriteString("      - openbao-credentials\n")
	}
	b.WriteString("      - runtime-token\n")
	b.WriteString("      - probe-client-cert\n")
	b.WriteString("      - probe-client-key\n")
}

func (c runtimeBrokerComposeConfig) writeHealthcheck(b *strings.Builder) {
	b.WriteString("    healthcheck:\n")
	b.WriteString("      test: [\"CMD\", \"curl\", \"--fail\", \"--silent\", \"--show-error\", \"--resolve\", \"baseharbor-runtime:8443:127.0.0.1\", \"--cacert\", \"/run/baseharbor/identity/ca.pem\", \"--cert\", \"/run/secrets/probe-client-cert\", \"--key\", \"/run/secrets/probe-client-key\", \"https://baseharbor-runtime:8443/readyz\"]\n")
	b.WriteString("      interval: 5s\n")
	b.WriteString("      timeout: 5s\n")
	b.WriteString("      retries: 12\n")
	b.WriteString("      start_period: 2s\n")
}

func (c runtimeBrokerComposeConfig) writeServiceNetworks(b *strings.Builder) {
	b.WriteString("    networks:\n")
	b.WriteString("      backend:\n")
	b.WriteString("        aliases:\n")
	b.WriteString("          - baseharbor-runtime\n")
	b.WriteString("          - baseharbor-secrets\n")
	if c.metricsEnabled {
		b.WriteString("      observability:\n")
		b.WriteString("        aliases:\n")
		b.WriteString("          - baseharbor-runtime\n")
	}
	if c.manifest.Services.Secrets {
		b.WriteString("      secrets: {}\n")
	}
	if len(c.manifest.Runtime.Permissions) > 0 {
		b.WriteString("      runtime-control: {}\n")
	}
	if c.otlp != nil && c.otlp.Provider == capability.ProviderOTelCollector {
		b.WriteString("      telemetry: {}\n")
	}
}

func (c runtimeBrokerComposeConfig) writeSecretsAndVolumes(b *strings.Builder) {
	b.WriteString("\nsecrets:\n")
	b.WriteString("  broker-key:\n")
	fmt.Fprintf(b, "    file: %s\n", strconv.Quote(c.mtls.BrokerKey))
	if c.manifest.Services.Secrets {
		b.WriteString("  openbao-credentials:\n")
		fmt.Fprintf(b, "    file: %s\n", strconv.Quote(c.credPath))
	}
	b.WriteString("  runtime-token:\n")
	fmt.Fprintf(b, "    file: %s\n", strconv.Quote(c.tokenPath))
	b.WriteString("  probe-client-cert:\n")
	fmt.Fprintf(b, "    file: %s\n", strconv.Quote(c.mtls.ClientCert))
	b.WriteString("  probe-client-key:\n")
	fmt.Fprintf(b, "    file: %s\n", strconv.Quote(c.mtls.ClientKey))

	if len(c.manifest.Runtime.Permissions) > 0 {
		b.WriteString("\nvolumes:\n")
		b.WriteString("  runtime-operations:\n")
	}
	if application.HasRuntimeMetricsPermissions(c.manifest) {
		if len(c.manifest.Runtime.Permissions) == 0 {
			b.WriteString("\nvolumes:\n")
		}
		b.WriteString("  runtime-metrics:\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(application.MetricsRuntimeTargetVolumeNameForNamespace(c.manifest, c.appFiles.Namespace)))
	}
}

func (c runtimeBrokerComposeConfig) writeNetworks(b *strings.Builder) {
	m := c.manifest
	b.WriteString("\nnetworks:\n")
	b.WriteString("  backend:\n")
	if application.HasManagedRuntimeServices(m) {
		b.WriteString("    external: true\n")
	}
	fmt.Fprintf(b, "    name: %s\n", strconv.Quote(c.backendNetwork))

	if c.metricsEnabled {
		b.WriteString("  observability:\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(ObservabilityNetworkNameForRuntime(m, c.appFiles)))
		b.WriteString("    internal: true\n")
	}
	if m.Services.Secrets {
		b.WriteString("  secrets:\n")
		b.WriteString("    external: true\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(c.secretsNetwork))
	}
	if len(m.Runtime.Permissions) > 0 {
		b.WriteString("  runtime-control:\n")
		b.WriteString("    external: true\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(c.runtimeControlNetwork))
	}
	if c.otlp != nil && c.otlp.Provider == capability.ProviderOTelCollector {
		b.WriteString("  telemetry:\n")
		b.WriteString("    external: true\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(c.telemetryNetwork))
	}
}
