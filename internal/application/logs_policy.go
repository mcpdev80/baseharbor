package application

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	LogsEnabledEnv        = "BASEHARBOR_LOGS_ENABLED"
	LogsCollectSourcesEnv = "BASEHARBOR_LOGS_COLLECT"
)

type LogsSourceClass string

const (
	LogsSourceApplication         LogsSourceClass = "application"
	LogsSourceApplicationProvider LogsSourceClass = "application-provider"
	LogsSourcePlatformProvider    LogsSourceClass = "platform-provider"
)

type LogsDeploymentPolicy struct {
	Enabled bool
	Collect map[LogsSourceClass]bool
}

func LogsPolicy(m Manifest) (LogsDeploymentPolicy, error) {
	enabled, err := LogsCollectionEnabled(m)
	if err != nil {
		return LogsDeploymentPolicy{}, err
	}
	collect := map[LogsSourceClass]bool{}
	if enabled {
		collect[LogsSourceApplication] = true
		collect[LogsSourceApplicationProvider] = true
		collect[LogsSourcePlatformProvider] = true
	}
	for _, raw := range m.Logs.Collect {
		class := LogsSourceClass(strings.TrimSpace(strings.ToLower(raw)))
		switch class {
		case LogsSourceApplication:
			// Portable intent selects logs. Deployment/placement policy decides
			// which owned source classes are eligible; it is not a provider list.
		case "":
		default:
			return LogsDeploymentPolicy{}, fmt.Errorf("manifest logs.collect contains unsupported source class %q", raw)
		}
	}
	if raw := strings.TrimSpace(os.Getenv(LogsCollectSourcesEnv)); raw != "" {
		collect = map[LogsSourceClass]bool{}
		for _, item := range strings.Split(raw, ",") {
			class := LogsSourceClass(strings.TrimSpace(strings.ToLower(item)))
			switch class {
			case LogsSourceApplication, LogsSourceApplicationProvider, LogsSourcePlatformProvider:
				collect[class] = true
			case "":
			default:
				return LogsDeploymentPolicy{}, fmt.Errorf("%s contains unsupported source class %q", LogsCollectSourcesEnv, item)
			}
		}
		if len(collect) == 0 {
			return LogsDeploymentPolicy{}, fmt.Errorf("%s must select at least one source class", LogsCollectSourcesEnv)
		}
	}
	return LogsDeploymentPolicy{Enabled: enabled, Collect: collect}, nil
}

func LogsCollectionEnabled(m Manifest) (bool, error) {
	requested := HasLogsCollection(m)
	if raw := strings.TrimSpace(os.Getenv(LogsEnabledEnv)); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return false, fmt.Errorf("%s must be true or false", LogsEnabledEnv)
		}
		if !requested && enabled {
			return false, fmt.Errorf("%s cannot enable logs without explicit logs.collect intent in baseharbor.yaml", LogsEnabledEnv)
		}
		return requested && enabled, nil
	}
	return requested, nil
}
