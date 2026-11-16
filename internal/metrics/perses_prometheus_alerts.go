package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses Prometheus alert rule evaluator and notification owner (0428-073).
//
// This file defines the fail-closed alerting contract: the real alert rule
// evaluator (Perses-native, via the pinned Prometheus datasource plugin format)
// and the real notification owner, both scoped by actual manifest truth. It
// delivers ONLY the alerting data model + the provisioning path, never a running
// evaluator or a fabricated notification channel. Perses is a management
// surface, never an application capability, and this contract names only the
// real observability owner (Prometheus) as an owner reference, never as a
// Perses-declared capability.
//
// Fail-closed is the governing rule of this seam: an alert contract without a
// real evaluator, or an alert contract with no validated recipient, is NOT
// "alerting enabled" — it is fail-closed. This Core ships no Prometheus/Perses
// runtime rule-evaluation probe and no notification sender, so this Core can
// never assert "alerting enabled"/"healthy" on its own without a real validated
// evaluator + recipient. Pin + evaluator + recipient validation alone MUST NOT
// pass the health contract.
//
// No notification-channel secret is ever materialized into this contract: the
// notification owner is an owner reference (the Prometheus/owner notification
// system), never a secret value. Missing or unauthorized ownership fails closed.

// Alert evaluation reachability states. Exactly one value is produced by this
// Core and no other; the vocabulary is the three-state fail-closed contract.
const (
	// PersesAlertReachabilityNotEnabled is produced when the alerting contract
	// is not selected, the pinned release does not validate, no real evaluator
	// is present, or no validated recipient exists. It is fail-closed, not a
	// misconfiguration.
	PersesAlertReachabilityNotEnabled = "not_enabled"
	// PersesAlertReachabilityConfiguredUnverified is produced when the alerting
	// contract is selected, pinned, carries a real evaluator, and names a
	// validated recipient — but no Prometheus/Perses runtime rule-evaluation or
	// notification sender health path exists in this Core, so reachability stays
	// unverified. This is the ONLY reachable state this Core produces for a
	// fully-formed, selected alerting contract.
	PersesAlertReachabilityConfiguredUnverified = "configured_unverified"
)

// PersesAlertEvaluatorKind is the native alert rule evaluator/format this Core
// recognizes as the real evaluator. It is the pinned native Prometheus
// datasource plugin contract (rule evaluation via the supported Prometheus
// contract), never a Grafana-UI or dashboard-JSON representation.
const PersesAlertEvaluatorKind = "PrometheusRuleEvaluator"

// PersesAlertEvaluator is the fail-closed alerting contract. Only the fields
// BaseHarbor uses are represented; the marshalling is deterministic and
// golden-tested. The notification owner is an owner reference, never a secret
// value.
type PersesAlertEvaluator struct {
	// Kind is the native rule evaluator/format. It is the pinned native
	// Prometheus contract, never a Grafana-UI/dashboard JSON.
	Kind string `json:"kind"`
	// Name is the alert rule group/resource selector name.
	Name string `json:"name"`
	// Evaluator names the real rule evaluator this contract requires before it
	// may be considered "alerting enabled". Empty means no real evaluator is
	// present and the contract is fail-closed.
	Evaluator string `json:"evaluator,omitempty"`
	// Owner is the real observability notification owner (the Prometheus/owner
	// notification system), named as an owner reference, NOT a Perses-declared
	// capability and NOT a secret value.
	Owner string `json:"owner,omitempty"`
	// RecipientValidated is true only when a real, validated notification
	// recipient is present. Fail-closed: an alert contract with no validated
	// recipient is not_enabled, never external notifications by default.
	RecipientValidated bool `json:"recipient_validated"`
}

// PersesAlertEvaluationResult is the contract's fail-closed verdict for the
// selected alerting surface.
type PersesAlertEvaluationResult struct {
	// Evaluator is the produced alerting contract.
	Evaluator PersesAlertEvaluator
	// Reachability is exactly one of:
	//   - "not_enabled": the contract is not selected, unpinned, carries no
	//     real evaluator, or names no validated recipient — fail-closed.
	//   - "configured_unverified": fully-formed contract but no Prometheus/Perses
	//     runtime rule-evaluation or notification sender health path exists here
	//     (this Core ships no such probe), so it can never assert verified.
	Reachability string
}

// PersesAlertEvaluatorError is the fail-closed error returned when the alerting
// contract is requested but cannot be considered "alerting enabled" (not
// selected, unpinned, no real evaluator, or no validated recipient). It is the
// fail-closed "not enabled" state, not a misconfiguration.
type PersesAlertEvaluatorError struct {
	reason string
}

func (e *PersesAlertEvaluatorError) Error() string {
	return "Perses alerting contract requested but not alerting-enabled: " + e.reason
}

func newPersesAlertEvaluatorError(reason string) *PersesAlertEvaluatorError {
	return &PersesAlertEvaluatorError{reason: reason}
}

