package application

import (
	"fmt"
	"os"
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
