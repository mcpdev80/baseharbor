package application

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const MetricsEnabledEnv = "BASEHARBOR_METRICS_ENABLED"

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
