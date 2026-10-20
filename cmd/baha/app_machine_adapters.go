package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// appMachineError exposes stable machine codes without leaking internal errors.
type appMachineError struct{ Code string }

func (e *appMachineError) Error() string { return "application machine: " + e.Code }

// classifyAppMachineError maps the private adapter classification (conflict,
// cancel, denied, unsupported, partial) onto the shared typed machine error
// owner so the public CLI/JSON/MCP boundary surfaces a concrete Code,
// CauseCode and remediation instead of the internal_error fallthrough. Every
// origin keeps its fail-closed semantics; no guard is weakened.
func classifyAppMachineError(err *appMachineError) *machine.Error {
	switch err.Code {
	case "conflict":
		return machine.NewError(machine.ErrorConflict,
			"application machine: "+err.Code,
			"Resubmit with the exact proposed intent, a matching snapshot revision and the current provider-consequence consent, or correct the deployment Target.",
			false)
	case "cancel":
		return machine.NewError(machine.ErrorRuntimeUnavailable,
			"application machine: "+err.Code,
			"Retry after confirming the local runtime and providers are responsive.",
			true)
	case "denied", "unsupported":
		return machine.NewError(machine.ErrorPolicyDenied,
			"application machine: "+err.Code,
			"Review ownership, authorization or provider support for this Target and retry.",
			false)
	case "partial":
		return machine.NewError(machine.ErrorVerificationFailed,
			"application machine: "+err.Code,
			"Inspect the partial change evidence and complete or roll back the affected provider operations.",
			true)
	default:
		return machine.NewError(machine.ErrorInternal,
			"application machine: "+err.Code,
			"Capture the reported machine code and investigate the underlying provider operation.",
			false)
	}
}

type appMachineRead func(context.Context, string) (GuidedReadContext, error)

// appMachineView is a read-only projection; authorization must already have
// happened in the existing GuidedReadContext reader.
type appMachineView struct {
	Target           string                           `json:"target"`
	ApplicationID    string                           `json:"application_id"`
	Requirements     []capability.Requirement         `json:"requirements"`
	Preferences      []application.ProviderPreference `json:"provider_preferences"`
	Footprint        application.Footprint            `json:"footprint"`
	SnapshotRevision string                           `json:"snapshot_revision"`
	PlanRevision     string                           `json:"plan_revision"`
}

func appMachineInspect(ctx context.Context, target string, read appMachineRead) (appMachineView, error) {
	if target == "" || read == nil {
		return appMachineView{}, &appMachineError{Code: "unsupported"}
	}
	view, err := read(ctx, target)
	if err != nil {
		return appMachineView{}, appMachineReadError(err)
	}
	if view.Target != target || view.Footprint.Target != target ||
		view.Footprint.Version != application.FootprintVersion ||
		view.ApplicationID == "" || view.PlanRevision == "" || view.SnapshotRevision == "" {
		return appMachineView{}, &appMachineError{Code: "conflict"}
	}
	// The Footprint owns requirements also stored outside the portable
	// Capabilities list (for example managed secrets).
	requirements := append([]capability.Requirement(nil), view.Capabilities...)
	for _, resolved := range view.Footprint.Requirements {
		found := false
		for _, requirement := range requirements {
			if requirement == resolved.Requirement {
				found = true
				break
			}
		}
		if !found {
			requirements = append(requirements, resolved.Requirement)
		}
	}
	// Marshal-copy the domain values, never expose the wider read context.
	var projection appMachineView
	raw, err := json.Marshal(appMachineView{
		Target: target, ApplicationID: view.ApplicationID,
		Requirements: requirements, Preferences: view.Preferences,
		Footprint: view.Footprint, SnapshotRevision: view.SnapshotRevision,
		PlanRevision: view.PlanRevision,
	})
	if err != nil || json.Unmarshal(raw, &projection) != nil {
		return appMachineView{}, &appMachineError{Code: "unsupported"}
	}
	return projection, nil
}

// appMachineHydrate uses the accepted YAML/source reconciliation authority.
func appMachineHydrate(ctx context.Context, target string, source []byte, read appEditLoadReader) (appEditLoaded, error) {
	loaded, err := appEditLoad(ctx, target, source, read)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return appEditLoaded{}, &appMachineError{Code: "cancel"}
		}
		var e *appEditLoadError
		if errors.As(err, &e) {
			switch e.Kind {
			case "missing", "wrong_target", "ambiguous", "incompatible":
				return appEditLoaded{}, &appMachineError{Code: "conflict"}
			}
		}
		return appEditLoaded{}, &appMachineError{Code: "denied"}
	}
	return loaded, nil
}

func appMachineReadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &appMachineError{Code: "cancel"}
	}
	var guided *GuidedReadError
	if errors.As(err, &guided) {
		switch guided.Kind {
		case "denied":
			return &appMachineError{Code: "denied"}
		case "wrong_target", "ambiguous", "incompatible", "missing":
			return &appMachineError{Code: "conflict"}
		case "unsupported":
			return &appMachineError{Code: "unsupported"}
		}
	}
	return &appMachineError{Code: "denied"}
}

// The registered caller must provide these existing, authorized read owners.
type appMachinePlanReader func(context.Context, string) (application.ProviderImpactPlan, error)
type appMachineAuthorize func(context.Context, string, string) error

// appMachineApply is an injected, already authorized application-owned
// execution path. This adapter does not implement runtime/provider mutations.
type appMachineApply func(context.Context, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error)

type appMachineChangeRequest struct {
	Target           string
	SnapshotRevision string
	PlanRevision     string
	Consent          application.ProviderChangeConsent
}

// appMachineChange binds one fresh authorized read to the exact impact plan.
// No execution occurs when any revision or approval is stale.
func appMachineChange(ctx context.Context, request appMachineChangeRequest, read appMachineRead,
	planRead appMachinePlanReader, authorize appMachineAuthorize, apply appMachineApply) (application.ProviderChangeResult, error) {
	if err := ctx.Err(); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if read == nil || planRead == nil || authorize == nil || request.Target == "" {
		return application.ProviderChangeResult{}, &appMachineError{Code: "unsupported"}
	}
	view, err := read(ctx, request.Target)
	if err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if err := ctx.Err(); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if view.Target != request.Target || view.Footprint.Target != request.Target ||
		view.ApplicationID == "" || request.SnapshotRevision == "" ||
		request.PlanRevision == "" || request.SnapshotRevision != view.SnapshotRevision ||
		request.PlanRevision != view.PlanRevision {
		return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
	}
	if err := authorize(ctx, request.Target, view.ApplicationID); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if err := ctx.Err(); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	plan, err := planRead(ctx, request.Target)
	if err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if err := ctx.Err(); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if plan.Target != request.Target || plan.PlanID != view.PlanRevision ||
		plan.Version != application.ProviderImpactVersion {
		return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
	}
	if err := application.ValidateProviderChangeConsent(plan, request.Consent); err != nil {
		var change *application.ProviderChangeError
		if errors.As(err, &change) {
			if change.Code == "stale-impact-consent" || change.Code == "invalid-impact-plan" {
				return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
			}
			if change.Code == "impact-confirmation-required" || change.Code == "data-deletion-consent-required" {
				return application.ProviderChangeResult{}, &appMachineError{Code: "cancel"}
			}
		}
		return application.ProviderChangeResult{}, &appMachineError{Code: "denied"}
	}
	if apply == nil {
		return application.ProviderChangeResult{}, &appMachineError{Code: "unsupported"}
	}
	if err := ctx.Err(); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	outcomes, executionErr := apply(ctx, plan, request.Consent)
	result, err := application.RecordProviderChangeResult(plan, outcomes)
	if err != nil {
		return application.ProviderChangeResult{}, &appMachineError{Code: "partial"}
	}
	if executionErr != nil {
		// Preserve the original lifecycle failure with the observed partial
		// result. The machine code stays stable for callers.
		return result, errors.Join(&appMachineError{Code: "partial"}, executionErr)
	}
	return result, nil
}

// appMachineFootprintJSON exposes exactly the canonical domain footprint bytes.
func appMachineFootprintJSON(view GuidedReadContext, target string) ([]byte, error) {
	var out bytes.Buffer
	if err := appFootprintShowContext(&out, view, appFootprintShowOptions{Target: target, JSON: true}); err != nil {
		return nil, &appMachineError{Code: "conflict"}
	}
	return bytes.Clone(bytes.TrimSpace(out.Bytes())), nil
}

// appMachineDesiredChoice is a Target-bound, full-requirement request.
// No mutation or provider selection is inferred from a bare provider kind.
// Its public JSON contract is snake_case so the MCP SDK infers consistent
// field names.
type appMachineDesiredChoice struct {
	Requirement capability.Requirement `json:"requirement"`
	InstanceID  string                 `json:"instance_id,omitempty"`
}

