package application

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const TracesEnabledEnv = "BASEHARBOR_TRACES_ENABLED"

func TracesCollectionEnabled(m Manifest) (bool, error) {
	if raw := strings.TrimSpace(os.Getenv(TracesEnabledEnv)); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return false, fmt.Errorf("%s must be true or false", TracesEnabledEnv)
		}
		return enabled, nil
	}
	return false, nil
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
