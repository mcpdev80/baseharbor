package application

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

const (
	MetricsEnabledEnv           = "BASEHARBOR_METRICS_ENABLED"
	MetricsProviderScopeEnv     = "BASEHARBOR_METRICS_PROVIDER_SCOPE"
	MetricsExternalReferenceEnv = "BASEHARBOR_METRICS_EXTERNAL_REFERENCE"
	MetricsCollectSourcesEnv    = "BASEHARBOR_METRICS_COLLECT"
)

type MetricsSourceClass string

const (
	MetricsSourceApplication         MetricsSourceClass = "application"
	MetricsSourceApplicationProvider MetricsSourceClass = "application-provider"
	MetricsSourcePlatformProvider    MetricsSourceClass = "platform-provider"
)

type MetricsDeploymentPolicy struct {
	Enabled           bool
	ProviderScope     capability.ProviderScope
	ExternalReference string
	Collect           map[MetricsSourceClass]bool
}

// MetricsPolicy is deployment/operator state, never portable application intent.
// Defaults deliberately grant the minimum useful collection surface:
// application metrics only. Broader provider/platform collection is explicit.
func MetricsPolicy(m Manifest) (MetricsDeploymentPolicy, error) {
	enabled, err := MetricsCollectionEnabled(m)
	if err != nil {
		return MetricsDeploymentPolicy{}, err
	}
	scope := capability.ScopeShared
	if raw := strings.TrimSpace(os.Getenv(MetricsProviderScopeEnv)); raw != "" {
		scope = capability.ProviderScope(strings.ToLower(raw))
	}
	switch scope {
	case capability.ScopeShared, capability.ScopeApplication, capability.ScopeExternal:
	default:
		return MetricsDeploymentPolicy{}, fmt.Errorf("%s must be shared, application, or external", MetricsProviderScopeEnv)
	}
	externalReference := strings.TrimSpace(os.Getenv(MetricsExternalReferenceEnv))
	if scope == capability.ScopeExternal && externalReference == "" {
		return MetricsDeploymentPolicy{}, fmt.Errorf("%s is required when %s=external", MetricsExternalReferenceEnv, MetricsProviderScopeEnv)
	}
	if scope != capability.ScopeExternal && externalReference != "" {
		return MetricsDeploymentPolicy{}, fmt.Errorf("%s is only valid when %s=external", MetricsExternalReferenceEnv, MetricsProviderScopeEnv)
	}

	collect := map[MetricsSourceClass]bool{
		MetricsSourceApplication: true,
	}
	if raw := strings.TrimSpace(os.Getenv(MetricsCollectSourcesEnv)); raw != "" {
		collect = map[MetricsSourceClass]bool{}
		for _, item := range strings.Split(raw, ",") {
			class := MetricsSourceClass(strings.TrimSpace(strings.ToLower(item)))
			switch class {
			case MetricsSourceApplication, MetricsSourceApplicationProvider, MetricsSourcePlatformProvider:
				collect[class] = true
			case "":
			default:
				return MetricsDeploymentPolicy{}, fmt.Errorf("%s contains unsupported source class %q", MetricsCollectSourcesEnv, item)
			}
		}
		if len(collect) == 0 {
			return MetricsDeploymentPolicy{}, fmt.Errorf("%s must select at least one source class", MetricsCollectSourcesEnv)
		}
	}
	if scope == capability.ScopeApplication && collect[MetricsSourcePlatformProvider] {
		return MetricsDeploymentPolicy{}, fmt.Errorf("application-scoped metrics provider cannot collect platform-provider sources")
	}
	return MetricsDeploymentPolicy{
		Enabled:           enabled,
		ProviderScope:     scope,
		ExternalReference: externalReference,
		Collect:           collect,
	}, nil
}

// MetricsCollectionEnabled is deployment policy, not portable application intent.
// Local development enables collection by default; test/staging/production require
// an explicit operator opt-in unless overridden with BASEHARBOR_METRICS_ENABLED.
func MetricsCollectionEnabled(m Manifest) (bool, error) {
	if raw := strings.TrimSpace(os.Getenv(MetricsEnabledEnv)); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return false, fmt.Errorf("%s must be true or false", MetricsEnabledEnv)
		}
		return enabled, nil
	}
	switch strings.ToLower(strings.TrimSpace(m.Environment)) {
	case "dev", "development":
		return true, nil
	default:
		return false, nil
	}
}

func MetricsTargetAlias(m Manifest, service string) string {
	sum := sha256.Sum256([]byte(m.Name + "\x00" + m.Environment + "\x00" + strings.TrimSpace(service)))
	return fmt.Sprintf("bhm-%x", sum[:8])
}

func MetricsProviderNetworkName(m Manifest, scope capability.ProviderScope) string {
	if scope == capability.ScopeApplication {
		sum := sha256.Sum256([]byte(m.Name + "\x00" + m.Environment))
		return fmt.Sprintf("baseharbor-metrics-%x", sum[:8])
	}
	return "baseharbor-metrics"
}