// appMachineDesiredRequest is the portable, Target-bound edit intent carried
// across the MCP boundary. Its public JSON contract is snake_case so the MCP
// SDK infers consistent field names; no field leaks reconciliation state.
type appMachineDesiredRequest struct {
	Target           string                    `json:"target" jsonschema:"exact resolved deployment target for this edit"`
	SnapshotRevision string                    `json:"snapshot_revision,omitempty" jsonschema:"optional current snapshot revision matched during propose"`
	Requirements     []capability.Requirement  `json:"requirements,omitempty" jsonschema:"portable capability requirements to converge"`
	Choices          []appMachineDesiredChoice `json:"choices,omitempty" jsonschema:"optional per-requirement provider instance selections"`
}
type appMachineChoiceReader func(context.Context, string, []capability.Requirement) ([]appProviderChoice, error)
type appMachineDesiredPlanner func(context.Context, string, []capability.Requirement, []application.ProviderPreference) (application.ProviderImpactPlan, error)

// appMachineDesiredResult is the typed propose response carried back across the
// MCP boundary. Its public JSON contract is snake_case so the SDK infers
// consistent field names; no field leaks internal reconciliation state.
type appMachineDesiredResult struct {
	ApplicationID    string                           `json:"application_id"`
	Desired          appMachineDesiredRequest         `json:"desired"`
	Requirements     []capability.Requirement         `json:"requirements,omitempty"`
	Preferences      []application.ProviderPreference `json:"preferences,omitempty"`
	Choices          []appProviderChoice              `json:"choices,omitempty"`
	Plan             application.ProviderImpactPlan   `json:"plan"`
	SnapshotRevision string                           `json:"snapshot_revision,omitempty"`
	PlanRevision     string                           `json:"plan_revision,omitempty"`
}

// appMachinePropose delegates provider compatibility to accepted core-913 choice
// projection and impact planning to an injected existing domain authority.
func appMachinePropose(ctx context.Context, req appMachineDesiredRequest, read appMachineRead,
	choices appMachineChoiceReader, plan appMachineDesiredPlanner) (appMachineDesiredResult, error) {
	if err := ctx.Err(); err != nil {
		return appMachineDesiredResult{}, appMachineReadError(err)
	}
	if read == nil || choices == nil || plan == nil || req.Target == "" {
		return appMachineDesiredResult{}, &appMachineError{Code: "unsupported"}
	}
	view, err := read(ctx, req.Target)
	if err != nil {
		return appMachineDesiredResult{}, appMachineReadError(err)
	}
	if err := ctx.Err(); err != nil {
		return appMachineDesiredResult{}, appMachineReadError(err)
	}
	if view.Target != req.Target || view.Footprint.Target != req.Target || req.SnapshotRevision == "" ||
		req.SnapshotRevision != view.SnapshotRevision {
		return appMachineDesiredResult{}, &appMachineError{Code: "conflict"}
	}
	seen := map[capability.Requirement]bool{}
	for _, requirement := range req.Requirements {
		if requirement.Kind == "" || requirement.Name == "" || seen[requirement] {
			return appMachineDesiredResult{}, &appMachineError{Code: "conflict"}
		}
		seen[requirement] = true
	}
	candidates, err := choices(ctx, req.Target, req.Requirements)
	if err != nil {
		return appMachineDesiredResult{}, appMachineReadError(err)
	}
	if err := ctx.Err(); err != nil {
		return appMachineDesiredResult{}, appMachineReadError(err)
	}
	eligible := map[capability.Requirement]appProviderChoice{}
	for _, choice := range candidates {
		if choice.Target != req.Target || !seen[choice.Requirement] {
			return appMachineDesiredResult{}, &appMachineError{Code: "conflict"}
		}
		eligible[choice.Requirement] = choice
	}
	if len(eligible) != len(seen) {
		return appMachineDesiredResult{}, &appMachineError{Code: "unsupported"}
	}
	selected := map[capability.Requirement]bool{}
	preferences := make([]application.ProviderPreference, 0, len(req.Choices))
	for _, wanted := range req.Choices {
		choice, ok := eligible[wanted.Requirement]
		if !ok || selected[wanted.Requirement] {
			return appMachineDesiredResult{}, &appMachineError{Code: "conflict"}
		}
		selected[wanted.Requirement] = true
		updated, err := choice.Select(wanted.InstanceID)
		if err != nil {
			return appMachineDesiredResult{}, &appMachineError{Code: "unsupported"}
		}
		preferences = append(preferences, updated.Preference().ProviderPreference)
	}
	if err := ctx.Err(); err != nil {
		return appMachineDesiredResult{}, appMachineReadError(err)
	}
	proposed, err := plan(ctx, req.Target, req.Requirements, preferences)
	if err != nil {
		return appMachineDesiredResult{}, appMachineReadError(err)
	}
	if err := ctx.Err(); err != nil {
		return appMachineDesiredResult{}, appMachineReadError(err)
	}
	if proposed.Target != req.Target || proposed.Version != application.ProviderImpactVersion ||
		proposed.PlanID == "" || proposed.Revision == "" {
		return appMachineDesiredResult{}, &appMachineError{Code: "conflict"}
	}
	return appMachineDesiredResult{
		ApplicationID: view.ApplicationID,
		Desired: appMachineDesiredRequest{
			Target: req.Target, SnapshotRevision: req.SnapshotRevision,
			Requirements: append([]capability.Requirement(nil), req.Requirements...),
			Choices:      append([]appMachineDesiredChoice(nil), req.Choices...),
		},
		Requirements: append([]capability.Requirement(nil), req.Requirements...),
		Preferences:  preferences, Choices: candidates, Plan: proposed,
		SnapshotRevision: view.SnapshotRevision, PlanRevision: proposed.PlanID,
	}, nil
}

