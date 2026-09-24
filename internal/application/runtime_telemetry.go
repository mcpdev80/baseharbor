package application

import (
	"errors"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

type RuntimeOTLPBinding struct {
	Provider          capability.ProviderKind
	ContainerEndpoint string
	CAFile            string
	ClientCertFile    string
	ClientKeyFile     string
	Headers           string
}

func ExistingRuntimeOTLPBinding(m Manifest, files RuntimeFiles) (RuntimeOTLPBinding, bool, error) {
	if !HasOTLPTelemetry(m) {
		return RuntimeOTLPBinding{}, false, nil
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RuntimeOTLPBinding{}, false, nil
		}
		return RuntimeOTLPBinding{}, false, err
	}
	endpoint := strings.TrimSpace(values["OTLP_CONTAINER_ENDPOINT"])
	if endpoint == "" {
		return RuntimeOTLPBinding{}, false, nil
	}
	binding := RuntimeOTLPBinding{
		Provider:          capability.ProviderKind(strings.TrimSpace(values["OTLP_PROVIDER"])),
		ContainerEndpoint: endpoint,
		CAFile:            strings.TrimSpace(values[OTLPTLSHostCAEnv]),
		ClientCertFile:    strings.TrimSpace(values[OTLPTLSHostClientCertEnv]),
		ClientKeyFile:     strings.TrimSpace(values[OTLPTLSHostClientKeyEnv]),
	}
	if binding.Provider == capability.ProviderExternalOTLP {
		binding.Headers = strings.TrimSpace(os.Getenv("BASEHARBOR_OTLP_HEADERS"))
	}
	return binding, true, nil
}
