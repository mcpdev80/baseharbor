package metrics

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/observability"
)

const (
	baseHarborAlertRuleFile = "baseharbor-target-unavailable.yml"
	baseHarborAlertQuery    = `up{job=~"baseharbor-applications|baseharbor-providers"} == 0`
	baseHarborAlertRule     = `groups:
- name: baseharbor-target-health
  interval: 5s
  rules:
  - alert: BaseHarborTargetUnavailable
    expr: ` + baseHarborAlertQuery + `
    labels:
      severity: warning
    annotations:
      summary: BaseHarbor metrics target is unavailable
`
)

// syncPrometheusAlertRules provisions the small BaseHarbor-owned availability
// rule only while Prometheus has a real scrape target. Prometheus stores the
// resulting ALERTS series in its existing TSDB volume; no notification route is
// configured here.
func syncPrometheusAlertRules(dir string, sources []observability.MetricsSource) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, baseHarborAlertRuleFile)
	if EffectiveProviderTargetCount(sources) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.WriteFile(path, []byte(baseHarborAlertRule), 0o644); err != nil {
		return err
	}
	return nil
}
