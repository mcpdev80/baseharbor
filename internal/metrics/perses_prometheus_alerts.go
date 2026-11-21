package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

const (
	PersesAlertReachabilityNotEnabled           = "not_enabled"
	PersesAlertReachabilityConfiguredUnverified = "configured_unverified"
	PersesAlertEvaluatorKind                    = "PrometheusRuleEvaluator"
	persesAlertRuleName                         = "BaseHarborTargetUnavailable"
)

// PersesAlertEvaluator describes rules evaluated by the existing Prometheus
// provider. Notification routing is separate and remains opt-in.
type PersesAlertEvaluator struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Evaluator string `json:"evaluator"`
	Owner     string `json:"owner"`
}

// PersesAlertEvaluationResult reports the configured evaluator without
// claiming live health; VerifyPersesAlertEvaluatorHealth checks the real
// Target-local Prometheus instance.
type PersesAlertEvaluationResult struct {
	Evaluator    PersesAlertEvaluator `json:"evaluator"`
	Reachability string               `json:"reachability"`
}

// PersesAlertEvaluatorError is returned when no configured, verified
// Prometheus alert rule can be provided for the selected Perses surface.
type PersesAlertEvaluatorError struct {
	reason string
}

func (e *PersesAlertEvaluatorError) Error() string {
	return "Perses alerting contract unavailable: " + e.reason
}

func newPersesAlertEvaluatorError(reason string) *PersesAlertEvaluatorError {
	return &PersesAlertEvaluatorError{reason: reason}
}

// BuildPersesAlertEvaluator describes the pinned Prometheus rule evaluator for
// a selected Perses surface. It fails closed when no Prometheus metric input is
// configured; this function does not claim that the live Target has loaded or
// evaluated its rules.
func BuildPersesAlertEvaluator(m application.Manifest) (PersesAlertEvaluationResult, error) {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return PersesAlertEvaluationResult{}, newPersesAlertEvaluatorError("a pinned Perses release is required")
	}
	return persesAlertEvaluatorFromRelease(m, capability.PersesDefaultRelease)
}

func persesAlertEvaluatorFromRelease(m application.Manifest, release capability.PersesRelease) (PersesAlertEvaluationResult, error) {
	if err := capability.ValidatePersesRelease(release); err != nil {
		return PersesAlertEvaluationResult{}, newPersesAlertEvaluatorError("a pinned Perses release is required")
	}
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return PersesAlertEvaluationResult{}, newPersesAlertEvaluatorError("Perses is not selected")
	}
	if len(m.Metrics.Sources) == 0 {
		return PersesAlertEvaluationResult{}, newPersesAlertEvaluatorError("no Prometheus metric inputs are configured")
	}
	return PersesAlertEvaluationResult{
		Evaluator: PersesAlertEvaluator{
			Kind: PersesAlertEvaluatorKind, Name: "baseharbor-observability-alerts",
			Evaluator: PersesAlertEvaluatorKind, Owner: string(capability.ProviderPrometheus),
		},
		Reachability: PersesAlertReachabilityConfiguredUnverified,
	}, nil
}

// VerifyPersesAlertEvaluatorHealth checks the exact managed rule through the
// authenticated Prometheus endpoint for the manifest's resolved Target. Missing
// Target state, rule files, or a recent healthy evaluation all fail closed.
func VerifyPersesAlertEvaluatorHealth(ctx context.Context, m application.Manifest) error {
	if _, err := BuildPersesAlertEvaluator(m); err != nil {
		return err
	}
	files, err := ExistingProviderFiles(m)
	if err != nil {
		return newPersesAlertEvaluatorError("the Target has no managed Prometheus provider")
	}
	return verifyPrometheusAlertRuleHealth(ctx, m, files)
}

func verifyPrometheusAlertRuleHealth(ctx context.Context, m application.Manifest, files ProviderFiles) error {
	data, err := os.ReadFile(filepath.Join(files.RulesDir, baseHarborAlertRuleFile))
	if err != nil || string(data) != baseHarborAlertRule {
		return newPersesAlertEvaluatorError("the managed Prometheus alert rule is not configured")
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return newPersesAlertEvaluatorError("the managed Prometheus endpoint is unavailable")
	}
	client, err := providerHTTPClient(m, files)
	if err != nil {
		return newPersesAlertEvaluatorError("Target-scoped Prometheus authorization is unavailable")
	}
	return verifyPrometheusAlertRuleAPI(ctx, client, endpoint)
}

func verifyPrometheusAlertRuleAPI(ctx context.Context, client *http.Client, endpoint string) error {
	values := url.Values{"type": []string{"alert"}}
	target := strings.TrimRight(endpoint, "/") + "/api/v1/rules?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return newPersesAlertEvaluatorError("Prometheus rule evaluation cannot be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return newPersesAlertEvaluatorError(fmt.Sprintf("Prometheus rules endpoint returned HTTP %d", resp.StatusCode))
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Groups []struct {
				Name  string `json:"name"`
				Rules []struct {
					Name           string `json:"name"`
					Query          string `json:"query"`
					Health         string `json:"health"`
					LastError      string `json:"lastError"`
					LastEvaluation string `json:"lastEvaluation"`
				} `json:"rules"`
			} `json:"groups"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return newPersesAlertEvaluatorError("Prometheus rules response is invalid")
	}
	if payload.Status != "success" {
		return newPersesAlertEvaluatorError("Prometheus did not return a successful rules response")
	}
	for _, group := range payload.Data.Groups {
		if group.Name != "baseharbor-target-health" {
			continue
		}
		for _, rule := range group.Rules {
			if rule.Name != persesAlertRuleName || strings.Join(strings.Fields(rule.Query), " ") != baseHarborAlertQuery || rule.Health != "ok" || strings.TrimSpace(rule.LastError) != "" {
				continue
			}
			lastEvaluation, err := time.Parse(time.RFC3339Nano, rule.LastEvaluation)
			if err != nil || time.Since(lastEvaluation) > 90*time.Second || lastEvaluation.After(time.Now().Add(10*time.Second)) {
				return newPersesAlertEvaluatorError("Prometheus has not recently evaluated the configured rule")
			}
			return nil
		}
	}
	return newPersesAlertEvaluatorError("Prometheus has not loaded the configured alert rule")
}

// PersesAlertEvaluatorJSON marshals the contract deterministically.
func PersesAlertEvaluatorJSON(d PersesAlertEvaluator) ([]byte, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
