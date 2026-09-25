package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

const (
	OTLPProviderEnv = "BASEHARBOR_OTLP_PROVIDER"
	OTLPEndpointEnv = "BASEHARBOR_OTLP_ENDPOINT"
)

func TelemetryProviderForDeployment() capability.Provider {
	if strings.TrimSpace(os.Getenv(OTLPEndpointEnv)) != "" || strings.EqualFold(strings.TrimSpace(os.Getenv(OTLPProviderEnv)), "external") {
		return capability.ExternalOTLP
	}
	return capability.OTelCollector
}

func ExternalOTLPEndpoint() string {
	return strings.TrimSpace(os.Getenv(OTLPEndpointEnv))
}

func MaterializeOTLPBinding(m Manifest, files RuntimeFiles, provider capability.ProviderKind, hostEndpoint, containerEndpoint string) error {
	if !HasOTLPTelemetry(m) {
		return nil
	}
	hostEndpoint = strings.TrimSpace(hostEndpoint)
	containerEndpoint = strings.TrimSpace(containerEndpoint)
	if hostEndpoint == "" || containerEndpoint == "" {
		return fmt.Errorf("OTLP binding endpoint is incomplete")
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	values["OTLP_PROVIDER"] = string(provider)
	values["OTLP_HOST_ENDPOINT"] = hostEndpoint
	values["OTLP_CONTAINER_ENDPOINT"] = containerEndpoint
	if err := writeRuntimeEnv(files.Env, m, values); err != nil {
		return err
	}
	appEnv, err := loadApplicationEnvValues(files.ApplicationEnv)
	if err != nil {
		return err
	}
	appEnv["OTEL_EXPORTER_OTLP_ENDPOINT"] = hostEndpoint
	appEnv["OTEL_EXPORTER_OTLP_PROTOCOL"] = "http/protobuf"
	appEnv["OTEL_SERVICE_NAME"] = m.Name
	appEnv["OTEL_RESOURCE_ATTRIBUTES"] = telemetryResourceAttributes(m, "", string(provider))
	return writeApplicationEnvValues(files.ApplicationEnv, appEnv)
}

const (
	OTLPTLSHostCAEnv         = "OTLP_TLS_CA_FILE"
	OTLPTLSHostClientCertEnv = "OTLP_TLS_CLIENT_CERT_FILE"
	OTLPTLSHostClientKeyEnv  = "OTLP_TLS_CLIENT_KEY_FILE"

	OTLPTLSContainerCA         = "/run/baseharbor/bindings/telemetry/ca.pem"
	OTLPTLSContainerClientCert = "/run/baseharbor/bindings/telemetry/client-cert.pem"
	OTLPTLSContainerClientKey  = "/run/baseharbor/bindings/telemetry/client-key.pem"
)

func MaterializeOTLPTLSBinding(m Manifest, files RuntimeFiles, caFile, clientCertFile, clientKeyFile string) error {
	if !HasOTLPTelemetry(m) {
		return nil
	}
	caFile = strings.TrimSpace(caFile)
	clientCertFile = strings.TrimSpace(clientCertFile)
	clientKeyFile = strings.TrimSpace(clientKeyFile)
	if caFile == "" {
		return fmt.Errorf("OTLP TLS trust bundle is required")
	}
	if (clientCertFile == "") != (clientKeyFile == "") {
		return fmt.Errorf("OTLP TLS client certificate and key must be provided together")
	}
	dir := filepath.Join(files.Bindings, "telemetry")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create OTLP TLS binding directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	project := func(source, name string) (string, error) {
		if strings.TrimSpace(source) == "" {
			return "", nil
		}
		info, err := os.Lstat(source)
		if err != nil {
			return "", fmt.Errorf("inspect OTLP TLS %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", fmt.Errorf("OTLP TLS %s must be a regular non-symlink file", name)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return "", fmt.Errorf("read OTLP TLS %s: %w", name, err)
		}
		if len(data) == 0 {
			return "", fmt.Errorf("OTLP TLS %s is empty", name)
		}
		target := filepath.Join(dir, name)
		if err := writeOwnerOnlyFile(target, data); err != nil {
			return "", err
		}
		// The enclosing directory remains owner-only while the bind-mounted file
		// must be readable by an arbitrary non-root workload UID.
		if err := os.Chmod(target, 0o644); err != nil {
			return "", err
		}
		return target, nil
	}
	ca, err := project(caFile, "ca.pem")
	if err != nil {
		return err
	}
	cert, err := project(clientCertFile, "client-cert.pem")
	if err != nil {
		return err
	}
	key, err := project(clientKeyFile, "client-key.pem")
	if err != nil {
		return err
	}

	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	values[OTLPTLSHostCAEnv] = ca
	if cert != "" {
		values[OTLPTLSHostClientCertEnv] = cert
		values[OTLPTLSHostClientKeyEnv] = key
	} else {
		delete(values, OTLPTLSHostClientCertEnv)
		delete(values, OTLPTLSHostClientKeyEnv)
	}
	return writeRuntimeEnv(files.Env, m, values)
}

func telemetryResourceAttributes(m Manifest, service, provider string) string {
	attributes := []string{
		"service.namespace=" + m.Name,
		"deployment.environment.name=" + m.Environment,
		"baseharbor.application=" + m.Name,
		"baseharbor.resource=telemetry.otlp/default",
	}
	if strings.TrimSpace(provider) != "" {
		attributes = append(attributes, "baseharbor.provider="+strings.TrimSpace(provider))
	}
	if strings.TrimSpace(service) != "" {
		attributes = append(attributes, "baseharbor.workload.service="+strings.TrimSpace(service))
	}
	return strings.Join(attributes, ",")
}
