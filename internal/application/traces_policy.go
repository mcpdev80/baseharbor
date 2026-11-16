package application

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	TracesEnabledEnv        = "BASEHARBOR_TRACES_ENABLED"
	TracesCollectSourcesEnv = "BASEHARBOR_TRACES_COLLECT"
)

type TracesSourceClass string

const (
	TracesSourceApplication         TracesSourceClass = "application"
	TracesSourceApplicationProvider TracesSourceClass = "application-provider"
	TracesSourcePlatformProvider    TracesSourceClass = "platform-provider"
)

type TracesDeploymentPolicy struct {
	Enabled bool
	Collect map[TracesSourceClass]bool
}

func TracesPolicy(m Manifest) (TracesDeploymentPolicy, error) {
	enabled, err := TracesCollectionEnabled(m)
	if err != nil {
		return TracesDeploymentPolicy{}, err
	}
	collect := map[TracesSourceClass]bool{}
	if enabled {
		collect[TracesSourceApplication] = true
		collect[TracesSourceApplicationProvider] = true
		collect[TracesSourcePlatformProvider] = true
	}
	if raw := strings.TrimSpace(os.Getenv(TracesCollectSourcesEnv)); raw != "" {
		collect = map[TracesSourceClass]bool{}
		for _, item := range strings.Split(raw, ",") {
			class := TracesSourceClass(strings.TrimSpace(strings.ToLower(item)))
			switch class {
			case TracesSourceApplication, TracesSourceApplicationProvider, TracesSourcePlatformProvider:
				collect[class] = true
			case "":
			default:
				return TracesDeploymentPolicy{}, fmt.Errorf("%s contains unsupported source class %q", TracesCollectSourcesEnv, item)
			}
		}
		if len(collect) == 0 {
			return TracesDeploymentPolicy{}, fmt.Errorf("%s must select at least one source class", TracesCollectSourcesEnv)
		}
	}
	return TracesDeploymentPolicy{Enabled: enabled, Collect: collect}, nil
}

func TracesCollectionEnabled(m Manifest) (bool, error) {
	requested := HasTraceSignal(m)
	if raw := strings.TrimSpace(os.Getenv(TracesEnabledEnv)); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return false, fmt.Errorf("%s must be true or false", TracesEnabledEnv)
		}
		if !requested && enabled {
			return false, fmt.Errorf("%s cannot enable traces without explicit OTLP trace intent in baseharbor.yaml", TracesEnabledEnv)
		}
		return requested && enabled, nil
	}
	return requested, nil
}

func HasTraceSignal(m Manifest) bool {
	if m.Telemetry.OTLP == nil {
		return false
	}
	for _, signal := range m.Telemetry.OTLP.Signals {
		if strings.EqualFold(strings.TrimSpace(signal), "traces") {
			return true
		}
	}
	return false
}
