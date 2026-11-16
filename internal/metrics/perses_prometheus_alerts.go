package metrics

import (
	"context"
	"encoding/json"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses Prometheus alert rule evaluator and notification owner (0428-073).
//
// This file defines the fail-closed alerting contract: the real alert rule
// evaluator and the real notification owner, both scoped by actual manifest
// truth. It delivers ONLY the alerting data model + the provisioning path,
// never a running evaluator or a fabricated notification channel. Perses is a
// management surface, never an application capability, and this contract names
// only the real observability owner (Prometheus) as an owner reference, never as
// a Perses-declared capability.
//
// Fail-closed is the governing rule of this seam: an alert contract without a
// real evaluator, or an alert contract with no validated recipient, is NOT
// "alerting enabled" — it is fail-closed. This Core ships no Prometheus/Perses
// runtime rule-evaluation probe and no notification sender, so this Core can
// never assert "alerting enabled"/"healthy" on its own without a real validated
// evaluator + recipient. Pin + evaluator + recipient validation alone MUST NOT
// pass the health contract.
//
// The real evaluator is not the Prometheus datasource-plugin pin: the pin is a
// datasource reference, never a rule evaluator. It is also not an authorized
// metrics SCRAPE source: a /metrics scrape target is neither a rule evaluator
// nor a notification recipient. Luna r3 (act-luna-r3.md) confirmed that no
// real Prometheus rule-evaluation and no notification-owner (recipient/sender)
// contract exists in this Core, so this seam can never currently produce a
// positive "alerting enabled" contract: every selected, pinned surface fails
// closed (not_enabled, RecipientValidated=false). A future Owner-approved real
// evaluator + recipient contract would set
// persesAlertRealEvaluatorContractPresent() to true and gate the positive path
// behind it; until then the alerting contract stays unavailable. The datasource
// pin alone, Perses selection alone, and a materialized /metrics scrape source
// are all NOT evaluator proof and NOT recipient proof.
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
// recognizes as the real evaluator. It is the Prometheus rule evaluator (rule
// evaluation via the supported Prometheus contract), never a Grafana-UI or
// dashboard-JSON representation.
const PersesAlertEvaluatorKind = "PrometheusRuleEvaluator"

// PersesAlertEvaluator is the fail-closed alerting contract. Only the fields
// BaseHarbor uses are represented; the marshalling is deterministic and
// golden-tested. The notification owner is an owner reference, never a secret
// value.
type PersesAlertEvaluator struct {
	// Kind is the native rule evaluator/format. It is the native Prometheus
	// contract, never a Grafana-UI/dashboard JSON.
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
// validates, a real evaluator (an authorized, materialized Prometheus rule
// source), and a validated recipient. Without any of these it returns the
// fail-closed error and never a fabricated "alerting enabled" contract or a
// notification secret.
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
	// Fail-closed: the alerting contract requires a real, authorized Prometheus
	// rule-evaluation AND a validated notification owner contract. This Core
	// ships NEITHER — see the contract banner on this file and the file:line
	// finding in the round-6 evidence. An authorized metrics SCRAPE source is a
	// scrape target, never a rule evaluator, and never a notification recipient:
	// it proves neither. Selection, the datasource pin, and a materialized
	// /metrics source are all not evaluator proof and not recipient proof.
	if !persesAlertRealEvaluatorContractPresent() {
		return PersesAlertEvaluationResult{}, newPersesAlertEvaluatorError("no real Prometheus rule-evaluation and notification-owner contract exists in this Core; selection, datasource pin and an authorized metrics scrape source are not evaluator or recipient proof, so the alerting contract is fail-closed (not_enabled)")
	}
	return PersesAlertEvaluationResult{
		Evaluator: PersesAlertEvaluator{
			Kind:               PersesAlertEvaluatorKind,
			Name:               "baseharbor-observability-alerts",
			Evaluator:          PersesAlertEvaluatorKind,
			Owner:              string(capability.ProviderPrometheus),
			RecipientValidated: true,
		},
		Reachability: PersesAlertReachabilityConfiguredUnverified,
	}, nil
}

// persesAlertRealEvaluatorContractPresent enforces the fail-closed reality that
// this Core ships no real Prometheus rule-evaluation and no notification-owner
// (recipient/sender) contract. It reports false so that every selected, pinned
// surface fails closed with not_enabled and RecipientValidated=false.
//
// This is NOT a scrape-source check: an authorized /metrics scrape source is a
// scrape target, never a rule evaluator, and never a notification recipient.
// The finding that no such contract exists in this Core is documented at the
// stop criterion below and in the round-6 evidence. A future Owner-approved
// real evaluator/recipient contract would set this to true and gate the
// positive path behind it; until then the alerting contract stays unavailable.
func persesAlertRealEvaluatorContractPresent() bool {
	return false
}

// VerifyPersesAlertEvaluatorHealth enforces the fail-closed alerting health
// contract: a selected, pinned alerting contract requires a real, authorized
// evaluator (a materialized, policy-allowed Prometheus rule source) AND a
// validated recipient AND a real reachable Prometheus rule-evaluation path. The
// seam ships no such runtime probe, so this Core can never assert
// "alerting enabled"/"verified" on its own — pin validation, evaluator and
// recipient alone MUST NOT pass. It returns the fail-closed
// PersesAlertEvaluatorError when the surface is unpinned, unselected,
// evaluator-less or recipient-less, matching the seam.
func VerifyPersesAlertEvaluatorHealth(_ context.Context, m application.Manifest) error {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return newPersesAlertEvaluatorError("a pinned Perses release (product version + image) is required to define an alert evaluator")
	}
	// Ground the health contract in manifest truth: a real, authorized
	// Prometheus rule-evaluation and a validated notification owner contract are
	// both required. This Core ships neither, so the health contract always
	// fails closed (not_enabled): selection, the datasource pin, and a
	// materialized /metrics scrape source are neither evaluator nor recipient
	// proof. Pin validation alone can NEVER pass.
	if !persesAlertRealEvaluatorContractPresent() {
		return newPersesAlertEvaluatorError("no real Prometheus rule-evaluation and notification-owner contract exists in this Core; selection, datasource pin and an authorized metrics scrape source are not evaluator or recipient proof, so the alerting contract fails closed (not_enabled)")
	}
	// Declared, pinned, evaluator-present and recipient-named; reachability stays
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