// appMachineDesiredChangeRequest is the immutable proposal plus the caller's
// original typed intent. The existing-state PlanRevision is not the desired
// change PlanID; the authoritative planner must recompute the latter.
type appMachineDesiredChangeRequest struct {
	Desired  appMachineDesiredRequest
	Proposal appMachineDesiredResult
	Consent  application.ProviderChangeConsent
}

// appMachineChangeDesired revalidates the exact previously proposed intent
// against a fresh authorized snapshot and planning authority before apply.
// Neither this adapter nor the caller may persist a pending provider selection.
func appMachineChangeDesired(ctx context.Context, req appMachineDesiredChangeRequest,
	read appMachineRead, choices appMachineChoiceReader, planner appMachineDesiredPlanner,
	authorize appMachineAuthorize, apply appMachineApply) (application.ProviderChangeResult, error) {
	if err := ctx.Err(); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if authorize == nil || apply == nil {
		return application.ProviderChangeResult{}, &appMachineError{Code: "unsupported"}
	}
	if !reflect.DeepEqual(req.Desired, req.Proposal.Desired) ||
		req.Desired.Target == "" || req.Desired.SnapshotRevision == "" ||
		req.Proposal.ApplicationID == "" || req.Proposal.Plan.PlanID == "" ||
		req.Proposal.Plan.Revision == "" || req.Proposal.PlanRevision != req.Proposal.Plan.PlanID ||
		req.Proposal.SnapshotRevision != req.Desired.SnapshotRevision {
		return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
	}
	// The same choice and impact authorities are invoked again, not a cached
	// plan from the unchanged application or a machine-owned plan store.
	fresh, err := appMachinePropose(ctx, req.Desired, read, choices, planner)
	if err != nil {
		return application.ProviderChangeResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if fresh.ApplicationID != req.Proposal.ApplicationID ||
		fresh.SnapshotRevision != req.Proposal.SnapshotRevision ||
		fresh.PlanRevision != req.Proposal.PlanRevision {
		return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
	}
	// The fresh plan must equal the proposed plan by VALUE, not pointer identity:
	// a JSON round-trip (MCP client) produces a distinct *ProviderRealization
	// After-pointer, which reflect.DeepEqual would flag as a divergence even
	// though the resolved impact is identical. Compare serialized forms instead.
	freshPlan, freshErr := json.Marshal(fresh.Plan)
	proposedPlan, proposedErr := json.Marshal(req.Proposal.Plan)
	if freshErr != nil || proposedErr != nil || string(freshPlan) != string(proposedPlan) {
		return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
	}
	if !reflect.DeepEqual(fresh.Requirements, req.Proposal.Requirements) ||
		!reflect.DeepEqual(fresh.Preferences, req.Proposal.Preferences) {
		return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
	}
	if err := authorize(ctx, req.Desired.Target, fresh.ApplicationID); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if err := ctx.Err(); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if err := application.ValidateProviderChangeConsent(fresh.Plan, req.Consent); err != nil {
		var change *application.ProviderChangeError
		if errors.As(err, &change) {
			switch change.Code {
			case "stale-impact-consent", "invalid-impact-plan":
				return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
			case "impact-confirmation-required", "data-deletion-consent-required":
				return application.ProviderChangeResult{}, &appMachineError{Code: "cancel"}
			}
		}
		return application.ProviderChangeResult{}, &appMachineError{Code: "denied"}
	}
	if err := ctx.Err(); err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	outcomes, executionErr := apply(ctx, fresh.Plan, req.Consent)
	result, resultErr := application.RecordProviderChangeResult(fresh.Plan, outcomes)
	if resultErr != nil {
		// Keep the stable machine classification without hiding either the
		// lifecycle failure or a result-projection failure.
		return result, errors.Join(&appMachineError{Code: "partial"}, executionErr, resultErr)
	}
	if executionErr != nil {
		return result, errors.Join(&appMachineError{Code: "partial"}, executionErr)
	}
	return result, nil
}