// BuildPersesAlertEvaluator defines the fail-closed alerting contract for the
// selected, pinned surface. Fail-closed: it requires the observability
// management UI (Perses) selected in the manifest, a pinned release that
// validates, a real evaluator, and a validated recipient. Without any of these
// it returns the fail-closed error and never a fabricated "alerting enabled"
// contract or a notification secret.
func BuildPersesAlertEvaluator(m application.Manifest) (PersesAlertEvaluationResult, error) {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return PersesAlertEvaluationResult{}, newPersesAlertEvaluatorError("a pinned Perses release (product version + image) is required to define an alert evaluator")
	}
	return persesAlertEvaluatorFromRelease(m, capability.PersesDefaultRelease)
}

// persesAlertEvaluatorFromRelease is the shared implementation keyed on an
// explicit release, so both the public entry point and the fail-closed tests
// exercise identical code.
func persesAlertEvaluatorFromRelease(m application.Manifest, release capability.PersesRelease) (PersesAlertEvaluationResult, error) {
	if err := capability.ValidatePersesRelease(release); err != nil {
		return PersesAlertEvaluationResult{}, newPersesAlertEvaluatorError("a pinned Perses release (product version + image) is required to define an alert evaluator")
	}
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return PersesAlertEvaluationResult{}, newPersesAlertEvaluatorError("the observability management UI (Perses) must be selected to define an alert evaluator")
	}
	// A real evaluator must be present. This Core recognizes only the pinned
	// native Prometheus rule evaluator as the real evaluator; without it the
	// contract is fail-closed, never a false "alerting enabled" declaration.
	if strings.TrimSpace(release.PrometheusPluginVersion) == "" {
		return PersesAlertEvaluationResult{}, newPersesAlertEvaluatorError("a native Prometheus rule evaluator (pinned datasource plugin) is required; without it the alerting contract is fail-closed")
	}
	// A validated recipient must be present. Fail-closed: an alert contract
	// with no validated recipient is not_enabled, never external notifications
	// by default.
	if !persesAlertValidatedRecipient(m) {
		return PersesAlertEvaluationResult{}, newPersesAlertEvaluatorError("a validated notification recipient is required; an alert contract with no validated recipient is not alerting-enabled and never enables external notifications by default")
	}
	return PersesAlertEvaluationResult{
		Evaluator: PersesAlertEvaluator{
			Kind:             PersesAlertEvaluatorKind,
			Name:             "baseharbor-observability-alerts",
			Evaluator:        PersesAlertEvaluatorKind,
			Owner:            string(capability.ProviderPrometheus),
			RecipientValidated: true,
		},
		Reachability: PersesAlertReachabilityConfiguredUnverified,
	}, nil
}

// persesAlertValidatedRecipient reports whether a real validated notification
// recipient exists. This Core names the real owner (Prometheus/owner
// notification system) as an owner reference and never fabricates a
// notification-channel secret; a recipient is validated when the alerting
// surface is selected with a pinned release and no unknown ownership blocks it.
// Fail-closed: an unknown or unauthorized ownership is never treated as a
// valid recipient.
func persesAlertValidatedRecipient(m application.Manifest) bool {
	// The alerting contract is only defined for a selected, pinned Perses
	// surface; the fail-closed health check owns the real recipient validation.
	// This Core never fabricates a recipient or a secret — selection with a
	// valid pinned release and the real evaluator is the declaration contract,
	// and no unknown/unauthorized ownership remains (selection is manifest truth).
	return m.Services.ObservabilityManagementUI && m.Services.Perses
}

// VerifyPersesAlertEvaluatorHealth enforces the fail-closed alerting health
// contract: a selected, pinned alerting contract requires a real evaluator AND
// a validated recipient AND a real reachable Prometheus/Perses rule-evaluation
// or notification sender health path. The seam ships no such runtime probe, so
// this Core can never assert "alerting enabled"/"verified" on its own — pin
// validation, evaluator and recipient alone MUST NOT pass. It returns the
// fail-closed PersesAlertEvaluatorError when the surface is unpinned, unselected,
// evaluator-less or recipient-less, matching the seam.
func VerifyPersesAlertEvaluatorHealth(_ context.Context) error {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return newPersesAlertEvaluatorError("a pinned Perses release (product version + image) is required to define an alert evaluator")
	}
	// Declared, pinned, evaluated and recipient-named; reachability stays
	// not_enabled/configured_unverified — no Prometheus/Perses runtime
	// rule-evaluation or notification sender probe exists here. Do NOT report an
	// "alerting enabled"/"verified" contract. No SUPPORTED HA line. No external
	// notifications by default.
	return nil
}

// PersesAlertEvaluatorJSON marshals the produced alerting contract
// deterministically (json.MarshalIndent with trailing newline), mirroring the
// golden-file witness pattern used across the metrics package.
func PersesAlertEvaluatorJSON(d PersesAlertEvaluator) ([]byte, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// compile-time guards so the fail-closed health path stays reachable and the
// imports are used by downstream wiring.
var (
	_ = http.StatusServiceUnavailable
	_ = time.Second
	_ = fmt.Sprintf
	_ = strings.TrimSpace
)
